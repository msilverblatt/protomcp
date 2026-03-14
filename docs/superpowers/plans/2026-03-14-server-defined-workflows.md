# Server-Defined Workflows Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `@workflow` and `@step` decorators that let Python MCP servers define multi-step processes as state machines, where the visible tool surface is the state.

**Architecture:** A single new module `workflow.py` that generates tool defs (via the existing tool group separate strategy pattern), manages workflow state, and controls tool visibility via `tool_manager.set_allowed()`. No changes to runner.py, proto, or the Go bridge. Builds entirely on existing primitives.

**Tech Stack:** Python 3.10+, existing protomcp primitives (`tool_manager`, `ToolDef`, `ToolResult`), `fnmatch` for glob patterns.

**Spec:** `docs/superpowers/specs/2026-03-14-server-defined-workflows-design.md`

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `sdk/python/src/protomcp/workflow.py` | `@workflow`, `@step`, `StepResult`, `WorkflowDef`, `StepDef`, `WorkflowState`, graph validation, tool visibility computation, step dispatch |
| `sdk/python/tests/test_workflow.py` | Unit tests for workflow registration, graph validation, step dispatch, visibility |
| `sdk/python/tests/test_workflow_errors.py` | Tests for error handling — stay in state, `on_error` transitions, no_cancel + error |
| `sdk/python/tests/test_workflow_visibility.py` | Tests for tool visibility — allow_during, block_during, step overrides, exclusive mode |
| `examples/python/workflow_deploy.py` | Example: deployment pipeline workflow |

### Files to modify

| File | Changes |
|------|---------|
| `sdk/python/src/protomcp/__init__.py` | Export `workflow`, `step`, `StepResult` |
| `sdk/python/src/protomcp/tool.py` | Update `get_registered_tools()` to include workflow-generated tool defs |

---

## Chunk 1: Core Data Structures and Registration

### Task 1: StepResult, StepDef, WorkflowDef, WorkflowState data structures

**Files:**
- Create: `sdk/python/src/protomcp/workflow.py`
- Create: `sdk/python/tests/test_workflow.py`

- [ ] **Step 1: Write failing tests for data structures**

```python
# sdk/python/tests/test_workflow.py
from protomcp.workflow import (
    workflow, step, StepResult, StepDef, WorkflowDef, WorkflowState,
    get_registered_workflows, clear_workflow_registry,
)

def test_step_result_defaults():
    r = StepResult(result="done")
    assert r.result == "done"
    assert r.next is None

def test_step_result_with_next():
    r = StepResult(result="ok", next=["approve", "reject"])
    assert r.next == ["approve", "reject"]
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`
Expected: FAIL — module doesn't exist

- [ ] **Step 3: Implement data structures in workflow.py**

```python
# sdk/python/src/protomcp/workflow.py
import fnmatch
import inspect
from dataclasses import dataclass, field
from typing import Any, Callable

from protomcp.tool import ToolDef, _type_to_schema, _is_optional_type
from protomcp.result import ToolResult
from protomcp import manager as tool_manager


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


_workflow_registry: list[WorkflowDef] = []
_active_workflow_stack: list[WorkflowState] = []


def get_registered_workflows() -> list[WorkflowDef]:
    return list(_workflow_registry)


def clear_workflow_registry():
    _workflow_registry.clear()
    _active_workflow_stack.clear()
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add sdk/python/src/protomcp/workflow.py sdk/python/tests/test_workflow.py
git commit -m "feat(workflow): add core data structures — StepResult, StepDef, WorkflowDef, WorkflowState"
```

---

### Task 2: `@step` decorator and `@workflow` decorator with registration

**Files:**
- Modify: `sdk/python/src/protomcp/workflow.py`
- Modify: `sdk/python/tests/test_workflow.py`

- [ ] **Step 1: Write failing tests for decorators**

Add to `sdk/python/tests/test_workflow.py`:

```python
def test_register_workflow():
    clear_workflow_registry()

    @workflow("deploy", description="Deploy pipeline")
    class DeployWorkflow:
        @step(initial=True, next=["approve", "reject"])
        def review(self, pr_url: str) -> StepResult:
            return StepResult(result="reviewed")

        @step(next=["run_tests"])
        def approve(self, reason: str) -> StepResult:
            return StepResult(result="approved")

        @step(terminal=True)
        def reject(self, reason: str) -> StepResult:
            return StepResult(result="rejected")

        @step(terminal=True)
        def run_tests(self) -> StepResult:
            return StepResult(result="tests passed")

    wfs = get_registered_workflows()
    assert len(wfs) == 1
    assert wfs[0].name == "deploy"
    assert len(wfs[0].steps) == 4
    step_names = [s.name for s in wfs[0].steps]
    assert "review" in step_names
    assert "approve" in step_names

def test_step_marks_initial():
    clear_workflow_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self) -> StepResult:
            return StepResult(result="started")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    wfs = get_registered_workflows()
    initial = [s for s in wfs[0].steps if s.initial]
    assert len(initial) == 1
    assert initial[0].name == "start"

def test_step_schema_generation():
    clear_workflow_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self, name: str, count: int = 5) -> StepResult:
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    wfs = get_registered_workflows()
    start_step = [s for s in wfs[0].steps if s.name == "start"][0]
    assert "name" in start_step.input_schema["properties"]
    assert start_step.input_schema["properties"]["count"]["default"] == 5
    assert start_step.input_schema["required"] == ["name"]

def test_workflow_captures_on_cancel():
    clear_workflow_registry()

    @workflow("test")
    class W:
        @step(initial=True, terminal=True)
        def start(self) -> StepResult:
            return StepResult(result="done")

        def on_cancel(self, current_step, history):
            return "cancelled"

    wfs = get_registered_workflows()
    assert wfs[0].on_cancel is not None

def test_workflow_captures_on_complete():
    clear_workflow_registry()

    @workflow("test")
    class W:
        @step(initial=True, terminal=True)
        def start(self) -> StepResult:
            return StepResult(result="done")

        def on_complete(self, history):
            pass

    wfs = get_registered_workflows()
    assert wfs[0].on_complete is not None
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`

- [ ] **Step 3: Implement `@step` and `@workflow` decorators**

Add to `workflow.py`:

```python
def step(
    initial: bool = False,
    next: list[str] | None = None,
    terminal: bool = False,
    no_cancel: bool = False,
    description: str = "",
    allow_during: list[str] | None = None,
    block_during: list[str] | None = None,
    on_error: dict[type, str] | None = None,
    requires: list[str] | None = None,
    enum_fields: dict[str, list] | None = None,
):
    """Decorator that marks a method as a workflow step."""
    def decorator(func: Callable) -> Callable:
        func._step_def = {
            "name": func.__name__,
            "initial": initial,
            "next": next,
            "terminal": terminal,
            "no_cancel": no_cancel,
            "description": description,
            "allow_during": allow_during,
            "block_during": block_during,
            "on_error": on_error,
            "requires": requires,
            "enum_fields": enum_fields,
        }
        return func
    return decorator


def _generate_step_schema(method: Callable) -> dict:
    """Generate JSON Schema for a step method, skipping self/cls/ctx."""
    from typing import get_type_hints, Any as TypingAny
    hints = get_type_hints(method)
    sig = inspect.signature(method)
    properties: dict[str, Any] = {}
    required: list[str] = []
    for param_name, param in sig.parameters.items():
        if param_name in ("self", "cls", "ctx"):
            continue
        hint = hints.get(param_name, TypingAny)
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
            sdef = getattr(method, "_step_def", None)
            if sdef is None:
                continue
            schema = _generate_step_schema(method)
            steps.append(StepDef(
                name=sdef["name"],
                description=sdef["description"],
                handler=method,
                input_schema=schema,
                initial=sdef["initial"],
                next=sdef["next"],
                terminal=sdef["terminal"],
                no_cancel=sdef["no_cancel"],
                allow_during=sdef["allow_during"],
                block_during=sdef["block_during"],
                on_error=sdef["on_error"],
                requires=sdef["requires"],
                enum_fields=sdef["enum_fields"],
            ))

        on_cancel_fn = getattr(instance, "on_cancel", None)
        on_complete_fn = getattr(instance, "on_complete", None)

        wf = WorkflowDef(
            name=name,
            description=description,
            steps=steps,
            instance=instance,
            allow_during=allow_during,
            block_during=block_during,
            on_cancel=on_cancel_fn if callable(on_cancel_fn) else None,
            on_complete=on_complete_fn if callable(on_complete_fn) else None,
        )
        _workflow_registry.append(wf)
        return cls
    return decorator
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add sdk/python/src/protomcp/workflow.py sdk/python/tests/test_workflow.py
git commit -m "feat(workflow): @step and @workflow decorators with registration and schema generation"
```

---

### Task 3: Graph validation

**Files:**
- Modify: `sdk/python/src/protomcp/workflow.py`
- Modify: `sdk/python/tests/test_workflow.py`

