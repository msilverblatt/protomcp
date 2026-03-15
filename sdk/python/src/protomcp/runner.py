import json
import os
import re
import sys
import time

import protomcp.protomcp_pb2 as pb

import inspect

from protomcp.transport import Transport
from protomcp.tool import get_registered_tools, get_hidden_tool_names
from protomcp.result import ToolResult
from protomcp.context import ToolContext, _deliver_sampling_response
from protomcp.log import ServerLogger
from protomcp.middleware import get_registered_middleware
from protomcp import manager
from protomcp.resource import get_registered_resources, get_registered_resource_templates, ResourceContent
from protomcp.prompt import get_registered_prompts
from protomcp.completion import get_completion_handler, CompletionResult
from protomcp.server_context import resolve_contexts
from protomcp.local_middleware import build_middleware_chain
from protomcp.telemetry import emit_telemetry, ToolCallEvent
from protomcp.sidecar import start_sidecars
from protomcp.discovery import discover_handlers, get_config

def _uri_matches_template(template: str, uri: str) -> bool:
    """Check if a URI matches a URI template pattern like 'notes://{id}'."""
    pattern = re.sub(r'\{[^}]+\}', '[^/]+', template)
    return bool(re.fullmatch(pattern, uri))

log: ServerLogger = ServerLogger(send_fn=lambda msg: None)

_first_tool_call = True

def run():
    socket_path = os.environ.get("PROTOMCP_SOCKET")
    if not socket_path:
        print("PROTOMCP_SOCKET not set — run via 'pmcp dev'", file=sys.stderr)
        sys.exit(1)

    transport = Transport(socket_path)
    transport.connect()
    manager._init(transport)

    start_sidecars("server_start")
    discover_handlers()

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
            _disable_hidden_tools(transport)
        elif env.HasField("call_tool"):
            _handle_call_tool(transport, env)
        elif env.HasField("reload"):
            _handle_reload(transport, env, _mw_handlers)
        elif env.HasField("middleware_intercept"):
            _handle_middleware_intercept(transport, env, _mw_handlers)
        elif env.HasField("list_resources_request"):
            _handle_list_resources(transport, env)
        elif env.HasField("read_resource_request"):
            _handle_read_resource(transport, env)
        elif env.HasField("list_resource_templates_request"):
            _handle_list_resource_templates(transport, env)
        elif env.HasField("list_prompts_request"):
            _handle_list_prompts(transport, env)
        elif env.HasField("get_prompt_request"):
            _handle_get_prompt(transport, env)
        elif env.HasField("completion_request"):
            _handle_completion(transport, env)
        elif env.HasField("sampling_response"):
            _deliver_sampling_response(env.request_id, env.sampling_response)

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
    global _first_tool_call
    if _first_tool_call:
        _first_tool_call = False
        start_sidecars("first_tool_call")

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

    start_time = time.monotonic()
    action_name = ""
    try:
        args = json.loads(req.arguments_json) if req.arguments_json else {}
        # Resolve server contexts and inject into args if handler accepts them
        ctx_values = resolve_contexts(args)
        sig = inspect.signature(handler)
        for param_name, value in ctx_values.items():
            if param_name in sig.parameters:
                args[param_name] = value
        # Build middleware chain around handler
        chain = build_middleware_chain(req.name, handler)
        action_name = args.get("action", "")
        emit_telemetry(ToolCallEvent(tool_name=req.name, action=action_name, phase="start", args=dict(args)))
        start_time = time.monotonic()
        if "ctx" in sig.parameters:
            ctx = ToolContext(
                progress_token=req.progress_token,
                send_fn=transport.send,
            )
            result = chain(ctx, args)
        else:
            result = chain(None, args)

        elapsed_ms = int((time.monotonic() - start_time) * 1000)
        result_str = result.result if isinstance(result, ToolResult) else str(result)
        emit_telemetry(ToolCallEvent(tool_name=req.name, action=action_name, phase="success", args={}, result=str(result_str)[:20000], duration_ms=elapsed_ms))

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
        elapsed_ms = int((time.monotonic() - start_time) * 1000)
        emit_telemetry(ToolCallEvent(tool_name=req.name, action=action_name, phase="error", args={}, error=e, duration_ms=elapsed_ms))
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
    # Re-discover handlers if hot_reload is enabled
    if get_config().get("hot_reload"):
        discover_handlers()
    # For now, just acknowledge. Full reload with importlib is complex.
    resp = pb.Envelope(
        reload_response=pb.ReloadResponse(success=True),
        request_id=env.request_id,
    )
    transport.send(resp)
    # Also re-send tool list with empty request_id so it routes to handshakeCh
    fake_env = pb.Envelope()  # empty envelope (request_id defaults to "")
    _handle_list_tools(transport, fake_env)
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

def _disable_hidden_tools(transport):
    """Disable any tools registered with hidden=True after handshake."""
    hidden = get_hidden_tool_names()
    if not hidden:
        return
    env = pb.Envelope(disable_tools=pb.DisableToolsRequest(tool_names=hidden))
    transport.send(env)

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

