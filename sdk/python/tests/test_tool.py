import json
from protomcp.tool import tool, get_registered_tools, clear_registry

def test_tool_decorator_registers():
    clear_registry()
    @tool(description="Add two numbers")
    def add(a: int, b: int) -> int:
        return a + b
    tools = get_registered_tools()
    assert any(t.name == "add" for t in tools)

def test_tool_schema_from_type_hints():
    clear_registry()
    @tool(description="Search documents")
    def search(query: str, limit: int = 10) -> list:
        return []
    tools = get_registered_tools()
    t = next(t for t in tools if t.name == "search")
    schema = json.loads(t.input_schema_json)
    assert schema["type"] == "object"
    assert schema["properties"]["query"]["type"] == "string"
    assert schema["properties"]["limit"]["default"] == 10
    assert "query" in schema["required"]
    assert "limit" not in schema["required"]

def test_tool_callable():
    clear_registry()
    @tool(description="Double a number")
    def double(n: int) -> int:
        return n * 2
    assert double(5) == 10

def test_tool_optional_params():
    clear_registry()
    from typing import Optional
    @tool(description="Greet")
    def greet(name: str, greeting: Optional[str] = None) -> str:
        return f"{greeting or 'Hello'}, {name}!"
    tools = get_registered_tools()
    t = next(t for t in tools if t.name == "greet")
    schema = json.loads(t.input_schema_json)
    assert "name" in schema["required"]
    assert "greeting" not in schema["required"]
