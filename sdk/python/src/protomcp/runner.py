import json
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'gen'))
import protomcp_pb2 as pb

import inspect

from protomcp.transport import Transport
from protomcp.tool import get_registered_tools
from protomcp.result import ToolResult
from protomcp.context import ToolContext
from protomcp.log import ServerLogger
from protomcp.middleware import get_registered_middleware
from protomcp import manager

log: ServerLogger = ServerLogger(send_fn=lambda msg: None)

def run():
    socket_path = os.environ.get("PROTOMCP_SOCKET")
    if not socket_path:
        print("PROTOMCP_SOCKET not set — run via 'pmcp dev'", file=sys.stderr)
        sys.exit(1)

    transport = Transport(socket_path)
    transport.connect()
    manager._init(transport)

    global log
    log = ServerLogger(send_fn=transport.send)

    # Track middleware handlers for intercept dispatch
    _mw_handlers = {}

    while True:
        try:
            env = transport.recv()
        except ConnectionError:
            break

        if env.HasField("list_tools"):
            _handle_list_tools(transport, env)
            _send_middleware_registrations(transport, _mw_handlers)
        elif env.HasField("call_tool"):
            _handle_call_tool(transport, env)
        elif env.HasField("reload"):
            _handle_reload(transport, env, _mw_handlers)
        elif env.HasField("middleware_intercept"):
            _handle_middleware_intercept(transport, env, _mw_handlers)

def _handle_list_tools(transport, env):
    tools = get_registered_tools()
    tool_defs = []
    for t in tools:
        tool_defs.append(pb.ToolDefinition(
            name=t.name,
            description=t.description,
            input_schema_json=t.input_schema_json,
            output_schema_json=t.output_schema_json,
            title=t.title,
            destructive_hint=t.destructive_hint,
            idempotent_hint=t.idempotent_hint,
            read_only_hint=t.read_only_hint,
            open_world_hint=t.open_world_hint,
            task_support=t.task_support,
        ))
    resp = pb.Envelope(
        tool_list=pb.ToolListResponse(tools=tool_defs),
        request_id=env.request_id,
    )
    transport.send(resp)

def _handle_call_tool(transport, env):
    req = env.call_tool
    tools = get_registered_tools()
    handler = None
    for t in tools:
        if t.name == req.name:
            handler = t.handler
            break

    if handler is None:
        resp = pb.Envelope(
            call_result=pb.CallToolResponse(
                is_error=True,
                result_json=json.dumps([{"type": "text", "text": f"Tool not found: {req.name}"}]),
            ),
            request_id=env.request_id,
        )
        transport.send(resp)
        return

    try:
        args = json.loads(req.arguments_json) if req.arguments_json else {}
        sig = inspect.signature(handler)
        if "ctx" in sig.parameters:
            ctx = ToolContext(
                progress_token=req.progress_token,
                send_fn=transport.send,
            )
            result = handler(ctx=ctx, **args)
        else:
            result = handler(**args)

        if isinstance(result, ToolResult):
            resp_msg = pb.CallToolResponse(
                is_error=result.is_error,
                result_json=json.dumps([{"type": "text", "text": str(result.result)}]),
                enable_tools=result.enable_tools or [],
                disable_tools=result.disable_tools or [],
            )
            if result.is_error and result.error_code:
                resp_msg.error.CopyFrom(pb.ToolError(
                    error_code=result.error_code,
                    message=result.message or "",
                    suggestion=result.suggestion or "",
                    retryable=result.retryable,
                ))
        else:
            resp_msg = pb.CallToolResponse(
                result_json=json.dumps([{"type": "text", "text": str(result)}]),
            )
    except Exception as e:
        resp_msg = pb.CallToolResponse(
            is_error=True,
            result_json=json.dumps([{"type": "text", "text": str(e)}]),
        )

    # Check if result_json exceeds chunk threshold — stream if so.
    chunk_threshold = int(os.environ.get('PROTOMCP_CHUNK_THRESHOLD', '65536'))
    result_json_str = resp_msg.result_json
    result_json_bytes = result_json_str.encode('utf-8') if result_json_str else b''

    if len(result_json_bytes) > chunk_threshold:
        transport.send_raw(
            request_id=env.request_id,
            field_name='result_json',
            data=result_json_bytes,
        )
    else:
        resp = pb.Envelope(call_result=resp_msg, request_id=env.request_id)
        transport.send(resp)

def _handle_reload(transport, env, mw_handlers):
    # For now, just acknowledge. Full reload with importlib is complex.
    resp = pb.Envelope(
        reload_response=pb.ReloadResponse(success=True),
        request_id=env.request_id,
    )
    transport.send(resp)
    # Also re-send tool list
    _handle_list_tools(transport, env)
    _send_middleware_registrations(transport, mw_handlers)

def _send_middleware_registrations(transport, mw_handlers):
    mw_defs = get_registered_middleware()
    for mw in mw_defs:
        mw_handlers[mw.name] = mw.handler
        reg = pb.Envelope(
            register_middleware=pb.RegisterMiddlewareRequest(
                name=mw.name,
                priority=mw.priority,
            ),
        )
        transport.send(reg)
        # Wait for acknowledgment
        try:
            ack = transport.recv()
        except ConnectionError:
            return
    # Send handshake-complete signal
    complete = pb.Envelope(
        reload_response=pb.ReloadResponse(success=True),
    )
    transport.send(complete)

def _handle_middleware_intercept(transport, env, mw_handlers):
    req = env.middleware_intercept
    handler = mw_handlers.get(req.middleware_name)

    resp_fields = {}
    if handler:
        try:
            result = handler(
                phase=req.phase,
                tool_name=req.tool_name,
                args_json=req.arguments_json,
                result_json=req.result_json,
                is_error=req.is_error,
            )
            if isinstance(result, dict):
                resp_fields = result
        except Exception as e:
            resp_fields = {"reject": True, "reject_reason": str(e)}

    resp = pb.Envelope(
        middleware_intercept_response=pb.MiddlewareInterceptResponse(
            arguments_json=resp_fields.get("arguments_json", ""),
            result_json=resp_fields.get("result_json", ""),
            reject=resp_fields.get("reject", False),
            reject_reason=resp_fields.get("reject_reason", ""),
        ),
        request_id=env.request_id,
    )
    transport.send(resp)
