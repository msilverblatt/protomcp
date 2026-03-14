import json
import pytest

from protomcp.group import (
    action,
    tool_group,
    get_registered_groups,
    clear_group_registry,
    groups_to_tool_defs,
    _dispatch_group_action,
    _generate_action_schema,
)
from protomcp.tool import get_registered_tools, clear_registry


@pytest.fixture(autouse=True)
def clean_registries():
    clear_group_registry()
    clear_registry()
    yield
    clear_group_registry()
    clear_registry()


def test_group_registration():
    @tool_group(name="math", description="Math operations")
    class MathGroup:
        @action("add", description="Add two numbers")
        def add(self, a: int, b: int) -> int:
            return a + b

        @action("multiply", description="Multiply two numbers")
        def multiply(self, x: int, y: int) -> int:
            return x * y

    groups = get_registered_groups()
    assert len(groups) == 1
    assert groups[0].name == "math"
    assert groups[0].description == "Math operations"
    assert len(groups[0].actions) == 2


def test_action_schema_generation():
    @tool_group(name="test_schema", description="Schema test")
    class SchemaGroup:
        @action("do_thing", description="Does a thing")
        def do_thing(self, name: str, count: int = 5) -> str:
            return f"{name}: {count}"

    groups = get_registered_groups()
    act = groups[0].actions[0]
    schema = act.input_schema
    assert "name" in schema["properties"]
    assert schema["properties"]["name"]["type"] == "string"
    assert "count" in schema["properties"]
    assert schema["properties"]["count"]["type"] == "integer"
    assert schema["properties"]["count"]["default"] == 5
    assert "name" in schema["required"]
    assert "count" not in schema.get("required", [])


def test_union_strategy_schema():
    @tool_group(name="db", description="DB ops")
    class DbGroup:
        @action("query", description="Run query")
        def query(self, sql: str) -> str:
            return sql

        @action("insert", description="Insert record")
        def insert(self, table: str, data: dict) -> str:
            return "ok"

    defs = groups_to_tool_defs()
    assert len(defs) == 1
    td = defs[0]
    assert td.name == "db"
    schema = json.loads(td.input_schema_json)
    assert set(schema["properties"]["action"]["enum"]) == {"query", "insert"}
    assert "oneOf" in schema
    assert len(schema["oneOf"]) == 2
    # Find each entry by const value
    entries = {e["properties"]["action"]["const"]: e for e in schema["oneOf"]}
    assert "sql" in entries["query"]["properties"]
    assert "table" in entries["insert"]["properties"]
    assert "data" in entries["insert"]["properties"]


def test_separate_strategy_schema():
    @tool_group(name="files", description="File ops", strategy="separate")
    class FileGroup:
        @action("read", description="Read a file")
        def read(self, path: str) -> str:
            return path

        @action("write", description="Write a file")
        def write(self, path: str, content: str) -> str:
            return "ok"

    defs = groups_to_tool_defs()
    assert len(defs) == 2
    names = [d.name for d in defs]
    assert "files.read" in names
    assert "files.write" in names
    read_def = next(d for d in defs if d.name == "files.read")
    read_schema = json.loads(read_def.input_schema_json)
    assert "path" in read_schema["properties"]
    assert "action" not in read_schema.get("properties", {})


def test_dispatch_correct_action():
    @tool_group(name="calc", description="Calculator")
    class CalcGroup:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> int:
            return a + b

    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="add", a=3, b=4)
    assert result == 7


def test_dispatch_unknown_action():
    @tool_group(name="calc2", description="Calculator")
    class CalcGroup2:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> int:
            return a + b

    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="ad")
    assert result.is_error
    assert "Unknown action" in result.result
    assert "add" in result.result


def test_dispatch_missing_action():
    @tool_group(name="calc3", description="Calculator")
    class CalcGroup3:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> int:
            return a + b

    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0])
    assert result.is_error
    assert "Missing" in result.result


def test_groups_in_get_registered_tools():
    @tool_group(name="tools_test", description="Test group")
    class ToolsTestGroup:
        @action("ping", description="Ping")
        def ping(self) -> str:
            return "pong"

    tools = get_registered_tools()
    names = [t.name for t in tools]
    assert "tools_test" in names


def test_union_handler_dispatch():
    @tool_group(name="handler_test", description="Handler test")
    class HandlerTestGroup:
        @action("greet", description="Greet")
        def greet(self, name: str) -> str:
            return f"Hello, {name}!"

    defs = groups_to_tool_defs()
    td = defs[0]
    result = td.handler(action="greet", name="World")
    assert result == "Hello, World!"


def test_separate_handler_dispatch():
    @tool_group(name="sep_test", description="Sep test", strategy="separate")
    class SepTestGroup:
        @action("echo", description="Echo")
        def echo(self, msg: str) -> str:
            return msg

    defs = groups_to_tool_defs()
    td = defs[0]
    result = td.handler(msg="hi")
    assert result == "hi"


def test_group_metadata_passthrough():
    @tool_group(name="meta", description="Meta test", destructive=True, hidden=True)
    class MetaGroup:
        @action("noop", description="No-op")
        def noop(self) -> str:
            return ""

    defs = groups_to_tool_defs()
    td = defs[0]
    assert td.destructive_hint is True
    assert td.hidden is True
