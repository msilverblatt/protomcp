# protomcp Design Spec

## Problem Statement

MCP (Model Context Protocol) has a powerful protocol, but it forces every tool developer to implement that protocol themselves. This creates three problems:

1. **Language lock-in**: Tools are almost exclusively Python and TypeScript because nobody wants to implement the MCP protocol layer in other languages just to expose a function.
2. **Restart hell**: During development, any code change requires restarting the MCP server process, which disconnects the client, loses conversation context, and breaks flow. This is the #1 developer pain point in the MCP ecosystem.
3. **Static tool lists**: There's no practical way to dynamically show/hide tools based on application state. Developers resort to prompt engineering ("don't use this tool until...") which wastes tokens and is unreliable.

## Solution

**protomcp** is a language-agnostic MCP runtime. A single precompiled Go binary handles all MCP protocol concerns. Developers write plain functions in any language, decorate them, and protomcp handles transport, hot-reload, and dynamic tool management.

The key architectural insight: the MCP protocol layer (JSON-RPC, transport negotiation, capability advertisement, notifications) almost never changes during development. What changes is the tool logic. protomcp splits on that boundary — the protocol layer is a stable Go binary, the tool logic is hot-reloadable user code in any language.

## Architecture

```
[Host/Client]
    | MCP protocol (stdio / SSE / streamable HTTP / gRPC / WebSocket)
[protomcp binary (Go)]
    | protobuf over unix socket
[tool process (Python / TS / Go / Rust / anything)]
```

### Component Responsibilities

**Go Binary (the runtime):**
- Speaks all five MCP transports to the host
- Communicates with tool processes over unix socket using protobuf
- Watches files for changes, signals tool process to reload
- Manages tool list state (enable/disable/allow/block/query)
- Fires `notifications/tools/list_changed` on any tool set change
- Proxies `tools/list` and `tools/call` — never interprets tool schemas
- Spawns and manages the tool process lifecycle (see Tool Process Lifecycle below)
- Handles middleware/hooks (auth, logging, rate limiting, custom interceptors)
- Structured error handling with agent-friendly error types
- Proxies progress notifications (`notifications/progress`) between tool process and host
- Manages async task lifecycle (`tasks/get`, `tasks/result`, `tasks/cancel`, `tasks/list`)
- Forwards cancellation requests (`notifications/cancelled`) to tool process
- Exposes server logging via MCP `notifications/message` (structured log forwarding)
- Graceful degradation — survives client disconnects, transport errors, tool process crashes
- On reload: waits for in-flight calls by default, immediate reload opt-in via `--hot-reload immediate`

**Protobuf Spec (the contract):**
- Single `.proto` file defining all messages between Go binary and tool process
- Generates types for Go, Python, TypeScript, Rust (and any other language with protobuf support)
- Any language that can speak protobuf over a unix socket can expose MCP tools

**Language Libraries (first-class: Python, TypeScript, Go, Rust):**
- Decorator/registration API with automatic schema generation from language-native types
  - Python: generated from type annotations at runtime via `inspect`
  - TypeScript: uses Zod schemas for runtime type information (TS types are erased at compile time). The `tool()` function accepts a Zod schema for args, providing both runtime validation and JSON schema generation.
  - Go: generated from struct tags
  - Rust: generated via derive macros
- `ToolResult` type with optional `enable_tools` / `disable_tools` fields
- `tool_manager` client for programmatic list control
- Progress reporting API (`report_progress(progress, total?, message?)`)
- Support for async tools via `task_support` parameter on `@tool()` decorator
- Module reload handler (language-specific mechanism)
- Unix socket client (generated from protobuf)

## Internal Protocol

Communication between the Go binary and tool process uses protobuf over a unix socket.

### Wire Format

Messages are **length-prefixed**: each message is preceded by a 4-byte big-endian uint32 indicating the byte length of the serialized protobuf message that follows.

All messages are wrapped in a single `Envelope` message with a `oneof` discriminator:

```protobuf
message Envelope {
  oneof msg {
    // Go -> Tool Process
    ReloadRequest reload = 1;
    ListToolsRequest list_tools = 2;
    CallToolRequest call_tool = 3;

    // Tool Process -> Go
    ReloadResponse reload_response = 4;
    ToolListResponse tool_list = 5;
    CallToolResponse call_result = 6;
    EnableToolsRequest enable_tools = 7;
    DisableToolsRequest disable_tools = 8;
    SetAllowedRequest set_allowed = 9;
    SetBlockedRequest set_blocked = 10;
    GetActiveToolsRequest get_active_tools = 11;
    BatchUpdateRequest batch = 12;
    ActiveToolsResponse active_tools = 13;

    // Progress, cancellation, logging, tasks
    ProgressNotification progress = 16;
    CancelRequest cancel = 17;
    LogMessage log = 18;
    CreateTaskResponse create_task = 19;
    TaskStatusRequest task_status = 20;
    TaskStatusResponse task_status_response = 21;
    TaskResultRequest task_result = 22;
    TaskCancelRequest task_cancel = 23;
  }
  // Correlation ID for matching requests to responses.
  // Required for CallToolRequest/CallToolResponse to support concurrent calls.
  // Optional for other message types.
  string request_id = 14;
}
```

This gives both sides a single deserialization path: read 4 bytes for length, read that many bytes, deserialize as `Envelope`, switch on the `oneof` variant. No type-tag ambiguity, no framing confusion. Every language library gets this for free from protoc.

### Messages: Go -> Tool Process

- `ReloadRequest` — reimport modules, re-register tools
- `ListToolsRequest` — return current tool definitions with schemas
- `CallToolRequest(name, args)` — execute a tool and return the result

### Messages: Tool Process -> Go

- `ReloadResponse(success, error?)` — acknowledge reload with success/failure status and optional error message (e.g., syntax error, import error)
- `ToolListResponse(tools[])` — current tool definitions with names, descriptions, and JSON schemas
- `CallToolResponse(result, enable_tools?, disable_tools?)` — tool execution result with optional tool list mutations
- `EnableToolsRequest(tool_names[])` — add tools to the active set (delta)
- `DisableToolsRequest(tool_names[])` — remove tools from the active set (delta)
- `SetAllowedRequest(tool_names[])` — only these tools are visible (allowlist)
- `SetBlockedRequest(tool_names[])` — everything visible except these (blocklist)
- `GetActiveToolsRequest()` — query current active tool set
- `BatchUpdateRequest(enable?, disable?, allow?, block?)` — atomic multi-operation update, single `list_changed` notification
- `ActiveToolsResponse(tool_names[])` — response to any tool list control command

### Messages: Bidirectional (Progress, Cancellation, Logging)

- `ProgressNotification(progress_token, progress, total?, message?)` — tool process reports progress on a long-running call; Go binary proxies as MCP `notifications/progress`
- `CancelRequest(request_id)` — Go binary forwards MCP `notifications/cancelled` to tool process; tool process should abort the in-flight call and return a cancellation error
- `LogMessage(level, logger?, data)` — tool process emits a structured log; Go binary forwards as MCP `notifications/message` with RFC 5424 severity level

### Messages: Task (Async) Lifecycle

- `CreateTaskResponse(task_id)` — tool process returns a task ID instead of a result for async tools; Go binary responds to client with `CreateTaskResult`
- `TaskStatusRequest(task_id)` — Go binary forwards client `tasks/get` poll to tool process
- `TaskStatusResponse(task_id, state, progress?, message?)` — tool process reports current task state (`running`, `completed`, `failed`, `cancelled`)
- `TaskResultRequest(task_id)` — Go binary forwards client `tasks/result` to tool process
- `TaskCancelRequest(task_id)` — Go binary forwards client `tasks/cancel` to tool process

## Tool Process Lifecycle

### Spawning

The Go binary spawns the tool process as a child process. The runtime is determined by file extension:

| Extension | Command |
|-----------|---------|
| `.py` | `python <file>` (or `python3`, respects `PROTOMCP_PYTHON` env var) |
| `.ts` | `npx tsx <file>` (respects `PROTOMCP_NODE` env var) |
| `.js` | `node <file>` |
| `.go` | `go run <file>` |
| `.rs` | `cargo run <file>` |
| other | Treated as an executable binary, run directly |