- [ ] **Step 1: Write failing tests for graph validation**

Add to `sdk/python/tests/test_workflow.py`:

```python
import pytest

def test_validation_no_initial():
    clear_workflow_registry()
    with pytest.raises(ValueError, match="no initial step"):
        @workflow("bad")
        class W:
            @step(next=["done"])
            def start(self) -> StepResult:
                return StepResult(result="ok")

            @step(terminal=True)
            def done(self) -> StepResult:
                return StepResult(result="done")

def test_validation_multiple_initial():
    clear_workflow_registry()
    with pytest.raises(ValueError, match="multiple initial"):
        @workflow("bad")
        class W:
            @step(initial=True, next=["done"])
            def start1(self) -> StepResult:
                return StepResult(result="ok")

            @step(initial=True, terminal=True)
            def start2(self) -> StepResult:
                return StepResult(result="ok")

            @step(terminal=True)
            def done(self) -> StepResult:
                return StepResult(result="done")

def test_validation_missing_next_ref():
    clear_workflow_registry()
    with pytest.raises(ValueError, match="nonexistent"):
        @workflow("bad")
        class W:
            @step(initial=True, next=["nonexistent"])
            def start(self) -> StepResult:
                return StepResult(result="ok")

def test_validation_dead_end():
    clear_workflow_registry()
    with pytest.raises(ValueError, match="no next"):
        @workflow("bad")
        class W:
            @step(initial=True)
            def start(self) -> StepResult:
                return StepResult(result="ok")

def test_validation_terminal_with_next():
    clear_workflow_registry()
    with pytest.raises(ValueError, match="terminal.*next"):
        @workflow("bad")
        class W:
            @step(initial=True, terminal=True, next=["other"])
            def start(self) -> StepResult:
                return StepResult(result="ok")

            @step(terminal=True)
            def other(self) -> StepResult:
                return StepResult(result="ok")

def test_validation_valid_graph():
    clear_workflow_registry()
    # Should not raise
    @workflow("good")
    class W:
        @step(initial=True, next=["middle"])
        def start(self) -> StepResult:
            return StepResult(result="ok")

        @step(next=["end"])
        def middle(self) -> StepResult:
            return StepResult(result="ok")

        @step(terminal=True)
        def end(self) -> StepResult:
            return StepResult(result="done")

    assert len(get_registered_workflows()) == 1
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`

- [ ] **Step 3: Implement graph validation**

Add to `workflow.py`, called inside the `workflow` decorator after collecting steps:

```python
def _validate_workflow_graph(name: str, steps: list[StepDef]):
    """Validate the workflow step graph. Raises ValueError on structural errors."""
    step_names = {s.name for s in steps}

    # Must have exactly one initial step
    initials = [s for s in steps if s.initial]
    if len(initials) == 0:
        raise ValueError(f"Workflow '{name}': no initial step defined. "
                         f"Mark one step with @step(initial=True)")
    if len(initials) > 1:
        raise ValueError(f"Workflow '{name}': multiple initial steps: "
                         f"{[s.name for s in initials]}")

    for s in steps:
        # Terminal steps cannot have next
        if s.terminal and s.next:
            raise ValueError(f"Workflow '{name}' step '{s.name}': "
                             f"terminal step cannot have next={s.next}")

        # Non-terminal steps must have next
        if not s.terminal and not s.next:
            raise ValueError(f"Workflow '{name}' step '{s.name}': "
                             f"non-terminal step has no next defined (dead end)")

        # All next refs must exist
        if s.next:
            for ref in s.next:
                if ref not in step_names:
                    raise ValueError(f"Workflow '{name}' step '{s.name}': "
                                     f"next references nonexistent step '{ref}'")

        # on_error targets must exist
        if s.on_error:
            for exc_type, target in s.on_error.items():
                if target not in step_names:
                    raise ValueError(f"Workflow '{name}' step '{s.name}': "
                                     f"on_error target '{target}' does not exist")
```

Call `_validate_workflow_graph(name, steps)` in the `workflow` decorator before appending to `_workflow_registry`.

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add sdk/python/src/protomcp/workflow.py sdk/python/tests/test_workflow.py
git commit -m "feat(workflow): graph validation — initial/terminal checks, next refs, dead ends"
```

---

## Chunk 2: Tool Generation and Step Dispatch

### Task 4: Generate ToolDefs from workflows

**Files:**
- Modify: `sdk/python/src/protomcp/workflow.py`
- Modify: `sdk/python/src/protomcp/tool.py`
- Modify: `sdk/python/src/protomcp/__init__.py`
- Modify: `sdk/python/tests/test_workflow.py`

- [ ] **Step 1: Write failing tests for tool generation**

```python
from protomcp.tool import get_registered_tools, clear_registry

