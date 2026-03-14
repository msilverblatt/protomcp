import inspect
import json
import types
import typing
from dataclasses import dataclass
from typing import Any, Callable, get_type_hints, get_origin, get_args

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
    from protomcp.group import groups_to_tool_defs
    from protomcp.workflow import workflows_to_tool_defs
    return list(_registry) + groups_to_tool_defs() + workflows_to_tool_defs()

def get_hidden_tool_names() -> list[str]:
    return [t.name for t in _registry if t.hidden]

def clear_registry():
    _registry.clear()

_PRIMITIVE_TYPE_MAP = {
    str: "string",
    int: "integer",
    float: "number",
    bool: "boolean",
    list: "array",
    dict: "object",
}

def _type_to_schema(hint) -> dict:
    origin = get_origin(hint)
    args = get_args(hint)
    if origin is typing.Union or origin is types.UnionType:
        non_none = [a for a in args if a is not type(None)]
        schemas = [_type_to_schema(a) for a in non_none]
        if type(None) in args:
            schemas.append({"type": "null"})
        if len(schemas) == 1:
            return schemas[0]
        return {"anyOf": schemas}
    if origin is list:
        if args:
            return {"type": "array", "items": _type_to_schema(args[0])}
        return {"type": "array"}
    if origin is dict:
        schema: dict[str, Any] = {"type": "object"}
        if len(args) == 2:
            schema["additionalProperties"] = _type_to_schema(args[1])
        return schema
    if origin is typing.Literal:
        value_types = set(type(v).__name__ for v in args)
        if len(value_types) == 1:
            t = type(args[0])
            json_type = _PRIMITIVE_TYPE_MAP.get(t)
            if json_type:
                return {"type": json_type, "enum": list(args)}
        return {"enum": list(args)}
    json_type = _PRIMITIVE_TYPE_MAP.get(hint)
    if json_type:
        return {"type": json_type}
    return {"type": "string"}

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
        prop = _type_to_schema(hint)
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
    origin = get_origin(hint)
    if origin is typing.Union or origin is types.UnionType:
        return type(None) in get_args(hint)
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
        prop = _type_to_schema(hint)
        if field.default is not dataclasses.MISSING or field.default_factory is not dataclasses.MISSING:  # type: ignore[misc]
            pass  # optional field
        elif not _is_optional_type(hint):
            required.append(field.name)
        properties[field.name] = prop
    schema: dict[str, Any] = {"type": "object", "properties": properties}
    if required:
        schema["required"] = required
    return schema