An explicit `--runtime <command>` flag overrides extension-based detection.

### Startup Handshake

1. Go binary creates a unix socket at `$XDG_RUNTIME_DIR/protomcp/<pid>.sock` (fallback: `/tmp/protomcp/<pid>.sock`)
2. Go binary spawns tool process, passing the socket path via `PROTOMCP_SOCKET` environment variable
3. Tool process connects to the socket
4. Go binary sends `ListToolsRequest`
5. Tool process responds with `ToolListResponse`
6. Go binary is now ready to accept MCP connections from the host

### Crash Recovery

If the tool process exits unexpectedly:
- Go binary logs the error with exit code and any stderr output
- All pending tool calls receive an error response
- Go binary attempts to restart the tool process (up to 3 retries with exponential backoff)
- If restart succeeds, Go binary re-fetches the tool list and fires `list_changed` if it changed
- If all retries fail, Go binary enters degraded mode: `tools/list` returns empty, all `tools/call` requests return an error explaining the tool process is down
- The MCP connection to the host stays alive throughout — the client is never disconnected

### Production Mode (`protomcp run`)

Dynamic tool list management (enable/disable/allow/block) works identically in `run` mode. Only file-watching hot-reload is disabled.

## Dynamic Tool Lists

### Design Philosophy

Instead of instructing an agent "don't use tool X until condition Y" (which wastes tokens and is unreliable), protomcp makes tools invisible until they should be available. The agent can't misuse what it can't see.

### Update Vectors

**1. Tool-call-driven (inline mutations):**

Tool return values can include `enable_tools` and `disable_tools` fields. The Go binary intercepts these before proxying the result to the host, updates the active tool set, and fires `list_changed`.

```python
@tool(description="Verify credentials")
def auth_check(token: str) -> ToolResult:
    if verify(token):
        return ToolResult(
            result="Authenticated",
            enable_tools=["delete_doc", "admin_panel"],
            disable_tools=["auth_check"]
        )
```

**2. Event-driven (programmatic control):**

Tool list can be modified at any time from event handlers, background tasks, lock managers, etc.

```python
from protomcp import tool_manager

def on_lock_acquired(resource):
    tool_manager.disable([f"edit_{resource}"])

def on_lock_released(resource):
    tool_manager.enable([f"edit_{resource}"])
```

### Control Modes

| Mode | Method | Behavior |
|------|--------|----------|
| Delta | `enable(tools)` / `disable(tools)` | Add/remove from current active set |
| Allowlist | `set_allowed(tools)` | Only these tools visible, all others hidden |
| Blocklist | `set_blocked(tools)` | All tools visible except these |
| Query | `get_active_tools()` | Returns current active tool set |
| Batch | `batch(enable?, disable?, allow?, block?)` | Atomic multi-op, single `list_changed` |

### Mode Interaction Semantics

The tool list operates in one of three modes at any time: **open** (default), **allowlist**, or **blocklist**.

- **Open mode** (default): All registered tools are active. `enable()`/`disable()` apply deltas.
- **Allowlist mode**: Entered by calling `set_allowed()`. Only listed tools are active. `enable()` adds to the allowlist. `disable()` removes from the allowlist. `set_blocked()` switches to blocklist mode (replaces, does not compose).
- **Blocklist mode**: Entered by calling `set_blocked()`. All tools active except listed ones. `enable()` removes from the blocklist. `disable()` adds to the blocklist. `set_allowed()` switches to allowlist mode (replaces, does not compose).

Calling `set_allowed([])` or `set_blocked([])` with an empty list resets to open mode.

In `batch()`, if both `allow` and `block` are specified, the operation is rejected with an error. Delta operations (`enable`/`disable`) in a batch are applied after the mode-setting operation (`allow` or `block`), if present.

All control methods work both inline (from tool return values) and programmatically (from event handlers). All tool list state lives in the Go binary — language libraries send commands, they don't track state.

## Hot Reload

### Mechanism

