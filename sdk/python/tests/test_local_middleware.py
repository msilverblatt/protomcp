from protomcp.local_middleware import (
    local_middleware, get_local_middleware, clear_local_middleware,
    build_middleware_chain, LocalMiddlewareDef,
)
from protomcp.result import ToolResult

def test_register():
    clear_local_middleware()
    @local_middleware(priority=10)
    def my_mw(ctx, tool_name, args, next_handler):
        return next_handler(ctx, args)
    mws = get_local_middleware()
    assert len(mws) == 1
    assert mws[0].priority == 10

def test_priority_ordering():
    clear_local_middleware()
    @local_middleware(priority=20)
    def second(ctx, tool_name, args, next_handler):
        return next_handler(ctx, args)
    @local_middleware(priority=10)
    def first(ctx, tool_name, args, next_handler):
        return next_handler(ctx, args)
    mws = get_local_middleware()
    assert mws[0].priority == 10
    assert mws[1].priority == 20

def test_chain_order():
    clear_local_middleware()
    order = []
    @local_middleware(priority=10)
    def first(ctx, tool_name, args, next_handler):
        order.append("first_before")
        result = next_handler(ctx, args)
        order.append("first_after")
        return result
    @local_middleware(priority=20)
    def second(ctx, tool_name, args, next_handler):
        order.append("second_before")
        result = next_handler(ctx, args)
        order.append("second_after")
        return result
    def handler(**args):
        order.append("handler")
        return ToolResult(result="ok")
    chain = build_middleware_chain("test", handler)
    result = chain(None, {})
    assert order == ["first_before", "second_before", "handler", "second_after", "first_after"]
    assert result.result == "ok"

def test_modify_args():
    clear_local_middleware()
    @local_middleware(priority=10)
    def inject(ctx, tool_name, args, next_handler):
        args["extra"] = "injected"
        return next_handler(ctx, args)
    received = {}
    def handler(**args):
        received.update(args)
        return ToolResult(result="ok")
    chain = build_middleware_chain("test", handler)
    chain(None, {"original": "value"})
    assert received == {"original": "value", "extra": "injected"}

def test_short_circuit():
    clear_local_middleware()
    @local_middleware(priority=10)
    def blocker(ctx, tool_name, args, next_handler):
        return ToolResult(result="blocked", is_error=True)
    called = []
    def handler(**args):
        called.append(True)
        return ToolResult(result="ok")
    chain = build_middleware_chain("test", handler)
    result = chain(None, {})
    assert result.result == "blocked"
    assert result.is_error
    assert not called

def test_catch_exception():
    clear_local_middleware()
    @local_middleware(priority=90)
    def catcher(ctx, tool_name, args, next_handler):
        try:
            return next_handler(ctx, args)
        except ValueError as e:
            return ToolResult(result=f"Caught: {e}", is_error=True)
    def handler(**args):
        raise ValueError("bad input")
    chain = build_middleware_chain("test", handler)
    result = chain(None, {})
    assert result.is_error
    assert "Caught: bad input" in result.result

def test_empty_chain():
    clear_local_middleware()
    def handler(**args):
        return ToolResult(result="direct")
    chain = build_middleware_chain("test", handler)
    result = chain(None, {"a": 1})
    assert result.result == "direct"