def test_workflow_generates_tool_defs():
    clear_workflow_registry()
    clear_registry()

    @workflow("deploy")
    class W:
        @step(initial=True, next=["done"])
        def review(self, pr_url: str) -> StepResult:
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    tools = get_registered_tools()
    tool_names = [t.name for t in tools]
    assert "deploy.review" in tool_names
    assert "deploy.done" in tool_names
    assert "deploy.cancel" in tool_names

def test_workflow_cancel_not_generated_when_all_steps_no_cancel():
    clear_workflow_registry()
    clear_registry()

    @workflow("locked")
    class W:
        @step(initial=True, next=["done"], no_cancel=True)
        def start(self) -> StepResult:
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    tools = get_registered_tools()
    tool_names = [t.name for t in tools]
    assert "locked.start" in tool_names
    # cancel should still exist since 'done' step allows cancel by default
    # (terminal steps don't get cancel injected anyway, but cancel tool
    # should exist if ANY non-terminal step allows it)
    # Actually: start has no_cancel=True, done is terminal. So no step gets cancel.
    assert "locked.cancel" not in tool_names
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py::test_workflow_generates_tool_defs -v`

- [ ] **Step 3: Implement tool generation**

Add to `workflow.py`:

```python
import json


def workflows_to_tool_defs() -> list[ToolDef]:
    """Convert registered workflows into ToolDef entries."""
    defs: list[ToolDef] = []
    for wf in _workflow_registry:
        defs.extend(_workflow_to_tool_defs(wf))
    return defs


def _workflow_to_tool_defs(wf: WorkflowDef) -> list[ToolDef]:
    """Generate separate ToolDefs for each step + cancel."""
    defs: list[ToolDef] = []

    for s in wf.steps:
        def make_handler(wf_def, step_def):
            def handler(**kwargs):
                return _handle_step_call(wf_def, step_def, kwargs)
            return handler

        defs.append(ToolDef(
            name=f"{wf.name}.{s.name}",
            description=s.description or f"{wf.name}: {s.name}",
            input_schema_json=json.dumps(s.input_schema),
            handler=make_handler(wf, s),
            hidden=not s.initial,  # only initial step starts visible
        ))

    # Add cancel tool if any non-terminal step allows cancel
    needs_cancel = any(not s.no_cancel and not s.terminal for s in wf.steps)
    if needs_cancel:
        def make_cancel_handler(wf_def):
            def handler(**kwargs):
                return _handle_cancel(wf_def)
            return handler

        defs.append(ToolDef(
            name=f"{wf.name}.cancel",
            description=f"Cancel the {wf.name} workflow",
            input_schema_json=json.dumps({"type": "object", "properties": {}}),
            handler=make_cancel_handler(wf),
            hidden=True,  # starts hidden, enabled on first transition
        ))

    return defs
```

Add placeholder dispatch functions (implemented in Task 5):

```python
def _handle_step_call(wf: WorkflowDef, step_def: StepDef, kwargs: dict):
    """Handle a step tool call. Implemented in Task 5."""
    return ToolResult(result="not yet implemented")


def _handle_cancel(wf: WorkflowDef):
    """Handle cancel. Implemented in Task 5."""
    return ToolResult(result="not yet implemented")
```

Update `sdk/python/src/protomcp/tool.py` `get_registered_tools()`:

```python
def get_registered_tools() -> list[ToolDef]:
    from protomcp.group import get_registered_groups, groups_to_tool_defs
    from protomcp.workflow import workflows_to_tool_defs
    return list(_registry) + groups_to_tool_defs() + workflows_to_tool_defs()
```

Update `sdk/python/src/protomcp/__init__.py` — add exports:

```python
from protomcp.workflow import workflow, step, StepResult, get_registered_workflows, clear_workflow_registry
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`

- [ ] **Step 5: Commit**

```bash
git add sdk/python/src/protomcp/workflow.py sdk/python/src/protomcp/tool.py sdk/python/src/protomcp/__init__.py sdk/python/tests/test_workflow.py
git commit -m "feat(workflow): generate ToolDefs from workflow steps with cancel injection"
```

---

### Task 5: Step dispatch — state management, transitions, cancel

**Files:**
- Modify: `sdk/python/src/protomcp/workflow.py`
- Modify: `sdk/python/tests/test_workflow.py`

- [ ] **Step 1: Write failing tests for step dispatch**

```python
def test_initial_step_dispatch():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self, name: str) -> StepResult:
            return StepResult(result=f"started {name}")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="complete")

    tools = get_registered_tools()
    start_tool = [t for t in tools if t.name == "test.start"][0]
    result = start_tool.handler(name="my-project")
    assert isinstance(result, ToolResult)
    assert "started my-project" in result.result

