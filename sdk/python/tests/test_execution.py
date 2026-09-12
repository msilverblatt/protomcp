"""Regression coverage for awaited tools, live cancellation and structured results."""
import asyncio
import json
import queue
import threading
import time

import pytest

from protomcp import ToolResult, tool
from protomcp import protomcp_pb2 as pb
from protomcp.context import ToolContext
from protomcp.execution import ToolDispatcher
from protomcp.local_middleware import clear_local_middleware, local_middleware
from protomcp.runner import _handle_call_tool
from protomcp.tool import clear_registry


class RecordingTransport:
    def __init__(self):
        self.messages = queue.Queue()
        self.raw = []

    def send(self, env):
        self.messages.put(env)

    def send_raw(self, **kwargs):
        self.raw.append(kwargs)


def request(name, request_id="call-1"):
    return pb.Envelope(request_id=request_id, call_tool=pb.CallToolRequest(name=name, arguments_json="{}"))


@pytest.fixture(autouse=True)
def clean():
    clear_registry()
    clear_local_middleware()
    yield
    clear_registry()
    clear_local_middleware()


def test_awaits_async_handler_and_middleware():
    @local_middleware()
    async def wrap(ctx, name, args, next_handler):
        result = await next_handler(ctx, args)
        result.result += " wrapped"
        return result

    @tool("async")
    async def answer():
        await asyncio.sleep(0)
        return ToolResult(result="42", structured_content={"answer": 42})

    transport = RecordingTransport()
    _handle_call_tool(transport, request("answer"))
    result = transport.messages.get(timeout=2).call_result
    assert json.loads(result.structured_content_json) == {"answer": 42}
    assert json.loads(result.result_json)[0]["text"] == "42 wrapped"


def test_async_exception_is_tool_error():
    @tool("failure")
    async def fail():
        await asyncio.sleep(0)
        raise ValueError("expected failure")

    transport = RecordingTransport()
    _handle_call_tool(transport, request("fail"))
    result = transport.messages.get(timeout=2).call_result
    assert result.is_error
    assert "expected failure" in result.result_json


@pytest.mark.parametrize("structured", [{}, {"rows": [[1, None, "hello"]]}])
def test_structured_text_fallback(structured):
    @tool("structured")
    def answer():
        return ToolResult(structured_content=structured)

    transport = RecordingTransport()
    _handle_call_tool(transport, request("answer"))
    result = transport.messages.get(timeout=2).call_result
    assert json.loads(result.structured_content_json) == structured
    assert json.loads(json.loads(result.result_json)[0]["text"]) == structured


@pytest.mark.parametrize("result", [
    ToolResult(result="x" * 200, structured_content={"rows": [1]}, enable_tools=["next"]),
    ToolResult(result="x" * 200, is_error=True, error_code="BAD", suggestion="retry"),
    ToolResult(result="x" * 200, disable_tools=["old"]),
])
def test_large_result_preserves_all_metadata(monkeypatch, result):
    monkeypatch.setenv("PROTOMCP_CHUNK_THRESHOLD", "10")

    @tool("large")
    def large():
        return result

    transport = RecordingTransport()
    _handle_call_tool(transport, request("large"))
    response = transport.messages.get(timeout=2).call_result
    assert not transport.raw
    assert response.is_error == result.is_error
    assert list(response.enable_tools) == (result.enable_tools or [])
    assert list(response.disable_tools) == (result.disable_tools or [])
    if result.structured_content is not None:
        assert json.loads(response.structured_content_json) == result.structured_content
    if result.error_code:
        assert response.error.error_code == result.error_code


def test_cancel_async_runs_cleanup_and_server_remains_usable():
    entered, cleaned = threading.Event(), threading.Event()

    @tool("wait")
    async def wait_forever():
        entered.set()
        try:
            await asyncio.sleep(60)
        finally:
            cleaned.set()

    @tool("ping")
    def ping():
        return ToolResult(result="pong")

    transport = RecordingTransport()
    calls = ToolDispatcher(transport, _handle_call_tool)
    try:
        calls.submit(request("wait_forever"))
        assert entered.wait(2)
        calls.cancel("unknown")
        calls.cancel("call-1")
        calls.cancel("call-1")
        assert cleaned.wait(2)
        calls.submit(request("ping", "call-2"))
        response = transport.messages.get(timeout=2)
        assert response.request_id == "call-2"
        assert "pong" in response.call_result.result_json
    finally:
        calls.close()


def test_sync_cancellation_and_queued_cancellation():
    entered = threading.Event()
    executed = []

    @tool("cooperative")
    def wait(ctx: ToolContext):
        entered.set()
        while not ctx.is_cancelled():
            time.sleep(0.005)
        return ToolResult(result="must not be delivered")

    @tool("queued")
    def queued():
        executed.append(True)

    transport = RecordingTransport()
    calls = ToolDispatcher(transport, _handle_call_tool)
    try:
        calls.submit(request("wait"))
        assert entered.wait(2)
        calls.submit(request("queued", "call-2"))
        calls.cancel("call-2")
        calls.cancel("call-1")
    finally:
        calls.close()
    assert not executed
    assert transport.messages.empty()


def test_invalid_structured_output_is_error():
    @tool("invalid")
    def invalid():
        return ToolResult(structured_content=[1, 2])

    transport = RecordingTransport()
    _handle_call_tool(transport, request("invalid"))
    response = transport.messages.get(timeout=2).call_result
    assert response.is_error
    assert "JSON object" in response.result_json