1. Go binary watches the tool file(s) for changes using filesystem events
2. On change detected, Go binary sends `Reload` message to tool process
3. Tool process reimports modules using language-specific mechanism:
   - Python: `importlib.reload()`
   - TypeScript/Node: module cache invalidation
   - Go: subprocess restart (Go plugins cannot be unloaded/reloaded once loaded)
   - Rust: subprocess restart
4. Tool process sends `ReloadResponse` indicating success or failure (with error details if e.g., syntax error). On failure, the Go binary logs the error and keeps serving the previous tool set.
5. On success, tool process sends updated `ToolListResponse` to Go binary
6. Go binary compares to previous list — if tools added/removed/changed, fires `list_changed`
7. Client re-fetches tool list, sees updated tools
8. No transport disconnection at any point — the MCP connection stays alive

### In-Flight Call Handling

- **Default**: Wait for any active tool call to finish before reloading (safe)
- **Opt-in**: `--hot-reload immediate` reloads immediately without waiting. For interpreted languages (Python, TS), module reimport replaces functions in-place, so in-flight calls may execute a mix of old and new code or fail. This mode is intended for developers who know their tools are stateless and short-lived. The Go binary logs a warning when immediate reload interrupts an in-flight call.
- **Timeout**: A configurable `--call-timeout <duration>` flag (default: 5m) prevents stuck tool calls from blocking reload indefinitely. If a call exceeds the timeout, it is cancelled and an error is returned to the client.

### What Triggers a Reload

- File modification detected by the Go binary's file watcher
- Any file in the watched directory (or explicit file list) matching relevant extensions

## Transport Support

All five transports at launch:

| Transport | Use Case | Notes |
|-----------|----------|-------|
| stdio | Local, Claude Desktop, most clients | Default. Go binary reads stdin, writes stdout |
| Streamable HTTP | Modern remote connections | Single HTTP endpoint, optional SSE streaming |
| SSE | Legacy remote connections | Deprecated in spec but still widely used |
| gRPC | Google ecosystem, high-performance | Uses gRPC's HTTP/2 framework, separate from the internal protobuf-over-socket protocol. Shares `.proto` message definitions where applicable but serves a different role. |
| WebSocket | Long-lived bidirectional, real-time | Session persistence across interruptions |

Transport is selected via `--transport` flag. Default is `stdio`.

## Middleware / Hooks

The Go binary supports an interceptor chain for cross-cutting concerns. Middleware runs in the Go binary regardless of tool language — write it once, applies to all tools.

### Built-in Middleware

- **Logging**: Structured request/response logging with configurable verbosity
- **Error handling**: Catches tool process errors, formats agent-friendly error messages with suggestions

### Custom Middleware

Users can register custom middleware that intercepts tool calls before/after execution. Middleware has access to:
- Tool name and arguments (before)
- Tool result (after)
- Tool list state
- Request metadata

### Middleware Versioning

Custom middleware is a v1.1 feature. In v1, only the built-in middleware (logging, error handling) ships. The middleware hook points will be designed in v1 to avoid breaking changes when custom middleware lands in v1.1.

## Structured Error Handling

Tool errors should be agent-friendly by default. The protobuf spec includes a structured error type:

- `error_code`: machine-readable error category
- `message`: human/agent-readable description
- `suggestion`: actionable next step ("User not found. Try searching by email instead.")
- `retryable`: boolean indicating if the operation can be retried

The Go binary formats these consistently before sending to the host, regardless of how the tool process reported the error.

## Progress Notifications

Long-running tools can report incremental progress to the client.

### Protocol Flow

1. Client sends `tools/call` with `_meta.progressToken`
2. Go binary forwards `CallToolRequest` to tool process, including the `progress_token`
3. Tool process sends `ProgressNotification` messages as work progresses
4. Go binary proxies each as MCP `notifications/progress` to the client
5. Tool process eventually returns `CallToolResponse` as normal

### Language API

```python
@tool(description="Index all documents")
def index_documents(directory: str, ctx: ToolContext) -> str:
    files = list_files(directory)
    for i, f in enumerate(files):
        index(f)
        ctx.report_progress(progress=i + 1, total=len(files), message=f"Indexing {f}")
    return f"Indexed {len(files)} documents"
```