def test_workflow_state_persists():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        def __init__(self):
            self.data = None

        @step(initial=True, next=["done"])
        def start(self, value: str) -> StepResult:
            self.data = value
            return StepResult(result="stored")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result=f"data was {self.data}")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    start.handler(value="hello")

    done = [t for t in tools if t.name == "test.done"][0]
    result = done.handler()
    assert "data was hello" in result.result

def test_dynamic_next_narrows():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["a", "b"])
        def start(self) -> StepResult:
            return StepResult(result="ok", next=["a"])  # narrow to just a

        @step(terminal=True)
        def a(self) -> StepResult:
            return StepResult(result="a")

        @step(terminal=True)
        def b(self) -> StepResult:
            return StepResult(result="b")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    result = start.handler()
    assert not result.is_error

def test_dynamic_next_rejects_invalid():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["a"])
        def start(self) -> StepResult:
            return StepResult(result="ok", next=["b"])  # b not in declared next

        @step(terminal=True)
        def a(self) -> StepResult:
            return StepResult(result="a")

        @step(terminal=True)
        def b(self) -> StepResult:
            return StepResult(result="b")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    result = start.handler()
    assert result.is_error
    assert "not declared" in result.result

def test_cancel_calls_on_cancel():
    clear_workflow_registry()
    clear_registry()
    cancelled = []

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self) -> StepResult:
            return StepResult(result="started")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

        def on_cancel(self, current_step, history):
            cancelled.append((current_step, len(history)))
            return "cleanup done"

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    start.handler()

    cancel = [t for t in tools if t.name == "test.cancel"][0]
    result = cancel.handler()
    assert "cleanup done" in result.result
    assert len(cancelled) == 1

def test_on_complete_called():
    clear_workflow_registry()
    clear_registry()
    completed = []

    @workflow("test")
    class W:
        @step(initial=True, terminal=True)
        def start(self) -> StepResult:
            return StepResult(result="done")

        def on_complete(self, history):
            completed.append(len(history))

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    start.handler()
    assert len(completed) == 1

def test_history_tracks_steps():
    clear_workflow_registry()
    clear_registry()
    recorded_history = []

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self) -> StepResult:
            return StepResult(result="started")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="finished")

        def on_complete(self, history):
            recorded_history.extend(history)

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    start.handler()
    done = [t for t in tools if t.name == "test.done"][0]
    done.handler()
    assert len(recorded_history) == 2
    assert recorded_history[0][0] == "start"
    assert recorded_history[1][0] == "done"
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`

- [ ] **Step 3: Implement step dispatch**

Replace the placeholder `_handle_step_call` and `_handle_cancel` in `workflow.py`:

```python
def _get_active_workflow() -> WorkflowState | None:
    """Get the currently active workflow state, or None."""
    return _active_workflow_stack[-1] if _active_workflow_stack else None


def _handle_step_call(wf: WorkflowDef, step_def: StepDef, kwargs: dict) -> ToolResult:
    """Handle a step tool call — manage state transitions."""
    active = _get_active_workflow()

    if step_def.initial and active is None:
        # Starting a new workflow
        state = WorkflowState(
            workflow_name=wf.name,
            current_step=step_def.name,
            history=[],
            pre_workflow_tools=tool_manager.get_active_tools(),
            instance=wf.instance,
        )
        _active_workflow_stack.append(state)
    elif active is not None and active.workflow_name == wf.name:
        # Continuing an active workflow
        active.current_step = step_def.name
    else:
        return ToolResult(
            result=f"Cannot call {wf.name}.{step_def.name} — workflow not active",
            is_error=True,
        )

    active = _active_workflow_stack[-1]

    # Run the step handler
    try:
        result = step_def.handler(**kwargs)
    except Exception as e:
        # Check on_error transitions
        if step_def.on_error:
            for exc_type, target_step in step_def.on_error.items():
                if isinstance(e, exc_type):
                    # Transition to error step
                    _transition_to_steps(wf, active, [target_step])
                    return ToolResult(
                        result=str(e),
                        is_error=True,
                    )
        # Default: stay in current state, agent can retry or cancel
        return ToolResult(result=str(e), is_error=True)

    if not isinstance(result, StepResult):
        result = StepResult(result=str(result))

    # Validate dynamic next
    if result.next is not None and step_def.next:
        for n in result.next:
            if n not in step_def.next:
                return ToolResult(
                    result=f"Step '{step_def.name}' returned next='{n}' which is not declared in next={step_def.next}",
                    is_error=True,
                )

    # Record in history
    active.history.append((step_def.name, result))

    # Terminal step — complete workflow
    if step_def.terminal:
        if wf.on_complete:
            wf.on_complete(active.history)
        _restore_tools(active)
        _active_workflow_stack.pop()
        return ToolResult(result=result.result)

    # Determine next steps
    next_steps = result.next if result.next is not None else step_def.next
    _transition_to_steps(wf, active, next_steps)

    return ToolResult(result=result.result)


