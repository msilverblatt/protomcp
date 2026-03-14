# Server-Defined Workflows

## Problem

Agents orchestrating multi-step processes must figure out which tools to call in what order, remember where they are, and avoid invalid sequences. This is error-prone — agents forget steps, call tools out of order, or skip required validation. The developer has no way to enforce a correct sequence at the protocol level.

## Solution

A `@workflow` decorator that lets the server define multi-step processes as state machines. The visible MCP tool surface reflects the current state — the agent only sees valid next actions. The server is the orchestrator; the agent is the executor.

No protocol changes. Workflows compile down to standard tool definitions and dynamic tool list updates.

---

## Core API

### `@workflow(name, allow_during?, block_during?)`

Class decorator. Registers a workflow definition. Methods decorated with `@step` become states in the state machine.

```python
from protomcp import workflow, step, StepResult

@workflow("deploy")
class DeployWorkflow:
    @step(initial=True, next=["approve", "reject"])
    def review(self, changes: str) -> StepResult:
        return StepResult(result="3 files changed")

    @step(next=["run_tests"])
    def approve(self, reason: str) -> StepResult:
        return StepResult(result="Approved")

    @step(next=["promote", "rollback"])
    def run_tests(self) -> StepResult:
        results = execute_tests()
        return StepResult(result=f"{results.passed} passed")

    @step(terminal=True)
    def promote(self) -> StepResult:
        return StepResult(result="Live in production")

    @step(terminal=True)
    def rollback(self) -> StepResult:
        return StepResult(result="Rolled back")

    @step(terminal=True)
    def reject(self, reason: str) -> StepResult:
        return StepResult(result=f"Rejected: {reason}")
```

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `name` | `str` | required | Workflow name. Tools are registered as `{name}.{step_name}` |
| `allow_during` | `list[str]` | `None` | Glob patterns of non-workflow tools allowed alongside workflow steps |
| `block_during` | `list[str]` | `None` | Glob patterns of non-workflow tools hidden during the workflow |

When neither is specified, the workflow is fully exclusive — only workflow steps and cancel are visible.

When both are specified, `allow_during` is applied first (whitelist), then `block_during` removes from the result.

### `@step(...)`

Method decorator. Defines a state in the workflow graph.

**Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `initial` | `bool` | `False` | This step is the entry point. Exactly one required per workflow |
| `next` | `list[str]` | `None` | Step names that become available after this step completes |
| `terminal` | `bool` | `False` | This step ends the workflow. Cannot have `next` |
| `no_cancel` | `bool` | `False` | Do not inject cancel at this step. Agent must continue forward |
| `allow_during` | `list[str]` | `None` | Override workflow-level tool visibility for this step |
| `block_during` | `list[str]` | `None` | Override workflow-level tool visibility for this step |
| `description` | `str` | `""` | Description shown to the agent |
| `requires` | `list[str]` | `None` | Required parameters (from declarative validation) |
| `enum_fields` | `dict` | `None` | Enum validation with fuzzy matching |

Step-level `allow_during`/`block_during` **replaces** (does not merge with) the workflow-level setting.

### `StepResult`

Returned by step handlers.

```python
@dataclass
class StepResult:
    result: str = ""           # Text shown to the agent
    next: list[str] | None = None  # Narrow the declared next set (must be subset)
```

If `next` is specified, it must be a subset of the step's declared `next`. The framework raises a runtime error if the handler returns a step not declared in the decorator. If `next` is not specified, all declared next steps are enabled.

```python
@step(next=["promote", "rollback", "retry"])
def run_tests(self) -> StepResult:
    results = execute_tests()
    if results.all_passed:
        return StepResult(result="All passed", next=["promote"])
    else:
        return StepResult(result="Failures detected", next=["retry", "rollback"])
```

---

## Workflow Lifecycle

### 1. Registration

When the `@workflow` class is processed:
- Each `@step` method is collected
- The step graph is validated (see Graph Validation below)
- Tool definitions are generated using `strategy="separate"` — each step becomes `{workflow}.{step}`
- Only the `initial` step is registered as enabled. All others + cancel start hidden
- Registration-time warnings are logged for tool visibility contradictions

### 2. Start

When the agent calls the initial step tool (e.g., `deploy.review`):
- The framework snapshots the current full tool list (`pre_workflow_tools`)
- Creates a `WorkflowState` instance with the workflow class instance, empty history
- Runs the step handler
- Transitions to the next state (enables next steps, disables current, applies visibility rules)

### 3. Transitions