```typescript
export const indexDocuments = tool({
  description: "Index all documents",
  args: z.object({ directory: z.string() }),
  handler: async (args, ctx) => {
    const files = listFiles(args.directory);
    for (let i = 0; i < files.length; i++) {
      await index(files[i]);
      ctx.reportProgress(i + 1, files.length, `Indexing ${files[i]}`);
    }
    return `Indexed ${files.length} documents`;
  }
});
```

The `ToolContext` (or `ctx`) is injected by the runtime when the tool handler signature requests it. If a tool doesn't need progress, it simply omits the `ctx` parameter.

### Behavior

- If the client did not send a `progressToken`, the Go binary silently drops `ProgressNotification` messages (no error to tool process)
- `total` is optional — omit for indeterminate progress
- `message` is optional — human-readable status for display

## Tasks (Async Execution)

Some tools take too long for a synchronous response. Tasks allow a tool to return immediately with a task ID, then the client polls for status and retrieves the result when ready.

### Declaring Async Tools

```python
@tool(description="Run full analysis", task_support=True)
async def run_analysis(dataset: str) -> str:
    # Long-running work happens here
    result = await heavy_computation(dataset)
    return result
```

When `task_support=True`, the tool's MCP definition includes `execution.taskSupport: true` in its metadata. The Go binary advertises `tasks` capability in its MCP `initialize` response.

### Protocol Flow

1. Client sends `tools/call` for an async-capable tool
2. Go binary forwards `CallToolRequest` to tool process
3. Tool process begins async work and immediately returns `CreateTaskResponse(task_id)`
4. Go binary responds to client with `CreateTaskResult { taskId, state: "running" }`
5. Client polls with `tasks/get(task_id)` → Go binary forwards `TaskStatusRequest` → tool process responds with `TaskStatusResponse`
6. When complete, client sends `tasks/result(task_id)` → Go binary forwards `TaskResultRequest` → tool process responds with `CallToolResponse` containing the final result
7. Client can cancel with `tasks/cancel(task_id)` → Go binary forwards `TaskCancelRequest` → tool process aborts and responds with cancellation acknowledgment

### Task State Machine

```
running → completed
running → failed
running → cancelled (via tasks/cancel)
```

### Task Lifecycle

- Task state lives in the tool process (it owns the computation)
- The Go binary tracks task IDs for routing and can return cached status if the tool process has crashed
- Tasks survive hot-reloads — in-flight tasks continue in the old module context (consistent with default reload behavior)
- Tasks do NOT survive tool process crashes — pending tasks are moved to `failed` state with an error message

## Cancellation

### Protocol Flow

1. Client sends MCP `notifications/cancelled` with `requestId`
2. Go binary looks up the in-flight call by `request_id`
3. Go binary sends `CancelRequest(request_id)` to tool process
4. Tool process should abort the operation and return a `CallToolResponse` with `isError: true` and error code `cancelled`

### Language API

```python
@tool(description="Long operation")
def long_operation(data: str, ctx: ToolContext) -> str:
    for chunk in process_chunks(data):
        if ctx.is_cancelled():
            raise CancelledError("Operation cancelled by client")
        handle(chunk)
    return "Done"
```

The `ctx.is_cancelled()` check is cooperative — the tool process must check periodically. The Go binary sets the cancellation flag when it receives the cancel request.

### Behavior

- If `request_id` doesn't match any in-flight call, the cancellation is silently ignored (per MCP spec)
- Cancellation is best-effort — the tool may complete before checking the flag
- For async tasks, cancellation goes through `tasks/cancel` instead

## Server Logging

Tool processes can emit structured logs that are forwarded to the MCP client as `notifications/message`.

### Protocol Flow

1. Tool process sends `LogMessage(level, logger?, data)` to Go binary
2. Go binary forwards as MCP `notifications/message` with the specified severity level

### Severity Levels (RFC 5424)

`emergency` | `alert` | `critical` | `error` | `warning` | `notice` | `info` | `debug`

### Language API

