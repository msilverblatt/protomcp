from protomcp.result import ToolResult

def test_tool_result_basic():
    r = ToolResult(result="success")
    assert r.result == "success"
    assert r.enable_tools is None

def test_tool_result_with_mutations():
    r = ToolResult(result="connected", enable_tools=["query_db"], disable_tools=["connect_db"])
    assert r.enable_tools == ["query_db"]
    assert r.disable_tools == ["connect_db"]

def test_tool_result_with_error():
    r = ToolResult(is_error=True, error_code="NOT_FOUND", message="User not found")
    assert r.is_error is True
    assert r.error_code == "NOT_FOUND"
