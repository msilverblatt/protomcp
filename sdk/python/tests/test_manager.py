import sys
import os
from unittest.mock import MagicMock

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'gen'))
import protomcp_pb2 as pb

from protomcp import manager


def _make_mock_transport(response_tool_names):
    """Create a mock transport that returns an ActiveToolsResponse."""
    mock = MagicMock()
    resp = pb.Envelope(
        active_tools=pb.ActiveToolsResponse(tool_names=response_tool_names)
    )
    mock.recv.return_value = resp
    return mock


def test_enable():
    mock = _make_mock_transport(["tool_a", "tool_b"])
    manager._init(mock)
    result = manager.enable(["tool_a", "tool_b"])
    assert result == ["tool_a", "tool_b"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("enable_tools")
    assert list(sent_env.enable_tools.tool_names) == ["tool_a", "tool_b"]


def test_disable():
    mock = _make_mock_transport(["tool_a"])
    manager._init(mock)
    result = manager.disable(["tool_b"])
    assert result == ["tool_a"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("disable_tools")
    assert list(sent_env.disable_tools.tool_names) == ["tool_b"]


def test_set_allowed():
    mock = _make_mock_transport(["tool_x"])
    manager._init(mock)
    result = manager.set_allowed(["tool_x"])
    assert result == ["tool_x"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("set_allowed")


def test_set_blocked():
    mock = _make_mock_transport(["tool_y"])
    manager._init(mock)
    result = manager.set_blocked(["tool_z"])
    assert result == ["tool_y"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("set_blocked")


def test_get_active_tools():
    mock = _make_mock_transport(["a", "b", "c"])
    manager._init(mock)
    result = manager.get_active_tools()
    assert result == ["a", "b", "c"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("get_active_tools")


def test_batch():
    mock = _make_mock_transport(["tool_1"])
    manager._init(mock)
    result = manager.batch(enable=["tool_1"], disable=["tool_2"])
    assert result == ["tool_1"]
    sent_env = mock.send.call_args[0][0]
    assert sent_env.HasField("batch")
    assert list(sent_env.batch.enable) == ["tool_1"]
    assert list(sent_env.batch.disable) == ["tool_2"]


def test_not_connected():
    manager._init(None)
    manager._transport = None
    try:
        manager.get_active_tools()
        assert False, "Should have raised RuntimeError"
    except RuntimeError as e:
        assert "not connected" in str(e)