```python
from protomcp import log

log.info("Processing started", data={"file_count": 42})
log.warning("Rate limit approaching", data={"remaining": 5})
log.debug("Cache hit", logger="cache_layer", data={"key": "user_123"})
```

### Behavior

- The Go binary's `--log-level` flag filters which log messages are forwarded to the client
- Logs below the configured level are dropped (not forwarded)
- `logger` is optional — used to identify the source component
- `data` can be any JSON-serializable value

## Structured Output

Tools can declare an output schema, and the Go binary validates that the result matches before returning to the client.

### Declaring Output Schema

```python
from protomcp import tool
from dataclasses import dataclass

@dataclass
class SearchResult:
    title: str
    url: str
    score: float

@tool(description="Search documents", output_type=SearchResult)
def search(query: str) -> list[SearchResult]:
    return [SearchResult(title="Doc", url="https://...", score=0.95)]
```

```typescript
const SearchResult = z.object({
  title: z.string(),
  url: z.string(),
  score: z.number(),
});

export const search = tool({
  description: "Search documents",
  args: z.object({ query: z.string() }),
  output: z.array(SearchResult),
  handler: (args) => {
    return [{ title: "Doc", url: "https://...", score: 0.95 }];
  }
});
```

### Behavior

- When `output_type` / `output` is specified, the tool's MCP definition includes `outputSchema`
- The Go binary includes `structuredContent` in the `CallToolResponse` alongside the text `content`
- Schema validation happens in the language library before sending the response — validation errors become tool errors

## Tool Metadata

Tools can declare additional metadata that enhances their discoverability and presentation in MCP clients.

### Fields

- `title`: Human-readable display name (distinct from the function name used as the tool ID)
- `readOnlyHint`, `destructiveHint`, `idempotentHint`, `openWorldHint`: Behavioral hints for the client (all optional booleans)

```python
@tool(
    description="Delete a document permanently",
    title="Delete Document",
    destructive=True,
    idempotent=True,
)
def delete_doc(doc_id: str) -> str:
    db.delete(doc_id)
    return f"Deleted {doc_id}"
```

### Behavior

- Metadata is included in the tool's MCP definition under `annotations`
- The Go binary passes through metadata from the tool process — it does not interpret or enforce hints
- Hints are advisory — clients may use them to show confirmation dialogs, group tools, etc.

## CLI

Minimal, hardened, no bloat.

### Commands

- `protomcp dev <file>` — start dev server with file watching, hot-reload, rich log output
- `protomcp run <file>` — production mode, no file watching

### Flags

- `--transport stdio|sse|http|grpc|ws` — transport selection (default: stdio)
- `--hot-reload immediate` — don't wait for in-flight calls before reloading
- `--call-timeout <duration>` — max time for a single tool call (default: 5m)
- `--log-level debug|info|warn|error` — log verbosity (default: info)
- `--socket <path>` — custom unix socket path (default: `$XDG_RUNTIME_DIR/protomcp/<pid>.sock`)
- `--runtime <command>` — override language runtime detection (e.g., `--runtime "python3.12"`)
- `--host <address>` — bind address for network transports (default: localhost)
- `--port <number>` — port for network transports (default: 8080)

### Dev Logs

`protomcp dev` outputs rich structured logs showing:
- Tool registrations on startup
- File changes detected and reloads triggered
- Tool list changes (added/removed/modified tools)
- `list_changed` notifications sent
- Tool call traces (name, args summary, duration, result summary)
- Errors with full context

## Developer Experience

### Getting Started

```bash
brew install protomcp   # or download binary
```

### Minimal Tool File (Python)

```python
from protomcp import tool

@tool(description="Add two numbers")
def add(a: int, b: int) -> int:
    return a + b
```

### Running

```bash
protomcp dev server.py
```

### MCP Client Configuration (one-time)

```json
{
  "mcpServers": {
    "my-tools": {
      "command": "protomcp",
      "args": ["dev", "server.py"]
    }
  }
}
```

This configuration never changes. The Go binary is the MCP server forever. What tools exist is determined by the tool file, which hot-reloads.

### Adding a Tool