def _handle_cancel(wf: WorkflowDef) -> ToolResult:
    """Handle workflow cancellation."""
    active = _get_active_workflow()
    if active is None or active.workflow_name != wf.name:
        return ToolResult(result="No active workflow to cancel", is_error=True)

    message = f"Workflow '{wf.name}' cancelled"
    if wf.on_cancel:
        cancel_result = wf.on_cancel(active.current_step, active.history)
        if cancel_result:
            message = str(cancel_result)

    _restore_tools(active)
    _active_workflow_stack.pop()
    return ToolResult(result=message)


def _transition_to_steps(wf: WorkflowDef, state: WorkflowState, next_steps: list[str]):
    """Transition tool visibility to show the given next steps."""
    step_defs = {s.name: s for s in wf.steps}
    visible: list[str] = []

    # Add next step tools
    for ns in next_steps:
        visible.append(f"{wf.name}.{ns}")

    # Add cancel if any next step allows it
    any_allows_cancel = any(not step_defs[ns].no_cancel for ns in next_steps if ns in step_defs)
    if any_allows_cancel:
        visible.append(f"{wf.name}.cancel")

    # Add non-workflow tools based on visibility rules
    # Determine which rule set to use (first next step's override, or workflow default)
    # Use the first next step's visibility if it has an override, otherwise workflow default
    first_next = step_defs.get(next_steps[0]) if next_steps else None
    allow = first_next.allow_during if first_next and first_next.allow_during is not None else wf.allow_during
    block = first_next.block_during if first_next and first_next.block_during is not None else wf.block_during

    if allow is not None or block is not None:
        # Get all registered tool names (from snapshot)
        all_tools = state.pre_workflow_tools
        for t in all_tools:
            if t.startswith(f"{wf.name}."):
                continue  # skip workflow's own tools
            if _matches_visibility(t, allow, block):
                visible.append(t)

    tool_manager.set_allowed(visible)


def _matches_visibility(tool_name: str, allow: list[str] | None, block: list[str] | None) -> bool:
    """Check if a tool passes the allow/block filter."""
    if allow is not None:
        if not any(fnmatch.fnmatch(tool_name, pattern) for pattern in allow):
            return False
    if block is not None:
        if any(fnmatch.fnmatch(tool_name, pattern) for pattern in block):
            return False
    return True


def _restore_tools(state: WorkflowState):
    """Restore the tool list to what it was before the workflow started."""
    tool_manager.set_allowed(state.pre_workflow_tools)
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow.py -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add sdk/python/src/protomcp/workflow.py sdk/python/tests/test_workflow.py
git commit -m "feat(workflow): step dispatch with state management, transitions, cancel, and on_complete"
```

---

## Chunk 3: Error Handling and Tool Visibility

### Task 6: Error handling — stay in state, on_error transitions

**Files:**
- Create: `sdk/python/tests/test_workflow_errors.py`
- Verify existing error handling in `workflow.py`

- [ ] **Step 1: Write error handling tests**

```python
# sdk/python/tests/test_workflow_errors.py
from protomcp.workflow import workflow, step, StepResult, clear_workflow_registry
from protomcp.tool import get_registered_tools, clear_registry
from protomcp.result import ToolResult

def test_error_stays_in_state():
    clear_workflow_registry()
    clear_registry()
    call_count = 0

    @workflow("test")
    class W:
        @step(initial=True, next=["done"])
        def start(self, value: str) -> StepResult:
            nonlocal call_count
            call_count += 1
            if value == "bad":
                raise ValueError("invalid input")
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]

    # First call fails
    result = start.handler(value="bad")
    assert result.is_error
    assert "invalid input" in result.result

    # Retry succeeds
    result = start.handler(value="good")
    assert not result.is_error
    assert call_count == 2

