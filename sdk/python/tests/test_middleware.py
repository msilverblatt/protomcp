from protomcp.middleware import middleware, get_registered_middleware, clear_middleware_registry


def test_middleware_decorator_registers():
    clear_middleware_registry()

    @middleware("audit", priority=10)
    def audit(phase, tool_name, args_json, result_json, is_error):
        return {}

    mws = get_registered_middleware()
    assert len(mws) == 1
    assert mws[0].name == "audit"
    assert mws[0].priority == 10
    assert mws[0].handler is audit
    clear_middleware_registry()


def test_middleware_priority_ordering():
    clear_middleware_registry()

    @middleware("second", priority=20)
    def second(phase, tool_name, args_json, result_json, is_error):
        return {}

    @middleware("first", priority=5)
    def first(phase, tool_name, args_json, result_json, is_error):
        return {}

    mws = get_registered_middleware()
    assert len(mws) == 2
    # Registry order is insertion order, not priority-sorted
    assert mws[0].name == "second"
    assert mws[1].name == "first"
    clear_middleware_registry()


def test_clear_middleware_registry():
    clear_middleware_registry()

    @middleware("temp", priority=1)
    def temp(phase, tool_name, args_json, result_json, is_error):
        return {}

    assert len(get_registered_middleware()) == 1
    clear_middleware_registry()
    assert len(get_registered_middleware()) == 0


def test_middleware_handler_callable():
    clear_middleware_registry()

    @middleware("test_mw", priority=50)
    def test_mw(phase, tool_name, args_json, result_json, is_error):
        return {"reject": True, "reject_reason": "blocked"}

    mws = get_registered_middleware()
    result = mws[0].handler(
        phase="before",
        tool_name="delete",
        args_json="{}",
        result_json="",
        is_error=False,
    )
    assert result["reject"] is True
    assert result["reject_reason"] == "blocked"
    clear_middleware_registry()
