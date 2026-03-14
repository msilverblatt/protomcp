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
    output_schema_json: str = ""
    title: str = ""
    destructive_hint: bool = False
    idempotent_hint: bool = False
    read_only_hint: bool = False
    open_world_hint: bool = False
    task_support: bool = False
    hidden: bool = False

def tool(
    description: str,
    output_type=None,
    title: str = "",
    destructive: bool = False,
    idempotent: bool = False,
    read_only: bool = False,
    open_world: bool = False,
    task_support: bool = False,
    hidden: bool = False,
):
    def decorator(func: Callable) -> Callable:
        schema = _generate_schema(func)
        output_schema = _generate_dataclass_schema(output_type) if output_type is not None else {}
        _registry.append(ToolDef(
            name=func.__name__,
            description=description,
            input_schema_json=json.dumps(schema),
            handler=func,
            output_schema_json=json.dumps(output_schema) if output_schema else "",
            title=title,
            destructive_hint=destructive,
            idempotent_hint=idempotent,
            read_only_hint=read_only,
            open_world_hint=open_world,
            task_support=task_support,
            hidden=hidden,
        ))
        return func
    return decorator

def get_registered_tools() -> list[ToolDef]:
    return list(_registry)

def get_hidden_tool_names() -> list[str]:
    return [t.name for t in _registry if t.hidden]

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

def _generate_dataclass_schema(dataclass_type) -> dict:
    import dataclasses
    if not dataclasses.is_dataclass(dataclass_type):
        return {}
    hints = get_type_hints(dataclass_type)
    fields = dataclasses.fields(dataclass_type)
    properties = {}
    required = []
    for field in fields:
        hint = hints.get(field.name, Any)
        json_type = _python_type_to_json(hint)
        prop: dict[str, Any] = {"type": json_type}
        if field.default is not dataclasses.MISSING or field.default_factory is not dataclasses.MISSING:  # type: ignore[misc]
            pass  # optional field
        elif not _is_optional_type(hint):
            required.append(field.name)
        properties[field.name] = prop
    schema: dict[str, Any] = {"type": "object", "properties": properties}
    if required:
        schema["required"] = required
    return schema

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