def test_on_error_transitions():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"],
              on_error={ValueError: "error_review"})
        def start(self, value: str) -> StepResult:
            if value == "bad":
                raise ValueError("invalid")
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

        @step(terminal=True)
        def error_review(self) -> StepResult:
            return StepResult(result="reviewed error")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    result = start.handler(value="bad")
    assert result.is_error

def test_on_error_catch_all():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"],
              on_error={Exception: "fallback"})
        def start(self) -> StepResult:
            raise RuntimeError("unexpected")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

        @step(terminal=True)
        def fallback(self) -> StepResult:
            return StepResult(result="recovered")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    result = start.handler()
    assert result.is_error
    assert "unexpected" in result.result

def test_no_cancel_with_error_allows_retry():
    clear_workflow_registry()
    clear_registry()
    attempts = 0

    @workflow("test")
    class W:
        @step(initial=True, next=["done"], no_cancel=True)
        def start(self) -> StepResult:
            nonlocal attempts
            attempts += 1
            if attempts < 3:
                raise ValueError("not ready yet")
            return StepResult(result="ok")

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]

    result = start.handler()
    assert result.is_error
    result = start.handler()
    assert result.is_error
    result = start.handler()
    assert not result.is_error
    assert attempts == 3

def test_unmatched_error_stays_in_state():
    clear_workflow_registry()
    clear_registry()

    @workflow("test")
    class W:
        @step(initial=True, next=["done"],
              on_error={ValueError: "recovery"})
        def start(self) -> StepResult:
            raise TypeError("wrong type")  # not ValueError

        @step(terminal=True)
        def done(self) -> StepResult:
            return StepResult(result="done")

        @step(terminal=True)
        def recovery(self) -> StepResult:
            return StepResult(result="recovered")

    tools = get_registered_tools()
    start = [t for t in tools if t.name == "test.start"][0]
    result = start.handler()
    assert result.is_error
    assert "wrong type" in result.result
    # Should stay in state, not transition to recovery
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow_errors.py -v`
Expected: All PASS (error handling already implemented in Task 5)

- [ ] **Step 3: Commit**

```bash
git add sdk/python/tests/test_workflow_errors.py
git commit -m "test(workflow): error handling — stay in state, on_error transitions, no_cancel + retry"
```

---

### Task 7: Tool visibility — allow_during, block_during, step overrides

**Files:**
- Create: `sdk/python/tests/test_workflow_visibility.py`

Note: Visibility tests need `tool_manager` to be connected. Since `tool_manager` requires a transport, these tests will mock it. The core `_matches_visibility` function can be tested directly.

- [ ] **Step 1: Write visibility tests**

```python
# sdk/python/tests/test_workflow_visibility.py
from protomcp.workflow import _matches_visibility

def test_no_filters_blocks_all():
    # When neither allow nor block specified (exclusive mode),
    # the function isn't called — but if it were with None/None, allow all
    assert _matches_visibility("status", None, None) is True

def test_allow_pattern():
    assert _matches_visibility("status", ["status", "logs.*"], None) is True
    assert _matches_visibility("logs.tail", ["status", "logs.*"], None) is True
    assert _matches_visibility("deploy.start", ["status", "logs.*"], None) is False

def test_block_pattern():
    assert _matches_visibility("status", None, ["deploy.*"]) is True
    assert _matches_visibility("deploy.start", None, ["deploy.*"]) is False

def test_allow_then_block():
    # allow status and logs.*, but block logs.debug
    allow = ["status", "logs.*"]
    block = ["logs.debug"]
    assert _matches_visibility("status", allow, block) is True
    assert _matches_visibility("logs.tail", allow, block) is True
    assert _matches_visibility("logs.debug", allow, block) is False
    assert _matches_visibility("deploy.start", allow, block) is False

def test_wildcard_patterns():
    assert _matches_visibility("read_file", ["read_*"], None) is True
    assert _matches_visibility("write_file", ["read_*"], None) is False

