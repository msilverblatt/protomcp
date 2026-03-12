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

## Deliverables by Priority

### v1.0 — Core

1. Go binary with all five transports (stdio, SSE, streamable HTTP, gRPC, WebSocket)
2. Protobuf spec (`.proto` file) defining the internal protocol
3. Hot-reload with file watching and `list_changed` notifications
4. Dynamic tool list management (enable/disable/allow/block/query/batch)
5. Structured error handling with agent-friendly error types
6. CLI: `protomcp dev` and `protomcp run`
7. Python library (decorator API, schema generation, tool_manager)
8. TypeScript library (same API surface)

### v1.1 — Expand

9. Go library
10. Rust library
11. Middleware/hooks system (interceptor chain in Go binary)
12. Auth support (OAuth 2.1, JWT, API Key) at the transport layer
13. Build-time validation (tool naming, descriptions, argument structure)

### v1.2 — Ecosystem

14. OpenAPI spec ingestion (auto-generate tools from API specs)
15. File-system routing (optional alternative to decorators)
16. Additional community language libraries

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
