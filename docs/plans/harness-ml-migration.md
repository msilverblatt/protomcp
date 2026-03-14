# Plan: Make protomcp the best MCP framework for complex Python servers

## Context

[HarnessML](https://github.com/msilverblatt/harness-ml) is an agent-driven ML framework with an MCP server (`harness-plugin`) that exposes 8 tools with 80+ actions. It currently uses [FastMCP](https://github.com/modelcontextprotocol/python-sdk) (the official Python MCP SDK). This plan documents what protomcp needs to become unequivocally better for servers of this complexity.

HarnessML is the reference case, but these improvements would benefit any non-trivial MCP server.

### Current harness-ml MCP architecture

```
mcp_server.py          ~1200 lines — tool signatures, _safe_tool wrapper, Studio lifecycle
handlers/
  models.py            ~100 lines — 10 actions (add, update, remove, list, clone, batch...)
  data.py              ~500 lines — 30 actions (add, profile, derive_column, views, upload...)
  features.py          ~150 lines — 7 actions (add, discover, prune, auto_search...)
  experiments.py       ~280 lines — 10 actions (create, run, explore, promote, journal...)
  pipeline.py          ~380 lines — 15 actions (run_backtest, diagnostics, explain, compare...)
  config.py            ~300 lines — 13 actions (init, backtest, ensemble, suggest_cv...)
  notebook.py          ~60 lines  — 5 actions (write, read, search, strike, summary)
  competitions.py      ~550 lines — 12 actions
  _validation.py       ~220 lines — fuzzy match, required params, cross-param hints
  _common.py           ~75 lines  — project_dir resolution, progress callback, event emitter
```

Key files to study:
- `mcp_server.py` — the full server, every pain point is visible here
- `_validation.py` — the validation layer that should be declarative
- `_common.py` — shared concerns that should be framework-level

---

## Item 1: Tool Groups with Per-Action Schemas

### The problem

Each harness-ml tool is a single function whose signature is the **union of all parameters for all actions**. The `data` tool has 25+ parameters; for any given action, only 2-3 are relevant. The JSON schema sent to the LLM is a 25-field blob with no indication of which fields go with which action.

Current pattern in `mcp_server.py`:

```python
@mcp.tool()
@_safe_tool
async def data(
    action: str,
    ctx: Context,
    data_path: str | None = None,      # used by: add, validate
    join_on: list[str] | None = None,   # used by: add
    prefix: str | None = None,          # used by: add
    column: str | None = None,          # used by: fill_nulls, detect_outliers
    strategy: str = "median",           # used by: fill_nulls
    value: float | None = None,         # used by: fill_nulls
    name: str | None = None,            # used by: add_view, add_source, derive_column
    source: str | None = None,          # used by: add_view
    steps: str | list | None = None,    # used by: add_view
    expression: str | None = None,      # used by: derive_column
    # ... 15 more parameters ...
    project_dir: str | None = None,
) -> str:
    """Manage data... (100-line docstring describing all 30 actions)"""
```

The LLM sees all 25 params and frequently guesses wrong (e.g., passing `type` instead of `model_type`, or `ingest` instead of `add`).

### The solution

Add a `@tool_group` decorator to the Python SDK that lets you define a namespace of related actions, each with its own typed signature:

```python
from protomcp import tool_group, action, ToolResult

@tool_group("data", description="Manage data in the project's feature store")
class DataTools:

    @action("add", description="Ingest a dataset (CSV/parquet/Excel)")
    def add(self, data_path: str, join_on: list[str] | None = None,
            prefix: str | None = None, auto_clean: bool = False) -> ToolResult:
        return ToolResult(result=handle_add(data_path, join_on, prefix, auto_clean))

    @action("profile", description="Profile the features dataset")
    def profile(self, category: str | None = None) -> ToolResult:
        return ToolResult(result=handle_profile(category))

    @action("derive_column", description="Derive a new column from a pandas expression")
    def derive_column(self, name: str, expression: str,
                      group_by: str | None = None, dtype: str | None = None) -> ToolResult:
        return ToolResult(result=handle_derive(name, expression, group_by, dtype))
```

### How it maps to MCP

Two strategies (configurable, default to Option A):

**Option A: Single tool with discriminated union schema.** The tool group registers as one MCP tool named `data`. The JSON schema uses `oneOf` with the `action` field as the discriminator:

```json
{
  "type": "object",
  "properties": {
    "action": {"type": "string", "enum": ["add", "profile", "derive_column", ...]}
  },
  "required": ["action"],
  "oneOf": [
    {
      "properties": {
        "action": {"const": "add"},
        "data_path": {"type": "string"},
        "join_on": {"type": "array", "items": {"type": "string"}},
        "prefix": {"type": "string"},
        "auto_clean": {"type": "boolean", "default": false}
      },
      "required": ["action", "data_path"]
    },
    {
      "properties": {
        "action": {"const": "profile"},
        "category": {"type": "string"}
      },
      "required": ["action"]
    }
  ]
}
```

**Option B: Separate tools with namespaced names.** Each action becomes its own MCP tool: `data.add`, `data.profile`, `data.derive_column`. Simpler schemas but more tools in the tool list.

### Implementation specifics

**Python SDK changes:**

1. New file: `sdk/python/src/protomcp/group.py`
2. `@tool_group(name, description)` class decorator — registers the class
3. `@action(name, description)` method decorator — registers each method within the group
4. Schema generation: iterate `@action` methods, generate per-action schema from type hints, combine into discriminated union or separate tools
5. `get_registered_tools()` in `tool.py` must also include tools from groups
6. Dispatch in `runner.py`: `_handle_call_tool` checks if the tool name matches a group, parses `action` from args, dispatches to the correct method

**Proto changes:** None needed — tool groups are a Python SDK concept that maps to standard `ToolDefinition` messages.

**Go bridge changes:** None — the Go side just sees normal tools.

### What harness-ml gets

- LLM sees clean per-action schemas instead of 25-param blobs
- Invalid parameter combinations are impossible at the schema level
- Docstrings can be per-action instead of one massive wall
- Eliminates the manual `dispatch(action, **kwargs)` pattern in every handler

---

## Item 2: Python-Side Middleware

### The problem

Every harness-ml tool is wrapped in `_safe_tool`, a 100-line async wrapper in `mcp_server.py` (lines 272-372) that handles:

1. **Event emission** — emits "running" event to SQLite before the call, "success"/"error" after
2. **Auto-install** — catches `ModuleNotFoundError`, runs `pip install`, retries once
3. **Error formatting** — catches `ValueError`, `JSONDecodeError`, generic `Exception`, formats as markdown
4. **Studio URL logging** — logs the Studio URL once per session
5. **Timing** — measures wall-clock duration for each call

Every new tool must be decorated with `@_safe_tool`. Miss it and you get raw exceptions instead of formatted errors.

protomcp already has middleware in the proto spec and Go bridge (`MiddlewareInterceptRequest`/`MiddlewareInterceptResponse`), but it executes **in the Go process** via a round-trip over the socket. For Python-side concerns (event emission, auto-install, error formatting), the round-trip is unnecessary overhead and the middleware handler doesn't have access to Python objects like the event emitter.

### The solution

Add **Python-local middleware** that runs in the Python process, wrapping tool handlers directly. Separate from the existing Go-bridge middleware which is for cross-process concerns.

```python
from protomcp import local_middleware

@local_middleware(priority=10)
def event_emission(ctx, tool_name, args, next_handler):
    """Emit tool call events to SQLite for Studio observability."""
    emitter = get_emitter()
    emitter.emit(tool=tool_name, status="running", params=args)
    start = time.monotonic()
    try:
        result = next_handler(ctx, args)
        elapsed_ms = int((time.monotonic() - start) * 1000)
        emitter.emit(tool=tool_name, status="success", result=result[:20000], duration_ms=elapsed_ms)
        return result
    except Exception as e:
        elapsed_ms = int((time.monotonic() - start) * 1000)
        emitter.emit(tool=tool_name, status="error", result=str(e)[:20000], duration_ms=elapsed_ms)
        raise

@local_middleware(priority=20)
def auto_install(ctx, tool_name, args, next_handler):
    """Auto-install missing Python packages and retry once."""
    try:
        return next_handler(ctx, args)
    except ModuleNotFoundError as e:
        if install_package(e.name):
            return next_handler(ctx, args)
        raise

@local_middleware(priority=90)
def error_formatter(ctx, tool_name, args, next_handler):
    """Convert exceptions to markdown error strings."""
    try:
        return next_handler(ctx, args)
    except ValueError as e:
        return ToolResult(result=f"**Error**: {e}", is_error=True)
    except Exception as e:
        tb = traceback.format_exception(e)[-2000:]
        return ToolResult(result=f"**Error**: {e}\n\n```\n{tb}\n```", is_error=True)
```

### Implementation specifics

**Python SDK changes:**

1. New file: `sdk/python/src/protomcp/local_middleware.py`
2. `@local_middleware(priority=N)` decorator — registers a function in a local middleware chain
3. `get_local_middleware()` returns sorted list by priority
4. In `runner.py` `_handle_call_tool`: before calling the handler, build a middleware chain. Each middleware calls `next_handler` to proceed or returns early to short-circuit.
5. The chain is: middleware[0] → middleware[1] → ... → actual_handler

**Key design decisions:**
- Local middleware runs **in-process** — no proto round-trip, has access to all Python state
- Separate from Go-bridge middleware (`@middleware`) which handles cross-process concerns
- `next_handler` signature: `(ctx: ToolContext, args: dict) -> ToolResult`
- Middleware receives both `tool_name` and unparsed `args` dict so it can log/modify them
- Priority ordering: lower numbers run first (outermost), higher numbers run last (closest to handler)

### What harness-ml gets

- Eliminates the entire `_safe_tool` wrapper (100 lines → 0)
- Event emission, auto-install, and error formatting become composable, testable units
- New tools automatically get all middleware without remembering `@_safe_tool`

---

## Item 3: Union/Complex Type Schema Generation

### The problem

Several harness-ml parameters use union types:

```python
params: str | dict | None = None       # JSON string or parsed dict
overlay: str | dict | None = None      # same
columns: list[str] | list[dict] | None = None  # list of names or list of config objects
items: str | list | None = None        # JSON string or parsed list
```

FastMCP + Pydantic v2 generates proper JSON Schema `anyOf` unions for these. protomcp's current schema generator (`tool.py` `_generate_schema`) maps Python types to JSON Schema via a simple dict:

```python
_PYTHON_TYPE_TO_JSON_SCHEMA = {
    str: "string", int: "integer", float: "number", bool: "boolean",
    list: "array", dict: "object",
}
```

`str | dict | None` would just become `"string"`. The LLM doesn't know it can also pass a dict.

### The solution

Upgrade `_generate_schema` to handle:

1. **`typing.Union` / `X | Y`** → JSON Schema `anyOf`
2. **`typing.Optional[X]`** → `anyOf` with null
3. **`list[str]`** → `{"type": "array", "items": {"type": "string"}}`
4. **`list[dict]`** → `{"type": "array", "items": {"type": "object"}}`
5. **`dict[str, Any]`** → `{"type": "object", "additionalProperties": true}`
6. **Nested unions** — `list[str] | list[dict] | None` → proper `anyOf` with array variants
7. **`Literal["a", "b", "c"]`** → `{"type": "string", "enum": ["a", "b", "c"]}`
8. **Dataclass params** → `{"type": "object", "properties": {...}}` (already partially supported)

### Implementation specifics

Replace `_python_type_to_json` in `sdk/python/src/protomcp/tool.py` with a recursive `_type_to_schema(hint) -> dict` function:

```python
def _type_to_schema(hint) -> dict:
    origin = typing.get_origin(hint)
    args = typing.get_args(hint)

    # Union (X | Y | None)
    if origin is typing.Union:
        non_none = [a for a in args if a is not type(None)]
        schemas = [_type_to_schema(a) for a in non_none]
        if type(None) in args:
            schemas.append({"type": "null"})
        if len(schemas) == 1:
            return schemas[0]
        return {"anyOf": schemas}

    # list[X]
    if origin is list:
        if args:
            return {"type": "array", "items": _type_to_schema(args[0])}
        return {"type": "array"}

    # dict[K, V]
    if origin is dict:
        schema = {"type": "object"}
        if len(args) == 2:
            schema["additionalProperties"] = _type_to_schema(args[1])
        return schema

    # Literal["a", "b"]
    if origin is typing.Literal:
        return {"type": "string", "enum": list(args)}

    # Primitives
    return {"type": _PYTHON_TYPE_TO_JSON_SCHEMA.get(hint, "string")}
```

Also update `_generate_schema` to use `_type_to_schema` instead of the flat lookup, and handle `default` values properly (include them in the schema, mark param as not-required if default exists).

### What harness-ml gets

- `params: str | dict | None` generates `{"anyOf": [{"type": "string"}, {"type": "object"}, {"type": "null"}]}` — LLM knows it can pass either
- `list[str]` generates proper array-of-string schema — LLM stops passing comma-separated strings
- `Literal` types enable enum validation at the schema level

---

## Item 4: Server-Level Shared Context

### The problem

Every harness-ml tool takes `project_dir: str | None = None`. Every handler calls `resolve_project_dir(project_dir)`. It's the same parameter, same default logic, same resolution — repeated across 8 tools and 80+ handler functions.

```python
# This appears in every single tool signature:
project_dir: str | None = None

# And every handler does this:
from harnessml.plugin.handlers._common import resolve_project_dir
def _handle_add(*, data_path, project_dir, **_kwargs):
    proj = resolve_project_dir(project_dir)  # same in every handler
```

### The solution

Add a **server context** concept — values that are computed once and injected into all tool handlers:

```python
from protomcp import server_context

@server_context("project_dir")
def resolve_project_dir(args: dict) -> Path:
    """Resolve from explicit param, env var, or cwd."""
    explicit = args.pop("project_dir", None)  # remove from args before handler sees them
    if explicit:
        return Path(explicit).resolve()
    env = os.environ.get("HARNESS_PROJECT_DIR")
    if env:
        return Path(env).resolve()
    return Path.cwd()
```

Then handlers receive it as a typed parameter:

```python
@action("add", description="Ingest a dataset")
def add(self, data_path: str, project_dir: Path) -> ToolResult:
    # project_dir is already resolved — no manual resolution needed
    ...
```

### Implementation specifics

**Python SDK changes:**

1. New file: `sdk/python/src/protomcp/server_context.py`
2. `@server_context(param_name)` — registers a resolver function
3. Resolvers receive the raw `args` dict and can pop values from it (removing them from what the handler sees)
4. Resolvers return a value that gets injected into the handler's kwargs by name
5. In `runner.py` `_handle_call_tool`: before calling the handler, run all context resolvers against `args`, inject results
6. Context params are **not included in the tool's JSON schema** (they're implicit), unless marked `expose=True`

