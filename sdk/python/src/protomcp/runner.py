import json
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'gen'))
import protomcp_pb2 as pb

from protomcp.transport import Transport
from protomcp.tool import get_registered_tools
from protomcp.result import ToolResult
from protomcp import manager

def run():
    socket_path = os.environ.get("PROTOMCP_SOCKET")
    if not socket_path:
        print("PROTOMCP_SOCKET not set — run via 'protomcp dev'", file=sys.stderr)
        sys.exit(1)

    transport = Transport(socket_path)
    transport.connect()
    manager._init(transport)

    while True:
        try:
            env = transport.recv()
        except ConnectionError:
            break

        if env.HasField("list_tools"):
            _handle_list_tools(transport, env)
        elif env.HasField("call_tool"):
            _handle_call_tool(transport, env)
        elif env.HasField("reload"):
            _handle_reload(transport, env)

def _handle_list_tools(transport, env):
    tools = get_registered_tools()
    tool_defs = []
    for t in tools:
        tool_defs.append(pb.ToolDefinition(
            name=t.name,
            description=t.description,
            input_schema_json=t.input_schema_json,
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

    resp = pb.Envelope(call_result=resp_msg, request_id=env.request_id)
    transport.send(resp)

def _handle_reload(transport, env):
    # For now, just acknowledge. Full reload with importlib is complex.
    resp = pb.Envelope(
        reload_response=pb.ReloadResponse(success=True),
        request_id=env.request_id,
    )
    transport.send(resp)
    # Also re-send tool list
    _handle_list_tools(transport, env)
