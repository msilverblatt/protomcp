import fnmatch
import inspect
import json
from dataclasses import dataclass, field
from typing import Any, Callable

from protomcp.tool import ToolDef, _type_to_schema, _is_optional_type, get_registered_tools
from protomcp.result import ToolResult

_workflow_registry: list["WorkflowDef"] = []
_active_workflow_stack: list["WorkflowState"] = []


@dataclass
class StepResult:
    result: str = ""
    next: list[str] | None = None


@dataclass
class StepDef:
    name: str
    description: str
    handler: Callable
    input_schema: dict
    initial: bool = False
    next: list[str] | None = None
    terminal: bool = False
    no_cancel: bool = False
    allow_during: list[str] | None = None
    block_during: list[str] | None = None
    on_error: dict[type, str] | None = None
    requires: list[str] | None = None
    enum_fields: dict[str, list] | None = None


@dataclass
class WorkflowDef:
    name: str
    description: str
    steps: list[StepDef]
    instance: Any
    allow_during: list[str] | None = None
    block_during: list[str] | None = None
    on_cancel: Callable | None = None
    on_complete: Callable | None = None


@dataclass
class WorkflowState:
    workflow_name: str
    current_step: str
    history: list[tuple[str, StepResult]]
    pre_workflow_tools: list[str]
    instance: Any


def step(
    name: str | None = None,
    description: str = "",
    initial: bool = False,
    next: list[str] | None = None,
    terminal: bool = False,
    no_cancel: bool = False,
    allow_during: list[str] | None = None,
    block_during: list[str] | None = None,
    on_error: dict[type, str] | None = None,
    requires: list[str] | None = None,
    enum_fields: dict[str, list] | None = None,
):
    """Decorator that marks a method as a workflow step.

    If ``name`` is not provided, the decorated function's name is used.
    """
    def decorator(func: Callable) -> Callable:
        step_name = name if name is not None else func.__name__
        func._step_def = {  # type: ignore[attr-defined]
            "name": step_name,
            "description": description,
            "initial": initial,
            "next": next,
            "terminal": terminal,
            "no_cancel": no_cancel,
            "allow_during": allow_during,
            "block_during": block_during,
            "on_error": on_error,
            "requires": requires,
            "enum_fields": enum_fields,
        }
        return func
    return decorator


def _generate_step_schema(method: Callable) -> dict:
    """Generate JSON Schema for a step method, skipping self/cls/ctx/ToolContext params."""
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


def _validate_workflow_graph(workflow_def: WorkflowDef) -> None:
    """Validate workflow graph structure. Raises ValueError on problems."""
    step_names = {s.name for s in workflow_def.steps}

    # Check initial steps
    initial_steps = [s for s in workflow_def.steps if s.initial]
    if len(initial_steps) == 0:
        raise ValueError(f"Workflow '{workflow_def.name}': no initial step defined")
    if len(initial_steps) > 1:
        names = [s.name for s in initial_steps]
        raise ValueError(f"Workflow '{workflow_def.name}': multiple initial steps: {names}")

    for s in workflow_def.steps:
        # Terminal step must not have next
        if s.terminal and s.next is not None:
            raise ValueError(
                f"Workflow '{workflow_def.name}': terminal step '{s.name}' has next"
            )

        # Non-terminal step must have next
        if not s.terminal and s.next is None:
            raise ValueError(
                f"Workflow '{workflow_def.name}': non-terminal step '{s.name}' has no next (dead end)"
            )

        # next references must exist
        if s.next is not None:
            for ref in s.next:
                if ref not in step_names:
                    raise ValueError(
                        f"Workflow '{workflow_def.name}': step '{s.name}' references nonexistent step '{ref}'"
                    )

        # on_error targets must exist
        if s.on_error is not None:
            for exc_type, target in s.on_error.items():
                if target not in step_names:
                    raise ValueError(
                        f"Workflow '{workflow_def.name}': step '{s.name}' on_error references nonexistent step '{target}'"
                    )


def workflow(
    name: str,
    description: str = "",
    allow_during: list[str] | None = None,
    block_during: list[str] | None = None,
):
    """Class decorator that registers a workflow."""
    def decorator(cls):
        instance = cls()
        steps: list[StepDef] = []
        for attr_name in dir(instance):
            method = getattr(instance, attr_name, None)
            if method is None:
                continue
            step_info = getattr(method, "_step_def", None)
            if step_info is None:
                continue
            schema = _generate_step_schema(method)
            steps.append(StepDef(
                name=step_info["name"],
                description=step_info["description"],
                handler=method,
                input_schema=schema,
                initial=step_info["initial"],
                next=step_info["next"],
                terminal=step_info["terminal"],
                no_cancel=step_info["no_cancel"],
                allow_during=step_info["allow_during"],
                block_during=step_info["block_during"],
                on_error=step_info["on_error"],
                requires=step_info["requires"],
                enum_fields=step_info["enum_fields"],
            ))

        on_cancel = getattr(instance, "on_cancel", None)
        if on_cancel is not None and not callable(on_cancel):
            on_cancel = None

        on_complete = getattr(instance, "on_complete", None)
        if on_complete is not None and not callable(on_complete):
            on_complete = None

        wf = WorkflowDef(
            name=name,
            description=description,
            steps=steps,
            instance=instance,
            allow_during=allow_during,
            block_during=block_during,
            on_cancel=on_cancel,
            on_complete=on_complete,
        )

        _validate_workflow_graph(wf)
        _workflow_registry.append(wf)
        return cls
    return decorator


