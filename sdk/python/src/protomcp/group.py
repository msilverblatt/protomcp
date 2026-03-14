import difflib
import inspect
import json
from dataclasses import dataclass, field
from typing import Any, Callable

from protomcp.tool import ToolDef, _type_to_schema, _is_optional_type
from protomcp.result import ToolResult

_group_registry: list["GroupDef"] = []


@dataclass
class ActionDef:
    name: str
    description: str
    handler: Callable
    input_schema: dict
    requires: list[str] = field(default_factory=list)
    enum_fields: dict[str, list] = field(default_factory=dict)
    cross_rules: list[tuple[Callable, str]] = field(default_factory=list)
    hints: dict[str, dict] = field(default_factory=dict)


@dataclass
class GroupDef:
    name: str
    description: str
    actions: list[ActionDef]
    instance: Any
    strategy: str = "union"
    title: str = ""
    destructive_hint: bool = False
    idempotent_hint: bool = False
    read_only_hint: bool = False
    open_world_hint: bool = False
    task_support: bool = False
    hidden: bool = False


def action(name: str, description: str = "", requires=None, enum_fields=None, cross_rules=None, hints=None):
    """Decorator that marks a method as a group action."""
    def decorator(func: Callable) -> Callable:
        func._action_def = {  # type: ignore[attr-defined]
            "name": name,
            "description": description,
            "requires": requires or [],
            "enum_fields": enum_fields or {},
            "cross_rules": cross_rules or [],
            "hints": hints or {},
        }
        return func
    return decorator


def _generate_action_schema(method: Callable) -> dict:
    """Generate JSON Schema for a method, skipping self/cls/ctx/ToolContext params."""
    from typing import get_type_hints, get_origin, get_args
    hints = get_type_hints(method)
    sig = inspect.signature(method)
    properties: dict[str, Any] = {}
    required: list[str] = []
    for param_name, param in sig.parameters.items():
        if param_name in ("self", "cls", "ctx"):
            continue
        hint = hints.get(param_name, Any)
        if hasattr(hint, "__name__") and hint.__name__ == "ToolContext":
            continue
        prop = _type_to_schema(hint)
        if param.default is not inspect.Parameter.empty:
            prop["default"] = param.default
        elif not _is_optional_type(hint):
            required.append(param_name)
        properties[param_name] = prop
    schema: dict[str, Any] = {"type": "object", "properties": properties}
    if required:
        schema["required"] = required
    return schema


def tool_group(
    name: str,
    description: str = "",
    strategy: str = "union",
    title: str = "",
    destructive: bool = False,
    idempotent: bool = False,
    read_only: bool = False,
    open_world: bool = False,
    task_support: bool = False,
    hidden: bool = False,
):
    """Class decorator that registers a tool group."""
    def decorator(cls):
        instance = cls()
        actions: list[ActionDef] = []
        for attr_name in dir(instance):
            method = getattr(instance, attr_name, None)
            if method is None:
                continue
            action_info = getattr(method, "_action_def", None)
            if action_info is None:
                continue
            schema = _generate_action_schema(method)
            actions.append(ActionDef(
                name=action_info["name"],
                description=action_info["description"],
                handler=method,
                input_schema=schema,
                requires=action_info["requires"],
                enum_fields=action_info["enum_fields"],
                cross_rules=action_info["cross_rules"],
                hints=action_info["hints"],
            ))
        group = GroupDef(
            name=name,
            description=description,
            actions=actions,
            instance=instance,
            strategy=strategy,
            title=title,
            destructive_hint=destructive,
            idempotent_hint=idempotent,
            read_only_hint=read_only,
            open_world_hint=open_world,
            task_support=task_support,
            hidden=hidden,
        )
        _group_registry.append(group)
        return cls
    return decorator


def _validate_action(action_def: ActionDef, kwargs: dict) -> ToolResult | None:
    """Validate action parameters against declarative rules. Returns ToolResult error or None."""
    # Check requires
    for field_name in action_def.requires:
        val = kwargs.get(field_name)
        if val is None or val == "":
            return ToolResult(
                result=f"Missing required field: {field_name}",
                is_error=True,
                error_code="MISSING_REQUIRED",
                message=f"Missing required field: {field_name}",
            )

    # Check enum_fields
    for field_name, valid in action_def.enum_fields.items():
        val = kwargs.get(field_name)
        if val is not None and val not in valid:
            matches = difflib.get_close_matches(str(val), [str(v) for v in valid], n=1, cutoff=0.6)
            suggestion = f"Did you mean '{matches[0]}'?" if matches else None
            return ToolResult(
                result=f"Invalid value '{val}' for field '{field_name}'. Valid options: {', '.join(str(v) for v in valid)}",
                is_error=True,
                error_code="INVALID_ENUM",
                message=f"Invalid value '{val}' for field '{field_name}'.",
                suggestion=suggestion,
            )

    # Check cross_rules
    for condition_fn, error_message in action_def.cross_rules:
        if condition_fn(kwargs):
            return ToolResult(
                result=error_message,
                is_error=True,
                error_code="CROSS_PARAM_VIOLATION",
                message=error_message,
            )

    return None


