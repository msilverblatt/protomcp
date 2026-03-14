from protomcp.tool import tool, get_registered_tools, clear_registry
from protomcp.result import ToolResult
from protomcp.context import ToolContext
from protomcp.log import ServerLogger
from protomcp.middleware import middleware, get_registered_middleware
from protomcp import manager as tool_manager
from protomcp.resource import resource, resource_template, ResourceContent
from protomcp.prompt import prompt, PromptArg, PromptMessage
from protomcp.completion import completion, CompletionResult
from protomcp.group import tool_group, action, get_registered_groups, clear_group_registry
from protomcp.server_context import server_context, get_registered_contexts, clear_context_registry
from protomcp.local_middleware import local_middleware, get_local_middleware, clear_local_middleware

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
    "resource",
    "resource_template",
    "ResourceContent",
    "prompt",
    "PromptArg",
    "PromptMessage",
    "completion",
    "CompletionResult",
    "tool_group",
    "action",
    "get_registered_groups",
    "clear_group_registry",
    "server_context",
    "get_registered_contexts",
    "clear_context_registry",
    "local_middleware",
    "get_local_middleware",
    "clear_local_middleware",
]
