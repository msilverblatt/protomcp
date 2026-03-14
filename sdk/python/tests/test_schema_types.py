import json
from typing import Any, Optional, Literal
from protomcp.tool import tool, get_registered_tools, clear_registry, _type_to_schema


# --- Unit tests for _type_to_schema ---

class TestTypeToSchema:
    def test_str(self):
        assert _type_to_schema(str) == {"type": "string"}

    def test_int(self):
        assert _type_to_schema(int) == {"type": "integer"}

    def test_float(self):
        assert _type_to_schema(float) == {"type": "number"}

    def test_bool(self):
        assert _type_to_schema(bool) == {"type": "boolean"}

    def test_list(self):
        assert _type_to_schema(list) == {"type": "array"}

    def test_dict(self):
        assert _type_to_schema(dict) == {"type": "object"}

    def test_list_str(self):
        assert _type_to_schema(list[str]) == {"type": "array", "items": {"type": "string"}}

    def test_list_int(self):
        assert _type_to_schema(list[int]) == {"type": "array", "items": {"type": "integer"}}

    def test_list_dict(self):
        assert _type_to_schema(list[dict]) == {"type": "array", "items": {"type": "object"}}

    def test_dict_str_any(self):
        assert _type_to_schema(dict[str, Any]) == {"type": "object", "additionalProperties": {"type": "string"}}

    def test_dict_str_str(self):
        assert _type_to_schema(dict[str, str]) == {"type": "object", "additionalProperties": {"type": "string"}}

    def test_optional_str(self):
        result = _type_to_schema(Optional[str])
        assert result == {"anyOf": [{"type": "string"}, {"type": "null"}]}

    def test_union_str_dict(self):
        result = _type_to_schema(str | dict)
        assert result == {"anyOf": [{"type": "string"}, {"type": "object"}]}

    def test_union_str_dict_none(self):
        result = _type_to_schema(str | dict | None)
        assert result == {"anyOf": [{"type": "string"}, {"type": "object"}, {"type": "null"}]}

    def test_union_list_str_list_dict_none(self):
        result = _type_to_schema(list[str] | list[dict] | None)
        assert result == {
            "anyOf": [
                {"type": "array", "items": {"type": "string"}},
                {"type": "array", "items": {"type": "object"}},
                {"type": "null"},
            ]
        }

    def test_literal_strings(self):
        result = _type_to_schema(Literal["a", "b", "c"])
        assert result == {"type": "string", "enum": ["a", "b", "c"]}

    def test_literal_ints(self):
        result = _type_to_schema(Literal[1, 2, 3])
        assert result == {"type": "integer", "enum": [1, 2, 3]}

    def test_list_list_str(self):
        result = _type_to_schema(list[list[str]])
        assert result == {
            "type": "array",
            "items": {"type": "array", "items": {"type": "string"}},
        }

    def test_unknown_type(self):
        class CustomClass:
            pass
        assert _type_to_schema(CustomClass) == {"type": "string"}


# --- Integration tests ---

def test_tool_with_union_params():
    clear_registry()

    @tool(description="Process input")
    def process(data: str | dict) -> str:
        return "done"

    tools = get_registered_tools()
    schema = json.loads(tools[0].input_schema_json)
    prop = schema["properties"]["data"]
    assert "anyOf" in prop
    assert {"type": "string"} in prop["anyOf"]
    assert {"type": "object"} in prop["anyOf"]


def test_tool_with_literal_param():
    clear_registry()

    @tool(description="Set mode")
    def set_mode(mode: Literal["fast", "slow", "balanced"]) -> str:
        return mode

    tools = get_registered_tools()
    schema = json.loads(tools[0].input_schema_json)
    prop = schema["properties"]["mode"]
    assert prop["type"] == "string"
    assert prop["enum"] == ["fast", "slow", "balanced"]