def _collect_hints(action_def: ActionDef, kwargs: dict) -> list[str]:
    """Collect advisory hint messages whose conditions are met."""
    messages = []
    for hint_info in action_def.hints.values():
        if hint_info["condition"](kwargs):
            messages.append(hint_info["message"])
    return messages


def _dispatch_group_action(group: GroupDef, **kwargs) -> Any:
    """Dispatch to the correct action handler within a group."""
    action_name = kwargs.pop("action", None)
    action_names = [a.name for a in group.actions]

    if action_name is None:
        return ToolResult(
            result=f"Missing 'action' field. Available actions: {', '.join(action_names)}",
            is_error=True,
            error_code="MISSING_ACTION",
        )

    for act in group.actions:
        if act.name == action_name:
            # Resolve server contexts
            from protomcp.server_context import resolve_contexts
            ctx_values = resolve_contexts(kwargs)
            sig = inspect.signature(act.handler)
            for param_name, value in ctx_values.items():
                if param_name in sig.parameters:
                    kwargs[param_name] = value

            # Validate before calling handler
            validation_error = _validate_action(act, kwargs)
            if validation_error is not None:
                return validation_error

            # Collect hints
            hints = _collect_hints(act, kwargs)

            # Call the handler
            result = act.handler(**kwargs)

            # Append hints if any
            if hints:
                hint_text = "\n\n**Hints:**\n" + "\n".join(f"- {m}" for m in hints)
                if isinstance(result, ToolResult):
                    return ToolResult(
                        result=result.result + hint_text,
                        is_error=result.is_error,
                        enable_tools=result.enable_tools,
                        disable_tools=result.disable_tools,
                        error_code=result.error_code,
                        message=result.message,
                        suggestion=result.suggestion,
                        retryable=result.retryable,
                    )
                else:
                    return ToolResult(result=str(result) + hint_text)

            return result

    matches = difflib.get_close_matches(action_name, action_names, n=1, cutoff=0.4)
    suggestion = f" Did you mean '{matches[0]}'?" if matches else ""
    return ToolResult(
        result=f"Unknown action '{action_name}'.{suggestion} Available actions: {', '.join(action_names)}",
        is_error=True,
        error_code="UNKNOWN_ACTION",
    )


def groups_to_tool_defs() -> list[ToolDef]:
    """Convert registered groups into ToolDef list."""
    defs: list[ToolDef] = []
    for group in _group_registry:
        if group.strategy == "separate":
            defs.extend(_group_to_separate_defs(group))
        else:
            defs.append(_group_to_union_def(group))
    return defs


def _group_to_union_def(group: GroupDef) -> ToolDef:
    """Create a single ToolDef with discriminated union schema."""
    action_names = [a.name for a in group.actions]
    one_of = []
    for act in group.actions:
        entry: dict[str, Any] = {
            "type": "object",
            "properties": {
                "action": {"const": act.name},
                **act.input_schema.get("properties", {}),
            },
            "required": ["action"] + act.input_schema.get("required", []),
        }
        one_of.append(entry)

    schema: dict[str, Any] = {
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": action_names,
            },
        },
        "required": ["action"],
        "oneOf": one_of,
    }

    action_list = ", ".join(action_names)
    desc = group.description
    if desc:
        desc += f" Actions: {action_list}"
    else:
        desc = f"Actions: {action_list}"

    def handler(**kwargs):
        return _dispatch_group_action(group, **kwargs)

    return ToolDef(
        name=group.name,
        description=desc,
        input_schema_json=json.dumps(schema),
        handler=handler,
        title=group.title,
        destructive_hint=group.destructive_hint,
        idempotent_hint=group.idempotent_hint,
        read_only_hint=group.read_only_hint,
        open_world_hint=group.open_world_hint,
        task_support=group.task_support,
        hidden=group.hidden,
    )


def _group_to_separate_defs(group: GroupDef) -> list[ToolDef]:
    """Create separate ToolDefs for each action."""
    defs: list[ToolDef] = []
    for act in group.actions:
        def make_handler(action_def=act):
            def handler(**kwargs):
                return action_def.handler(**kwargs)
            return handler

        defs.append(ToolDef(
            name=f"{group.name}.{act.name}",
            description=act.description or f"{group.name} {act.name}",
            input_schema_json=json.dumps(act.input_schema),
            handler=make_handler(),
            title=group.title,
            destructive_hint=group.destructive_hint,
            idempotent_hint=group.idempotent_hint,
            read_only_hint=group.read_only_hint,
            open_world_hint=group.open_world_hint,
            task_support=group.task_support,
            hidden=group.hidden,
        ))
    return defs


def get_registered_groups() -> list[GroupDef]:
    return list(_group_registry)


def clear_group_registry():
    _group_registry.clear()