def _handle_list_resources(transport, env):
    resources = get_registered_resources()
    defs = [pb.ResourceDefinition(
        uri=r.uri, name=r.name, description=r.description,
        mime_type=r.mime_type, size=r.size,
    ) for r in resources]
    resp = pb.Envelope(
        resource_list_response=pb.ResourceListResponse(resources=defs),
        request_id=env.request_id,
    )
    transport.send(resp)

def _handle_list_resource_templates(transport, env):
    templates = get_registered_resource_templates()
    defs = [pb.ResourceTemplateDefinition(
        uri_template=t.uri_template, name=t.name,
        description=t.description, mime_type=t.mime_type,
    ) for t in templates]
    resp = pb.Envelope(
        resource_template_list_response=pb.ResourceTemplateListResponse(templates=defs),
        request_id=env.request_id,
    )
    transport.send(resp)

def _handle_read_resource(transport, env):
    uri = env.read_resource_request.uri
    resources = get_registered_resources()
    templates = get_registered_resource_templates()

    handler = None
    for r in resources:
        if r.uri == uri:
            handler = r.handler
            break
    if handler is None:
        for t in templates:
            if _uri_matches_template(t.uri_template, uri):
                handler = t.handler
                break

    if handler is None:
        resp = pb.Envelope(
            read_resource_response=pb.ReadResourceResponse(contents=[
                pb.ResourceContent(uri=uri, text=f"Resource not found: {uri}", mime_type="text/plain")
            ]),
            request_id=env.request_id,
        )
        transport.send(resp)
        return

    try:
        result = handler(uri)
        if isinstance(result, ResourceContent):
            result = [result]
        elif isinstance(result, str):
            result = [ResourceContent(uri=uri, text=result)]

        contents = []
        for rc in result:
            c = pb.ResourceContent(uri=rc.uri, mime_type=rc.mime_type)
            if rc.blob:
                c.blob = rc.blob
            else:
                c.text = rc.text
            contents.append(c)

        resp = pb.Envelope(
            read_resource_response=pb.ReadResourceResponse(contents=contents),
            request_id=env.request_id,
        )
    except Exception as e:
        resp = pb.Envelope(
            read_resource_response=pb.ReadResourceResponse(contents=[
                pb.ResourceContent(uri=uri, text=str(e), mime_type="text/plain")
            ]),
            request_id=env.request_id,
        )
    transport.send(resp)

def _handle_list_prompts(transport, env):
    prompts = get_registered_prompts()
    defs = []
    for p in prompts:
        args = [pb.PromptArgument(name=a.name, description=a.description, required=a.required) for a in p.arguments]
        defs.append(pb.PromptDefinition(name=p.name, description=p.description, arguments=args))
    resp = pb.Envelope(
        prompt_list_response=pb.PromptListResponse(prompts=defs),
        request_id=env.request_id,
    )
    transport.send(resp)

def _handle_get_prompt(transport, env):
    req = env.get_prompt_request
    prompts = get_registered_prompts()
    handler = None
    for p in prompts:
        if p.name == req.name:
            handler = p.handler
            break

    if handler is None:
        resp = pb.Envelope(
            get_prompt_response=pb.GetPromptResponse(
                description=f"Prompt not found: {req.name}",
                messages=[pb.PromptMessage(role="assistant", content_json=json.dumps({"type": "text", "text": f"Prompt not found: {req.name}"}))],
            ),
            request_id=env.request_id,
        )
        transport.send(resp)
        return

    try:
        args = json.loads(req.arguments_json) if req.arguments_json else {}
        result = handler(**args)
        if not isinstance(result, list):
            result = [result]

        messages = []
        for msg in result:
            content_json = json.dumps({"type": "text", "text": msg.content})
            messages.append(pb.PromptMessage(role=msg.role, content_json=content_json))

        resp = pb.Envelope(
            get_prompt_response=pb.GetPromptResponse(description="", messages=messages),
            request_id=env.request_id,
        )
    except Exception as e:
        resp = pb.Envelope(
            get_prompt_response=pb.GetPromptResponse(
                description="Error",
                messages=[pb.PromptMessage(role="assistant", content_json=json.dumps({"type": "text", "text": str(e)}))],
            ),
            request_id=env.request_id,
        )
    transport.send(resp)

def _handle_completion(transport, env):
    req = env.completion_request
    handler = get_completion_handler(req.ref_type, req.ref_name, req.argument_name)

    if handler is None:
        resp = pb.Envelope(
            completion_response=pb.CompletionResponse(values=[], total=0, has_more=False),
            request_id=env.request_id,
        )
        transport.send(resp)
        return

    try:
        result = handler(req.argument_value)
        if isinstance(result, CompletionResult):
            resp = pb.Envelope(
                completion_response=pb.CompletionResponse(
                    values=result.values, total=result.total, has_more=result.has_more,
                ),
                request_id=env.request_id,
            )
        elif isinstance(result, list):
            resp = pb.Envelope(
                completion_response=pb.CompletionResponse(values=result, total=len(result), has_more=False),
                request_id=env.request_id,
            )
        else:
            resp = pb.Envelope(
                completion_response=pb.CompletionResponse(values=[], total=0, has_more=False),
                request_id=env.request_id,
            )
    except Exception as e:
        resp = pb.Envelope(
            completion_response=pb.CompletionResponse(values=[], total=0, has_more=False),
            request_id=env.request_id,
        )
    transport.send(resp)
