import inspect
import json
import typing
from dataclasses import dataclass
from typing import Any, Callable, get_type_hints

_registry: list["ToolDef"] = []

@dataclass
class ToolDef:
    name: str
    description: str
    input_schema_json: str
    handler: Callable

def tool(description: str):
    def decorator(func: Callable) -> Callable:
        schema = _generate_schema(func)
        _registry.append(ToolDef(
            name=func.__name__,
            description=description,
            input_schema_json=json.dumps(schema),
            handler=func,
        ))
        return func
    return decorator

def get_registered_tools() -> list[ToolDef]:
    return list(_registry)

def clear_registry():
    _registry.clear()

_PYTHON_TYPE_TO_JSON_SCHEMA = {
    str: "string",
    int: "integer",
    float: "number",
    bool: "boolean",
    list: "array",
    dict: "object",
}

def _generate_schema(func: Callable) -> dict:
    hints = get_type_hints(func)
    sig = inspect.signature(func)
    properties = {}
    required = []
    for name, param in sig.parameters.items():
        if name in ("self", "cls", "ctx"):
            continue
        hint = hints.get(name, Any)
        if hasattr(hint, "__name__") and hint.__name__ == "ToolContext":
            continue
        json_type = _python_type_to_json(hint)
        prop: dict[str, Any] = {"type": json_type}
        if param.default is not inspect.Parameter.empty:
            prop["default"] = param.default
        elif not _is_optional_type(hint):
            required.append(name)
        properties[name] = prop
    schema: dict[str, Any] = {"type": "object", "properties": properties}
    if required:
        schema["required"] = required
    return schema

def _is_optional_type(hint) -> bool:
    origin = getattr(hint, "__origin__", None)
    if origin is typing.Union:
        return type(None) in hint.__args__
    return False

def _python_type_to_json(hint) -> str:
    origin = getattr(hint, "__origin__", None)
    if origin is type(None):
        return "null"
    if origin is typing.Union:
        args = hint.__args__
        non_none = [a for a in args if a is not type(None)]
        if len(non_none) == 1:
            return _python_type_to_json(non_none[0])
    return _PYTHON_TYPE_TO_JSON_SCHEMA.get(hint, "string")