Edit `server.py`, add a new decorated function, save. Log shows:

```
[reload] server.py changed, reloading...
[tools]  tool added: multiply
[mcp]    firing list_changed notification
```

Agent immediately sees the new tool. No restart required.

## Language Library API Surface

Each first-class library (Python, TypeScript, Go, Rust) provides the same conceptual API adapted to language idioms.

### Tool Registration

```python
# Python
from protomcp import tool, ToolResult

@tool(description="Search documents by query")
def search_docs(query: str, limit: int = 10) -> list[dict]:
    return db.search(query, limit)
```

```typescript
// TypeScript — uses Zod for runtime schema generation (TS types are erased at compile time)
import { tool, ToolResult } from 'protomcp';
import { z } from 'zod';

export const searchDocs = tool({
  description: "Search documents by query",
  args: z.object({
    query: z.string().describe("Search query"),
    limit: z.number().default(10).describe("Max results to return"),
  }),
  handler: (args) => {
    return db.search(args.query, args.limit);
  }
});
```

### Tool List Control

```python
from protomcp import tool_manager

# Delta
tool_manager.enable(["query_db", "insert_record"])
tool_manager.disable(["initialize_db"])

# Allowlist / Blocklist
tool_manager.set_allowed(["read_only_tool"])
tool_manager.set_blocked(["dangerous_tool"])

# Query
active = tool_manager.get_active_tools()

# Batch (atomic, single list_changed)
tool_manager.batch(
    enable=["tool_a", "tool_b"],
    disable=["tool_c"]
)
```

### Inline Tool List Mutations

```python
@tool(description="Connect to database")
def connect_db(connection_string: str) -> ToolResult:
    db.connect(connection_string)
    return ToolResult(
        result="Connected to database",
        enable_tools=["query_db", "insert_record", "disconnect_db"],
        disable_tools=["connect_db"]
    )
```

### Progress Reporting

```python
@tool(description="Process dataset")
def process_dataset(path: str, ctx: ToolContext) -> str:
    items = load(path)
    for i, item in enumerate(items):
        process(item)
        ctx.report_progress(i + 1, len(items), f"Processing {item.name}")
    return f"Processed {len(items)} items"
```

### Async Tools

```python
@tool(description="Train model", task_support=True)
async def train_model(config: dict) -> str:
    result = await run_training(config)
    return f"Model trained: accuracy={result.accuracy}"
```

### Cancellation

```python
@tool(description="Batch process")
def batch_process(items: list[str], ctx: ToolContext) -> str:
    for item in items:
        if ctx.is_cancelled():
            raise CancelledError("Cancelled by client")
        process(item)
    return "Done"
```

### Server Logging

```python
from protomcp import log

@tool(description="Sync data")
def sync_data(source: str) -> str:
    log.info("Starting sync", data={"source": source})
    result = sync(source)
    log.info("Sync complete", data={"records": result.count})
    return f"Synced {result.count} records"
```

## Documentation

### Framework

Documentation is built with **Starlight** (Astro) — the emerging standard for open source dev tools docs. Starlight ships minimal JavaScript, has built-in search and i18n, and is framework-agnostic (no React/Vue dependency).

### Site Structure

```
docs.protomcp.dev/
├── Getting Started
│   ├── Installation
│   ├── Quick Start (5-minute tutorial: install, write a tool, run)
│   └── How It Works (architecture explainer with diagrams)
├── Guides
│   ├── Writing Tools (Python)
│   ├── Writing Tools (TypeScript)
│   ├── Writing Tools (Go)
│   ├── Writing Tools (Rust)
│   ├── Writing Tools (Other Languages)
│   ├── Dynamic Tool Lists
│   ├── Hot Reload
│   ├── Progress Notifications
│   ├── Async Tasks
│   ├── Cancellation
│   ├── Server Logging
│   ├── Structured Output
│   ├── Error Handling
│   └── Production Deployment
├── Reference
│   ├── CLI Reference
│   ├── Protobuf Spec
│   ├── Python API
│   ├── TypeScript API
│   ├── Go API
│   ├── Rust API
│   └── Configuration
├── Concepts
│   ├── Architecture
│   ├── Tool List Modes (open/allowlist/blocklist)
│   ├── Transport Options
│   └── MCP Protocol (for the curious — not required reading)
└── Community
    ├── Contributing
    ├── Writing a Language Library
    └── Changelog
```

