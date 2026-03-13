from protomcp.tool import tool, get_registered_tools, clear_registry
from protomcp.result import ToolResult
from protomcp.context import ToolContext
from protomcp.log import ServerLogger
from protomcp.middleware import middleware, get_registered_middleware
from protomcp import manager as tool_manager

# Module-level logger; replaced with a transport-connected instance when run() is called
log: ServerLogger = ServerLogger(send_fn=lambda msg: None)

__all__ = [
    "tool",
    "get_registered_tools",
    "clear_registry",
    "ToolResult",
    "tool_manager",
    "ToolContext",
    "ServerLogger",
    "log",
    "middleware",
    "get_registered_middleware",
]
