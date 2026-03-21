from protomcp import tool, ToolResult
from protomcp.local_middleware import local_middleware
from protomcp.runner import run

_call_log = []

@local_middleware(priority=10)
def audit_logger(ctx, tool_name, args, next_handler):
    """Logs every tool call and passes through."""
    _call_log.append({"tool": tool_name, "args": dict(args)})
    return next_handler(ctx, args)

@local_middleware(priority=20)
def arg_injector(ctx, tool_name, args, next_handler):
    """Injects a 'source' field into all tool args."""
    args["source"] = "middleware"
    return next_handler(ctx, args)

@tool(description="Echo args back as JSON")
def echo_args(**kwargs) -> str:
    import json
    return json.dumps(kwargs, sort_keys=True)

@tool(description="Get the call log")
def get_call_log(**kwargs) -> str:
    import json
    return json.dumps(_call_log, sort_keys=True)

if __name__ == "__main__":
    run()