### Documentation Standards

- **Every feature has a guide and a reference page.** Guides explain when and why. Reference pages are exhaustive API docs.
- **Every code example is tested.** Examples are extracted from actual test files or validated in CI. No stale examples.
- **"Writing Tools (Other Languages)" guide** walks through implementing the protobuf contract from scratch in an unsupported language. This is a key differentiator — protomcp's promise is "any language," so the docs must prove it.
- **Architecture diagrams** use Mermaid (Starlight has built-in support) showing the Go binary, unix socket, tool process, and MCP client relationships.
- **Interactive examples** where possible — embedded terminal output showing hot-reload in action, tool list changes, etc.

## Deliverables by Priority

### v1.0 — Core

1. Go binary with all five transports (stdio, SSE, streamable HTTP, gRPC, WebSocket)
2. Protobuf spec (`.proto` file) defining the internal protocol
3. Hot-reload with file watching and `list_changed` notifications
4. Dynamic tool list management (enable/disable/allow/block/query/batch)
5. Structured error handling with agent-friendly error types
6. Progress notifications (proxy `notifications/progress` between tool process and host)
7. Async task support (task lifecycle: create, poll, result, cancel)
8. Cancellation (cooperative cancellation via `notifications/cancelled`)
9. Server logging (structured log forwarding via `notifications/message`)
10. Structured output (`outputSchema` + `structuredContent` validation)
11. Tool metadata (title, behavioral hints/annotations)
12. CLI: `protomcp dev` and `protomcp run`
13. Python library (decorator API, schema generation, tool_manager, progress, async tasks, logging)
14. TypeScript library (same API surface)
15. Documentation site (Starlight): Getting Started, Guides (Python, TS, Dynamic Tool Lists, Hot Reload, Progress, Async Tasks, Logging), Reference (CLI, Protobuf Spec, Python API, TS API), Concepts (Architecture, Tool List Modes, Transports)

### v1.1 — Expand

10. Go library
11. Rust library
12. Middleware/hooks system (interceptor chain in Go binary)
13. Auth support (OAuth 2.1, JWT, API Key) at the transport layer
14. Build-time validation (tool naming, descriptions, argument structure)
15. Documentation: Go/Rust guides and API reference, Middleware guide, Auth guide, "Writing a Language Library" community guide

### v1.2 — Ecosystem

16. OpenAPI spec ingestion (auto-generate tools from API specs)
17. File-system routing (optional alternative to decorators)
18. Additional community language libraries
19. Documentation: OpenAPI guide, File-system routing guide, expanded community section

## Non-Goals

- No scaffolding generators (`protomcp init`, `protomcp create`)
- No dashboards or web UIs
- No Docker dependency
- No class-based inheritance patterns
- No opinion on how tool code is structured beyond the decorator API
- No 1:1 REST-to-MCP mapping encouragement (docs should push "design for outcomes")

## Open Questions

1. **Multi-process support**: Should the Go binary support proxying to multiple tool processes simultaneously (multi-file, multi-language)? e.g., `protomcp dev server.py tools.ts`. The current protobuf protocol implicitly assumes a single tool process. If multi-process is a future possibility, the `.proto` should include a process/namespace identifier from day one to avoid a breaking protocol change later. **Recommendation**: Include an optional `namespace` field in the protobuf messages now, enforce single-process in v1, enable multi-process in a future version.
2. **Config file format**: Should there be a config file for production deployments, or are CLI flags sufficient? **Recommendation**: CLI flags only for v1. Add optional `protomcp.toml` in v1.1 if users request it.
3. **Directory watching**: Should `protomcp dev` support watching a directory of files, or only explicit file paths? **Recommendation**: Support both. `protomcp dev server.py` watches that file. `protomcp dev ./tools/` watches all matching files in the directory. File extension determines which files are relevant.