After each step handler completes:
1. Record `(step_name, StepResult)` in history
2. If terminal step: call `on_complete` if defined, restore `pre_workflow_tools`, done
3. Compute the new visible tool set:
   - Start with `{next step tools}`
   - Add `{workflow}.cancel` unless `no_cancel=True` on all next steps
   - Add non-workflow tools that pass the allow/block filter (step-level if specified, else workflow-level)
4. Call `tool_manager.set_allowed(visible_set)`

### 4. Cancel

`{workflow}.cancel` is auto-injected at every step unless `no_cancel=True`. When called:
- Calls `on_cancel(current_step, history)` on the workflow instance if defined
- Restores `pre_workflow_tools`
- Returns the `on_cancel` return value (or a default message) as the tool result

### 5. Complete

When a terminal step finishes:
- Calls `on_complete(history)` on the workflow instance if defined
- Restores `pre_workflow_tools`

---

## Workflow State

```python
@dataclass
class WorkflowState:
    workflow_name: str
    current_step: str
    history: list[tuple[str, StepResult]]
    pre_workflow_tools: list[str]  # snapshot for restore
    instance: Any                  # workflow class instance
```

The workflow class is instantiated once when the workflow starts. The instance persists across all steps, so `self` carries state:

```python
@workflow("deploy")
class DeployWorkflow:
    def __init__(self):
        self.pr_url = None
        self.test_results = None

    @step(initial=True, next=["approve", "reject"])
    def review(self, pr_url: str) -> StepResult:
        self.pr_url = pr_url
        return StepResult(result="Ready for review")

    @step(next=["run_tests"], no_cancel=True)
    def approve(self, reason: str) -> StepResult:
        return StepResult(result="Approved for testing")
```

**Internal state stack:** The framework uses a stack for `WorkflowState` (depth capped at 1 for now). This enables sub-workflows in a future version without breaking changes.

---

## Tool Visibility Rules

### Workflow-level

| Configuration | Behavior |
|--------------|----------|
| Neither `allow_during` nor `block_during` | Fully exclusive: only workflow steps + cancel visible |
| `allow_during=["status", "logs.*"]` | Exclusive + these specific tools allowed |
| `block_during=["deploy.*", "delete_*"]` | All tools visible except these |
| Both specified | `allow_during` whitelist first, then `block_during` removes from result |

Glob patterns: `*` matches within a name, `.*` matches namespaced tools (e.g., `logs.*` matches `logs.tail`, `logs.search`).

### Step-level overrides

If a step specifies `allow_during` or `block_during`, it **replaces** the workflow-level setting for that step entirely:

```python
@workflow("deploy", allow_during=["status", "logs.*"])
class DeployWorkflow:
    @step(initial=True, next=["approve"],
          allow_during=["diff", "file_read", "status"])  # replaces workflow-level
    def review(self, changes: str) -> StepResult: ...

    @step(next=["verify"], no_cancel=True,
          allow_during=[])  # full lockdown, not even status
    def run_migration(self) -> StepResult: ...

    @step(terminal=True)  # inherits workflow-level: status + logs.*
    def verify(self) -> StepResult: ...
```

### Edge case: mid-workflow tool list changes

If a non-workflow tool modifies the tool list (e.g., `status` enables `debug_dump`), the workflow's allow/block filter is reapplied on the next step transition. The workflow is authoritative.

---

## Graph Validation

### Hard errors (prevent registration)

- No `initial=True` step defined
- Multiple `initial=True` steps
- A `next` reference points to a step name that doesn't exist in the workflow
- A non-terminal step has no `next` defined (dead end)
- A terminal step has `next` defined (contradiction)

### Warnings (logged at startup)