def _matches_visibility(tool_name: str, allow_during: list[str] | None, block_during: list[str] | None) -> bool:
    """Check if a tool name matches visibility filters using fnmatch glob patterns.

    If neither allow nor block is specified, returns False (fully exclusive).
    If allow is specified, tool must match at least one allow pattern.
    If block is specified, tool must not match any block pattern.
    Allow is checked first, then block filters the result.
    """
    if allow_during is None and block_during is None:
        return False

    if allow_during is not None:
        allowed = any(fnmatch.fnmatch(tool_name, pat) for pat in allow_during)
        if not allowed:
            return False

    if block_during is not None:
        blocked = any(fnmatch.fnmatch(tool_name, pat) for pat in block_during)
        if blocked:
            return False

    return True


def _get_step_visibility(step_def: StepDef, workflow_def: WorkflowDef) -> tuple[list[str] | None, list[str] | None]:
    """Get effective visibility for a step. Step-level overrides workflow-level."""
    if step_def.allow_during is not None or step_def.block_during is not None:
        return step_def.allow_during, step_def.block_during
    return workflow_def.allow_during, workflow_def.block_during


def _compute_transition(
    workflow_def: WorkflowDef,
    state: WorkflowState,
    next_step_names: list[str],
) -> tuple[list[str], list[str]]:
    """Compute enable/disable tool lists for a step transition.

    Returns (enable_tools, disable_tools) to put on the ToolResult.
    This avoids calling tool_manager during a tool call handler (which deadlocks).
    """
    step_map = {s.name: s for s in workflow_def.steps}
    all_tools = [t.name for t in get_registered_tools()]

    # Tools that should be visible after transition
    allowed: set[str] = set()

    # Add the next step tools
    for sn in next_step_names:
        allowed.add(f"{workflow_def.name}.{sn}")

    # Add cancel tool if any next step allows cancel
    any_cancelable = any(not step_map[sn].no_cancel for sn in next_step_names if sn in step_map)
    if any_cancelable:
        allowed.add(f"{workflow_def.name}.cancel")

    # Add visibility-matched non-workflow tools
    if next_step_names:
        first_step = step_map.get(next_step_names[0])
        if first_step:
            allow_during, block_during = _get_step_visibility(first_step, workflow_def)
            for tool_name in state.pre_workflow_tools:
                if _matches_visibility(tool_name, allow_during, block_during):
                    allowed.add(tool_name)

    # Compute diff: enable what should be visible, disable what shouldn't
    all_set = set(all_tools)
    enable = sorted(allowed)
    disable = sorted(all_set - allowed)

    return enable, disable


def _find_workflow(name: str) -> WorkflowDef | None:
    for wf in _workflow_registry:
        if wf.name == name:
            return wf
    return None


def _find_step(workflow_def: WorkflowDef, step_name: str) -> StepDef | None:
    for s in workflow_def.steps:
        if s.name == step_name:
            return s
    return None


def _get_active_state() -> WorkflowState | None:
    if _active_workflow_stack:
        return _active_workflow_stack[-1]
    return None


