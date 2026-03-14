from dataclasses import dataclass
from typing import Any, Callable

_local_mw_registry: list["LocalMiddlewareDef"] = []

@dataclass
class LocalMiddlewareDef:
    priority: int
    handler: Callable  # (ctx, tool_name, args, next_handler) -> ToolResult

def local_middleware(priority: int = 100):
    """Register a local (in-process) middleware that wraps tool handlers."""
    def decorator(func: Callable) -> Callable:
        _local_mw_registry.append(LocalMiddlewareDef(priority=priority, handler=func))
        return func
    return decorator

def get_local_middleware() -> list[LocalMiddlewareDef]:
    """Return middleware sorted by priority (lowest first = outermost)."""
    return sorted(_local_mw_registry, key=lambda m: m.priority)

def clear_local_middleware():
    _local_mw_registry.clear()

def build_middleware_chain(tool_name: str, handler: Callable) -> Callable:
    """Build a chain wrapping handler. Returns callable: (ctx, args_dict) -> result.

    The handler is called as handler(ctx=ctx, **args) if ctx is not None, else handler(**args).
    Each middleware receives (ctx, tool_name, args, next_handler) where next_handler is (ctx, args) -> result.
    """
    middlewares = get_local_middleware()

    def final_handler(ctx, args: dict):
        if ctx is not None:
            return handler(ctx=ctx, **args)
        return handler(**args)

    chain = final_handler
    for mw in reversed(middlewares):
        mw_handler = mw.handler
        def make_link(mw_fn, next_fn):
            def link(ctx, args: dict):
                def call_next(c, a):
                    return next_fn(c, a)
                return mw_fn(ctx, tool_name, args, call_next)
            return link
        chain = make_link(mw_handler, chain)

    return chain
