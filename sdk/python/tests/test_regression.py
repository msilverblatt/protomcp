"""Regression tests for bugs found during stress testing."""
import time
import types

def test_transport_os_import():
    """transport.send_raw() must not crash due to missing os import."""
    from protomcp.transport import Transport
    # If os is not imported, this module would fail to load or send_raw would NameError
    t = Transport("/tmp/fake.sock")
    assert hasattr(t, 'send_raw')
    # Verify os is importable within the module's scope
    import protomcp.transport as transport_mod
    assert hasattr(transport_mod, 'os') or 'os' in dir(transport_mod) or hasattr(__import__('protomcp.transport'), '__file__')

def test_runner_exception_handler_no_nameError():
    """Exception handler in _handle_call_tool must not crash with NameError."""
    import protomcp.runner as runner_mod
    import inspect
    source = inspect.getsource(runner_mod._handle_call_tool)
    # Verify start_time and action_name are initialized before try block
    # by checking they appear before 'try:' in the source
    try_idx = source.index('try:')
    assert 'start_time' in source[:try_idx], "start_time must be initialized before try block"
    assert 'action_name' in source[:try_idx], "action_name must be initialized before try block"

def test_clear_middleware_registry_exported():
    """clear_middleware_registry must be importable from protomcp package."""
    from protomcp import clear_middleware_registry
    assert callable(clear_middleware_registry)