def _handle_step_call(workflow_name: str, step_name: str, kwargs: dict) -> Any:
    """Handle a step tool call."""
    wf = _find_workflow(workflow_name)
    if wf is None:
        return ToolResult(result=f"Unknown workflow: {workflow_name}", is_error=True)

    step_def = _find_step(wf, step_name)
    if step_def is None:
        return ToolResult(result=f"Unknown step: {step_name}", is_error=True)

    state = _get_active_state()

    if step_def.initial:
        # Start a new workflow — snapshot all registered tool names (no transport roundtrip)
        all_tool_names = [t.name for t in get_registered_tools()]
        # Pre-workflow tools are everything that's not part of this workflow
        pre_tools = [t for t in all_tool_names if not t.startswith(f"{workflow_name}.")]
        state = WorkflowState(
            workflow_name=workflow_name,
            current_step=step_name,
            history=[],
            pre_workflow_tools=pre_tools,
            instance=wf.instance,
        )
        _active_workflow_stack.append(state)
    else:
        # Must have an active workflow
        if state is None or state.workflow_name != workflow_name:
            return ToolResult(
                result=f"No active workflow '{workflow_name}' to continue",
                is_error=True,
            )
        state.current_step = step_name

    # Run the handler
    try:
        result = step_def.handler(**kwargs)
    except Exception as exc:
        # Check on_error mapping
        if step_def.on_error is not None:
            for exc_type, target_step in step_def.on_error.items():
                if isinstance(exc, exc_type):
                    state.current_step = target_step
                    # Transition to the error target step
                    enable, disable = _compute_transition(wf, state, [target_step])
                    return ToolResult(
                        result=f"Error caught ({type(exc).__name__}: {exc}), transitioning to '{target_step}'",
                        enable_tools=enable,
                        disable_tools=disable,
                    )
        # No matching on_error: stay in current state for retry
        return ToolResult(
            result=f"Step '{step_name}' failed: {type(exc).__name__}: {exc}. You can retry.",
            is_error=True,
        )

    # Normalize result
    if not isinstance(result, StepResult):
        result = StepResult(result=str(result) if result is not None else "")

    # Validate dynamic next
    if result.next is not None:
        if step_def.next is None:
            return ToolResult(
                result=f"Step '{step_name}' returned next={result.next} but has no declared next",
                is_error=True,
            )
        if not set(result.next).issubset(set(step_def.next)):
            invalid = set(result.next) - set(step_def.next)
            return ToolResult(
                result=f"Step '{step_name}' returned invalid next steps: {invalid}. Declared: {step_def.next}",
                is_error=True,
            )

    # Record in history
    state.history.append((step_name, result))

    # Determine effective next
    effective_next = result.next if result.next is not None else step_def.next

    if step_def.terminal:
        # Workflow complete
        if wf.on_complete is not None:
            wf.on_complete()
        # Restore pre-workflow tools via enable/disable
        all_tool_names = set(t.name for t in get_registered_tools())
        restore_enable = sorted(set(state.pre_workflow_tools))
        restore_disable = sorted(all_tool_names - set(state.pre_workflow_tools))
        _active_workflow_stack.pop()
        return ToolResult(
            result=result.result or "Workflow complete",
            enable_tools=restore_enable,
            disable_tools=restore_disable,
        )
    else:
        # Transition to next steps
        enable, disable = _compute_transition(wf, state, effective_next or [])
        return ToolResult(
            result=result.result or f"Proceed to: {effective_next}",
            enable_tools=enable,
            disable_tools=disable,
        )


def _handle_cancel(workflow_name: str) -> Any:
    """Handle a cancel tool call."""
    state = _get_active_state()
    if state is None or state.workflow_name != workflow_name:
        return ToolResult(
            result=f"No active workflow '{workflow_name}' to cancel",
            is_error=True,
        )

    wf = _find_workflow(workflow_name)
    if wf is None:
        return ToolResult(result=f"Unknown workflow: {workflow_name}", is_error=True)

    if wf.on_cancel is not None:
        wf.on_cancel()

    # Restore pre-workflow tools via enable/disable
    all_tool_names = set(t.name for t in get_registered_tools())
    restore_enable = sorted(set(state.pre_workflow_tools))
    restore_disable = sorted(all_tool_names - set(state.pre_workflow_tools))
    _active_workflow_stack.pop()
    return ToolResult(
        result=f"Workflow '{workflow_name}' cancelled",
        enable_tools=restore_enable,
        disable_tools=restore_disable,
    )


def workflows_to_tool_defs() -> list[ToolDef]:
    """Convert registered workflows into ToolDef list."""
    defs: list[ToolDef] = []
    for wf in _workflow_registry:
        step_map = {s.name: s for s in wf.steps}
        has_cancelable = any(not s.no_cancel for s in wf.steps if not s.terminal)

        for s in wf.steps:
            tool_name = f"{wf.name}.{s.name}"

            def make_handler(wf_name=wf.name, sname=s.name):
                def handler(**kwargs):
                    return _handle_step_call(wf_name, sname, kwargs)
                return handler

            defs.append(ToolDef(
                name=tool_name,
                description=s.description or f"{wf.name} {s.name}",
                input_schema_json=json.dumps(s.input_schema),
                handler=make_handler(),
                hidden=not s.initial,
            ))

        if has_cancelable:
            cancel_name = f"{wf.name}.cancel"

            def make_cancel_handler(wf_name=wf.name):
                def handler(**kwargs):
                    return _handle_cancel(wf_name)
                return handler

            defs.append(ToolDef(
                name=cancel_name,
                description=f"Cancel the {wf.name} workflow",
                input_schema_json=json.dumps({"type": "object", "properties": {}}),
                handler=make_cancel_handler(),
                hidden=True,
            ))

    return defs


def get_registered_workflows() -> list[WorkflowDef]:
    return list(_workflow_registry)


def clear_workflow_registry():
    _workflow_registry.clear()
    _active_workflow_stack.clear()