def test_exact_match():
    assert _matches_visibility("status", ["status"], None) is True
    assert _matches_visibility("status2", ["status"], None) is False
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/test_workflow_visibility.py -v`
Expected: All PASS

- [ ] **Step 3: Commit**

```bash
git add sdk/python/tests/test_workflow_visibility.py
git commit -m "test(workflow): tool visibility — allow/block patterns, wildcards, combination filters"
```

---

## Chunk 4: Example, Docs, Full Test Suite

### Task 8: Example and documentation

**Files:**
- Create: `examples/python/workflow_deploy.py`
- Modify: `docs/src/content/docs/guides/writing-tools-python.mdx`
- Modify: `docs/src/content/docs/reference/python-api.mdx`

- [ ] **Step 1: Create deploy workflow example**

```python
# examples/python/workflow_deploy.py
"""
Workflow Example: Deployment Pipeline
=====================================
Demonstrates @workflow with multi-step state machine.
Run with: pmcp dev -- python workflow_deploy.py
"""
from protomcp import workflow, step, StepResult, tool, ToolResult
from protomcp.runner import run


@workflow("deploy", allow_during=["status"])
class DeployWorkflow:
    def __init__(self):
        self.pr_url = None
        self.test_results = None

    @step(initial=True, next=["approve", "reject"],
          description="Review changes before deployment")
    def review(self, pr_url: str) -> StepResult:
        self.pr_url = pr_url
        return StepResult(result=f"Reviewing {pr_url}: 5 files changed")

    @step(next=["run_tests"],
          description="Approve the changes for deployment")
    def approve(self, reason: str) -> StepResult:
        return StepResult(result=f"Approved: {reason}")

    @step(terminal=True,
          description="Reject the changes")
    def reject(self, reason: str) -> StepResult:
        return StepResult(result=f"Rejected: {reason}")

    @step(next=["promote", "rollback"], no_cancel=True,
          description="Run test suite against staging")
    def run_tests(self) -> StepResult:
        self.test_results = {"passed": 42, "failed": 0}
        if self.test_results["failed"] == 0:
            return StepResult(result="All 42 tests passed", next=["promote"])
        return StepResult(result="Tests failed", next=["rollback"])

    @step(terminal=True, no_cancel=True,
          description="Deploy to production")
    def promote(self) -> StepResult:
        return StepResult(result=f"Deployed {self.pr_url} to production")

    @step(terminal=True,
          description="Roll back staging deployment")
    def rollback(self) -> StepResult:
        return StepResult(result="Rolled back staging")

    def on_cancel(self, current_step, history):
        return f"Deploy cancelled at step '{current_step}'"

    def on_complete(self, history):
        steps = " → ".join(s[0] for s in history)
        print(f"[audit] Deploy complete: {steps}")


@tool("Check deployment status", read_only=True)
def status() -> ToolResult:
    return ToolResult(result="All systems nominal")


if __name__ == "__main__":
    run()
```

- [ ] **Step 2: Add Workflows section to Python guide**

Add a `## Workflows` section to `docs/src/content/docs/guides/writing-tools-python.mdx` covering:
- `@workflow` and `@step` decorators
- Step lifecycle (initial → transitions → terminal)
- `StepResult` with dynamic `next`
- `no_cancel` for committed steps
- `on_cancel` and `on_complete` hooks
- `allow_during` / `block_during` with glob patterns
- Step-level visibility overrides
- Error handling: stay in state, `on_error` transitions

- [ ] **Step 3: Add Workflow API entries to python-api.mdx**

Add reference entries for:
- `@workflow(name, description, allow_during, block_during)`
- `@step(initial, next, terminal, no_cancel, description, allow_during, block_during, on_error, requires, enum_fields)`
- `StepResult(result, next)`
- `WorkflowDef`, `StepDef`, `WorkflowState`
- `get_registered_workflows()`, `clear_workflow_registry()`

- [ ] **Step 4: Run full test suite**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest tests/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add examples/python/workflow_deploy.py docs/src/content/docs/guides/writing-tools-python.mdx docs/src/content/docs/reference/python-api.mdx
git commit -m "docs: add workflow guide, API reference, and deploy pipeline example"
```

---

## Summary

| Task | What | New/Modified files |
|------|------|-------------------|
| 1 | Data structures | `workflow.py`, `test_workflow.py` |
| 2 | `@step` + `@workflow` decorators | `workflow.py`, `test_workflow.py` |
| 3 | Graph validation | `workflow.py`, `test_workflow.py` |
| 4 | Tool generation | `workflow.py`, `tool.py`, `__init__.py`, `test_workflow.py` |
| 5 | Step dispatch + state + cancel | `workflow.py`, `test_workflow.py` |
| 6 | Error handling tests | `test_workflow_errors.py` |
| 7 | Visibility tests | `test_workflow_visibility.py` |
| 8 | Example + docs | `workflow_deploy.py`, guide + API ref |