**Design consideration:** `project_dir` specifically should probably still appear in the schema (so the LLM can explicitly pass it), but the resolver provides the default. Use `@server_context("project_dir", expose=True)` to include it in schemas with the resolver as the default.

### What harness-ml gets

- Removes `project_dir` boilerplate from every tool signature and handler
- `resolve_project_dir` logic lives in one place instead of being called 80+ times
- Same pattern works for any server-wide concern (e.g., database connection, config object)

---

## Item 5: Structured Telemetry Sink

### The problem

Harness-ml emits events to SQLite so the Studio dashboard can show real-time MCP activity. This is currently wired manually:

1. `_get_emitter()` lazily creates a `EventEmitter` in `mcp_server.py`
2. `_safe_tool` calls `emitter.emit(tool=..., action=..., status="running")` before each call
3. `_safe_tool` calls `emitter.emit(status="success")` or `emitter.emit(status="error")` after
4. Handlers access the emitter via thread-local `_common.get_active_emitter()` for sub-events
5. Progress callbacks also report to the emitter via `emitter.progress()`

The emitter is set globally via `_common.set_active_emitter(emitter)` before each tool call — a thread-safety concern that's managed via locks.

### The solution

Add a **telemetry hook** system where the framework automatically emits structured tool-call events to a configurable sink:

```python
from protomcp import telemetry_sink

@telemetry_sink
def studio_events(event: ToolCallEvent):
    """Forward tool call events to Studio's SQLite event store."""
    emitter = get_emitter()
    if event.phase == "start":
        emitter.emit(tool=event.tool_name, action=event.action, params=event.args, status="running")
    elif event.phase == "success":
        emitter.emit(tool=event.tool_name, action=event.action, result=event.result,
                     duration_ms=event.duration_ms, status="success")
    elif event.phase == "error":
        emitter.emit(tool=event.tool_name, action=event.action, result=str(event.error),
                     duration_ms=event.duration_ms, status="error")
    elif event.phase == "progress":
        emitter.progress(current=event.progress, total=event.total, message=event.message)
```

### Implementation specifics

**Python SDK changes:**

1. New file: `sdk/python/src/protomcp/telemetry.py`
2. `ToolCallEvent` dataclass with fields: `tool_name`, `action` (parsed from args if present), `args`, `result`, `error`, `duration_ms`, `phase` (start/success/error/progress), `progress`, `total`, `message`
3. `@telemetry_sink` decorator — registers a function that receives `ToolCallEvent`
4. In `runner.py`: emit events at tool call start, completion, error, and progress notification
5. Telemetry sinks are called **synchronously in a try/except** — failures are swallowed (fail-safe, like harness-ml's current emitter)
6. Multiple sinks supported (e.g., SQLite + file log + webhook)

**Relationship to middleware:** Telemetry is **not** middleware — it's observe-only. It cannot modify args or results. This keeps it simple and fail-safe. Middleware is for transformation; telemetry is for observation.

### What harness-ml gets

- Eliminates the `set_active_emitter` / `get_active_emitter` thread-local dance
- Event emission moves from `_safe_tool` wrapper to framework-level
- Progress events automatically flow to both MCP client and telemetry sinks
- New tools get telemetry for free

---

## Item 6: Declarative Per-Action Validation

### The problem

Harness-ml has a manual validation layer in `_validation.py` with:

- `validate_required(value, param_name)` — check mandatory params
- `validate_enum(value, valid_set, param_name)` — fuzzy match with "did you mean?"
- `validate_cross_params(rules)` — dependency rules like "if mode is regressor, cdf_scale should be set"
- `collect_hints(action, tool, **kwargs)` — non-blocking advisory messages

Every handler starts with manual validation calls:

```python
def _handle_add(*, data_path, project_dir, **_kwargs):
    err = validate_required(data_path, "data_path")
    if err:
        return err
    # ... actual logic
```

### The solution

Make validation declarative on the `@action` decorator:

```python
@action("add",
    description="Ingest a dataset",
    requires=["data_path"],
    enum_fields={"strategy": ["median", "mean", "mode", "zero", "value"]},
    hints={
        "join_on": {
            "condition": lambda args: not args.get("join_on"),
            "message": "No join_on specified. Data will be appended as new rows."
        }
    }
)
def add(self, data_path: str, join_on: list[str] | None = None, strategy: str = "median"):
    # No manual validation needed — framework handles it
    ...
```

### Implementation specifics

**Python SDK changes:**

1. Extend `@action` decorator to accept `requires`, `enum_fields`, `cross_rules`, `hints`
2. Before dispatching to handler, run validation:
   - Check `requires` — return error if any required param is None/empty
   - Check `enum_fields` — use `difflib.get_close_matches` for fuzzy match (same as harness-ml's current impl)
   - Check `cross_rules` — list of `(condition_fn, error_message)` tuples
3. After successful handler execution, check `hints` — append non-blocking advisory messages
4. Validation errors use the `ToolError` proto message with `error_code`, `message`, `suggestion`, `retryable` fields — this is richer than FastMCP's plain string errors

**Fuzzy enum matching:** Built into the framework, not user code. When an enum field gets an invalid value, the error automatically includes "Did you mean X?" if a close match exists. This is a common pattern that every action-based MCP server needs.

### What harness-ml gets

- Eliminates ~50 manual `validate_required()` calls across handlers
- Fuzzy enum matching is automatic for any field with declared valid values
- Cross-parameter hints (like "regressor mode needs cdf_scale") are declarative
- `ToolError` gives the LLM structured error info (error code, suggestion, retryable flag) instead of parsing markdown strings

---

## Item 7: Sidecar Process Management

### The problem

Harness-ml auto-starts Studio as a detached subprocess (~80 lines in `mcp_server.py` lines 31-164):

- Health-check via HTTP
- PID file management
- Version mismatch detection and restart
- Graceful shutdown with SIGTERM → wait → SIGKILL

This is generic process lifecycle code that has nothing to do with MCP or ML.

### The solution

Add a `@sidecar` decorator:

```python
from protomcp import sidecar

@sidecar(
    name="studio",
    command=["python", "-m", "harnessml.studio.cli", "--port", "8421"],
    health_check="http://localhost:8421/api/health",
    start_on="first_tool_call",   # or "server_start"
    restart_on_version_mismatch=True,
)
def studio_sidecar():
    """Harness Studio companion dashboard."""
    pass
```

### Implementation specifics

**Python SDK changes:**

1. New file: `sdk/python/src/protomcp/sidecar.py`
2. `@sidecar(name, command, health_check, start_on, restart_on_version_mismatch)` — registers a sidecar definition
3. Sidecar lifecycle:
   - `start_on="first_tool_call"`: start when the first tool call arrives
   - `start_on="server_start"`: start when `protomcp.run()` is called
4. Health check: HTTP GET to the URL, expect 200. Optional: parse version from JSON response and compare to expected.
5. PID management: write to `~/.protomcp/sidecars/{name}.pid`, clean up on shutdown
6. Restart: SIGTERM → 3s wait → SIGKILL → relaunch
7. Shutdown hook: kill sidecars when the Python process exits (via `atexit`)

**Go-side support:** The Go bridge could also manage sidecars (since it's the long-running process), but Python-side is simpler for harness-ml's case since Studio is a Python process.

### What harness-ml gets

- Eliminates ~80 lines of manual process management in `mcp_server.py`
- PID management, health checking, version mismatch restart — all handled by framework
- Clean shutdown on server exit

---

## Item 8: Handler Module Auto-Discovery

### The problem

Harness-ml's hot-reload pattern manually loads handler modules:

```python
def _load_handler(module_name: str):
    mod = importlib.import_module(f"harnessml.plugin.handlers.{module_name}")
    if _DEV_MODE:
        importlib.reload(mod)
    return mod
```

Each tool function calls `_load_handler("models").dispatch(action, ...)`. This is manual wiring that breaks if you rename a handler file or forget to add the load call.

### The solution

Support a `handlers_dir` configuration that auto-discovers handler modules:

```python
# In server.py
from protomcp import configure

configure(
    handlers_dir="handlers/",   # auto-discover all .py files
    hot_reload=True,            # re-import on each call in dev mode
)
```

Each handler file exports a tool group:

```python
# handlers/data.py
from protomcp import tool_group, action

@tool_group("data", description="Manage data")
class DataTools:
    @action("add")
    def add(self, data_path: str): ...
```

protomcp scans `handlers_dir`, imports all modules, and registers their tool groups. On reload, it re-imports all modules in the directory.

### Implementation specifics

**Python SDK changes:**

1. Add `configure(handlers_dir=..., hot_reload=...)` function
2. In `runner.py` at startup: scan directory, import all `.py` files, collect registered tools/groups
3. On `ReloadRequest`: re-scan directory, clear registries, re-import all modules, re-register
4. File-level granularity: if a single file changes, only reimport that file (optimization, not required for v1)

**Interaction with pmcp dev:** `pmcp dev` already watches for file changes and sends `ReloadRequest`. This feature just makes the Python side respond correctly to reloads by re-importing the handlers directory.

### What harness-ml gets

- Drop a new handler file in `handlers/` → tool automatically available
- Rename a handler file → tool automatically updated
- No manual `_load_handler` wiring
- Hot-reload works at the handler level, not just the top-level server file

---

## Priority and Dependencies

```
Item 3 (Union types)           ← No dependencies, do first
    ↓
Item 1 (Tool groups)           ← Depends on Item 3 for proper per-action schemas
    ↓
Item 6 (Declarative validation) ← Best implemented as part of tool groups
    ↓
Item 4 (Server context)        ← Works with tool groups for implicit params
    ↓
Item 2 (Local middleware)      ← Independent, but most useful after tool groups exist
    ↓
Item 5 (Telemetry)             ← Can be implemented as a special local middleware
    ↓
Item 7 (Sidecars)              ← Independent, low priority
Item 8 (Handler discovery)     ← Independent, nice-to-have
```

### Recommended implementation order

1. **Item 3** — Union type schemas (half-day, unblocks everything else)
2. **Item 1** — Tool groups with per-action schemas (2-3 days, biggest impact)
3. **Item 2** — Local middleware (1 day, eliminates `_safe_tool`)
4. **Item 6** — Declarative validation (1 day, built into tool groups)
5. **Item 4** — Server context (half-day, small but eliminates noise)
6. **Item 5** — Telemetry sinks (1 day)
7. **Item 7** — Sidecar management (half-day)
8. **Item 8** — Handler auto-discovery (half-day)

### What "done" looks like

When all 8 items are implemented, harness-ml's `mcp_server.py` goes from ~1200 lines to ~50 lines (just imports and a `configure()` call). The 8 handler files stay roughly the same size but lose all manual validation boilerplate. The `_safe_tool` wrapper, `_validation.py`, `_common.py`, and all Studio lifecycle code are eliminated entirely.

The agent gets clean per-action schemas, proper union types, fuzzy enum matching, and structured errors — all from the framework, not from 500 lines of custom harness-ml code.