- A step is unreachable (not referenced by any other step's `next`, and not `initial`)
- `no_cancel=True` on a terminal step (redundant)
- Step allows a tool that the workflow blocks
- Step or workflow references a tool name/pattern that isn't registered

---

## Lifecycle Hooks

### `on_cancel(current_step, history) -> str`

Optional method on the workflow class. Called when the agent cancels. Return value becomes the tool result.

```python
def on_cancel(self, current_step: str, history: list) -> str:
    cleanup_staging()
    return "Deploy cancelled, staging cleaned up"
```

### `on_complete(history) -> None`

Optional method on the workflow class. Called when a terminal step finishes. For cleanup, logging, audit trails.

```python
def on_complete(self, history: list) -> None:
    log_audit_trail(history)
```

---

## Implementation Architecture

### Builds on existing primitives

| Feature | How workflows use it |
|---------|---------------------|
| Tool groups (separate strategy) | Each step becomes a `{workflow}.{step}` tool |
| Dynamic tool lists (`tool_manager`) | `set_allowed()` enforces visibility on each transition |
| Declarative validation | Steps can use `requires`, `enum_fields`, `cross_rules` |
| Server context | Context resolvers inject into step handlers |
| Local middleware | Middleware wraps step handlers like any tool call |
| Telemetry | Step transitions emit `ToolCallEvent` with `action=step_name` |

### New module

`sdk/python/src/protomcp/workflow.py` — depends on `group.py`, `tool.py`, `manager.py`. Does NOT modify `runner.py`.

### Key types

- `WorkflowDef` — stores the workflow definition (name, steps, visibility config)
- `StepDef` — stores step metadata (name, next, flags, handler)
- `WorkflowState` — runtime state (current step, history, tool snapshot, instance)
- `_active_workflow_stack: list[WorkflowState]` — module-level state, depth capped at 1

### How it generates tools

`@workflow` creates a tool group with `strategy="separate"`. The initial step's tool handler:
1. Creates `WorkflowState`, snapshots tools
2. Runs the step handler
3. Computes next visible set
4. Calls `tool_manager.set_allowed()`

Subsequent step handlers:
1. Validate the step is actually available (defensive check)
2. Run the step handler
3. If terminal: restore snapshot, call `on_complete`
4. Else: compute next visible set, `set_allowed()`

The cancel handler:
1. Call `on_cancel` if defined
2. Restore snapshot

### No proto changes

Workflows produce standard `ToolDefinition` messages. Transitions use standard `enable_tools`/`disable_tools` or `set_allowed`. The Go bridge and MCP host see normal tool list changes.

### No runner changes

Workflow tools are just tools with handlers that manage their own visibility via `tool_manager`. The runner dispatches them like any other tool.

---

## Multi-SDK Support

Workflows should be implemented in all 4 SDKs following the same pattern as tool groups:

| SDK | API style |
|-----|-----------|
| Python | `@workflow` / `@step` decorators |
| Go | `Workflow("name", opts...)` with `Step("name", opts...)` |
| TypeScript | `workflow({name, steps: {...}})` |
| Rust | `workflow("name").step("review", \|s\| ...).register()` |

---

## Examples

### Deployment pipeline

```python
@workflow("deploy", allow_during=["status", "logs.*"])
class DeployWorkflow:
    def __init__(self):
        self.changes = None

    @step(initial=True, next=["approve", "reject"],
          allow_during=["diff", "file_read", "status"])
    def review(self, pr_url: str) -> StepResult:
        self.changes = fetch_diff(pr_url)
        return StepResult(result=f"{len(self.changes)} files changed")

    @step(next=["run_tests"])
    def approve(self, reason: str) -> StepResult:
        return StepResult(result="Approved for staging")

    @step(terminal=True)
    def reject(self, reason: str) -> StepResult:
        return StepResult(result=f"Rejected: {reason}")

    @step(next=["promote", "rollback"], no_cancel=True)
    def run_tests(self) -> StepResult:
        results = execute_tests()
        if results.all_passed:
            return StepResult(result="All passed", next=["promote"])
        return StepResult(result="Failures", next=["rollback"])

    @step(terminal=True, no_cancel=True)
    def promote(self) -> StepResult:
        deploy_to_prod()
        return StepResult(result="Live in production")

    @step(terminal=True)
    def rollback(self) -> StepResult:
        rollback_staging()
        return StepResult(result="Rolled back")

    def on_cancel(self, current_step, history):
        cleanup_staging()
        return "Deploy cancelled"

    def on_complete(self, history):
        log_audit_trail(history)
```

### Data onboarding

```python
@workflow("onboard", block_during=["deploy.*", "delete_*"])
class OnboardWorkflow:
    @step(initial=True, next=["add_source"])
    def configure(self, project_name: str) -> StepResult:
        init_project(project_name)
        return StepResult(result=f"Project '{project_name}' created")

    @step(next=["add_source", "select_model"],
          description="Add a data source (CSV, database, API)")
    def add_source(self, source_path: str, source_type: str = "csv") -> StepResult:
        ingest(source_path, source_type)
        return StepResult(result=f"Added {source_path}")

    @step(next=["train"], requires=["model_type"])
    def select_model(self, model_type: str) -> StepResult:
        return StepResult(result=f"Selected {model_type}")

    @step(terminal=True, no_cancel=True)
    def train(self) -> StepResult:
        run_training()
        return StepResult(result="Training complete")
```
