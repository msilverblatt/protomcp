# Test Engine & Playground Design Spec

## Goal

Add testing and playground functionality to protomcp — a shared test engine that powers both a CLI (`pmcp test`) and a web UI (`pmcp playground`). The test engine acts as an MCP client, connecting through the full protocol stack to exercise tools, resources, and prompts with full protocol tracing.

## Architecture

```
                        ┌──────────────────────┐
                        │   pmcp test (CLI)    │
                        └──────────┬───────────┘
                                   │
                        ┌──────────▼───────────┐
                        │    Test Engine        │
                        │  (internal/testengine)│
                        └──────────┬───────────┘
                                   │
                        ┌──────────▼───────────┐     ┌──────────────────────┐
                        │   pmcp playground    │────►│  React SPA (embed)   │
                        │   (HTTP + WebSocket) │     │  internal/playground │
                        └──────────────────────┘     └──────────────────────┘
```

The test engine is the shared foundation. The CLI and playground are thin interfaces on top of it. Neither contains business logic.

---

## Test Engine (`internal/testengine/`)

### Connection Model

The test engine acts as a full MCP client. It starts a tool process via `process.Manager`, wires it through the bridge to an `mcp.Server`, then connects to that server using the official SDK's `mcp.Client` over an in-memory transport.

```
    mcp.Client (test engine)
           │
    InMemoryTransport pair
           │
    mcp.Server (official SDK)
           │
    bridge.Bridge
           │
    process.Manager
           │
    Your Tool Code
```

The SDK provides `mcp.NewInMemoryTransports()` which returns two connected transports — one for the server, one for the client. No ports, no stdio. The test engine connects the client side and gets a `ClientSession`.

### Protocol Tracing

The SDK provides `mcp.LoggingTransport` which wraps any transport and writes every JSON-RPC message (both directions) to an `io.Writer`. We wrap the in-memory transport with this to capture the full protocol trace.

The trace writer feeds a `TraceLog` that stores entries and broadcasts to live subscribers (the playground's WebSocket).

### Core Types

```go
package testengine

type Engine struct {
    client  *mcp.Client
    session *mcp.ClientSession
    server  *mcp.Server
    pm      *process.Manager
    bridge  *bridge.Bridge
    trace   *TraceLog
    toolsCh chan []*mcp.Tool  // pushed on tool list changes
}

type TraceEntry struct {
    Timestamp time.Time `json:"timestamp"`
    Direction string    `json:"direction"` // "send" or "recv"
    Raw       string    `json:"raw"`       // full JSON-RPC message
    Method    string    `json:"method"`    // parsed method name
}

type TraceLog struct {
    mu      sync.Mutex
    entries []TraceEntry
    subs    []chan TraceEntry
}

type CallResult struct {
    Result        *mcp.CallToolResult `json:"result"`
    Duration      time.Duration       `json:"duration_ms"`
    ToolsEnabled  []string            `json:"tools_enabled,omitempty"`
    ToolsDisabled []string            `json:"tools_disabled,omitempty"`
}

type Option func(*engineConfig)
type engineConfig struct {
    runtime     string
    socketPath  string
    callTimeout time.Duration
}
```

### Engine API

```go
func New(file string, opts ...Option) (*Engine, error)
func (e *Engine) Start(ctx context.Context) error
func (e *Engine) Stop()

// MCP operations — all go through full protocol stack
func (e *Engine) ListTools(ctx context.Context) ([]*mcp.Tool, error)
func (e *Engine) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error)
func (e *Engine) ListResources(ctx context.Context) ([]*mcp.Resource, error)
func (e *Engine) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error)
func (e *Engine) ListPrompts(ctx context.Context) ([]*mcp.Prompt, error)
func (e *Engine) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error)

// Tracing
func (e *Engine) Trace() *TraceLog
func (e *Engine) SubscribeTrace() <-chan TraceEntry
func (e *Engine) UnsubscribeTrace(ch <-chan TraceEntry)

// Tool list change observation
func (e *Engine) OnToolsChanged(fn func(tools []*mcp.Tool))

// Hot reload
func (e *Engine) Reload(ctx context.Context) error
```

### TraceLog API

```go
func (t *TraceLog) Entries() []TraceEntry         // snapshot
func (t *TraceLog) Subscribe() chan TraceEntry     // live stream
func (t *TraceLog) Unsubscribe(ch chan TraceEntry)
func (t *TraceLog) Clear()
```

### Trace Writer

A custom `io.Writer` that parses the SDK's log lines into `TraceEntry` structs. The `LoggingTransport` writes lines like:

```
read: {"jsonrpc":"2.0","id":1,"method":"initialize",...}
write: {"jsonrpc":"2.0","id":1,"result":{...}}
```

The trace writer parses these, extracts direction and method, timestamps them, appends to the log, and broadcasts to subscribers.

### Tool List Change Tracking

The engine wires `process.Manager.OnEnableTools` and `OnDisableTools` callbacks. When tool list changes occur during a `CallTool`, the engine captures them in the `CallResult`. The engine also maintains a `toolsCh` channel that the playground can listen on.

### Hot Reload

The engine wraps `reload.NewWatcher` for dev mode. On file change, it calls `pm.Reload()`, re-syncs the bridge, and pushes the updated tool list to subscribers.

---

## CLI: `pmcp test`

### Command Structure

Three subcommands added to the existing config parser:

```
pmcp test <file> list                              — list tools, resources, prompts
pmcp test <file> call <name> --args '{...}'        — call a tool
pmcp test <file> scenario <scenario.json>          — run scenario (V2)
```

The `test` command is added alongside `dev`, `run`, and `validate` in `config.Parse()`.

### `pmcp test <file> list`

Creates an engine, starts the tool process, discovers all features:

```
$ pmcp test tools.py list

  Tools
  ────────────────────────────────────────────────────────────────
  add               Add two numbers              a (integer, required)
                                                 b (integer, required)
  get_weather       Get weather for a location   city (string, required)
                                                 units (string, optional)

  Resources
  ────────────────────────────────────────────────────────────────
  config://app      App configuration            application/json
  notes://{id}      Read a note (template)       text/plain

  Prompts
  ────────────────────────────────────────────────────────────────
  summarize         Summarize a document         topic (required), style

  3 tools, 2 resources, 1 prompt
```

With `--format json`, outputs machine-readable JSON.

### `pmcp test <file> call <name> --args '{...}'`

Calls a single tool and shows result + protocol trace:

```
$ pmcp test tools.py call get_weather --args '{"city": "SF"}'

  Result (247ms)
  ────────────────────────────────────────────────────────────────
  {"location": "SF", "temperature_f": 62.1, "conditions": "Foggy"}

  Protocol Trace
  ────────────────────────────────────────────────────────────────
  0ms    → initialize {protocolVersion: "2025-03-26", ...}
  1ms    ← initialize result {capabilities: {tools: {listChanged: true}}}
  1ms    → notifications/initialized
  2ms    → tools/call {name: "get_weather", arguments: {"city": "SF"}}
  247ms  ← tools/call result {content: [{type: "text", text: "..."}]}

  Tool list: no changes
```

If tool list changes occurred:
```
  Tool list changes
  ────────────────────────────────────────────────────────────────
  + create_record    (enabled)
  - login            (disabled)
```

`--trace=false` suppresses the protocol trace. `--format json` outputs everything as JSON.

### `pmcp test <file> scenario <file.json>` (V2)

Prints "scenario runner coming in a future release" for now. The config parser accepts the command so the CLI surface is stable.

### Config Changes

```go
// In config.go, add to Config:
TestSubcommand string   // "list", "call", "scenario"
TestToolName   string   // for "call" subcommand
TestArgs       string   // --args JSON string
TestScenario   string   // scenario file path
```

---

## Playground Backend (`internal/playground/`)

### Server

Started with `pmcp playground <file> [--port 3000]`. Creates a test engine, then serves:

```
GET  /                    → React SPA (embed.FS)
GET  /api/tools           → list tools with full schemas
GET  /api/resources       → list resources
GET  /api/prompts         → list prompts with argument schemas
POST /api/call            → call tool, return result + timing + tool list changes
POST /api/resource/read   → read a resource by URI
POST /api/prompt/get      → get prompt with arguments
POST /api/reload          → trigger hot reload
GET  /api/trace           → get full trace log
GET  /ws                  → WebSocket event stream
```

### REST Endpoints

All return JSON. Content-Type: application/json.

`GET /api/tools` returns:
```json
{
  "tools": [
    {
      "name": "get_weather",
      "description": "Get weather for a location",
      "inputSchema": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}
    }
  ]
}
```

`POST /api/call` with `{"name": "get_weather", "args": {"city": "SF"}}` returns:
```json
{
  "result": {"content": [{"type": "text", "text": "..."}], "isError": false},
  "duration_ms": 247,
  "tools_enabled": [],
  "tools_disabled": []
}
```

### WebSocket Event Stream

`GET /ws` upgrades to WebSocket. Events are JSON with a `type` field:

```json
{"type": "trace", "entry": {"timestamp": "...", "direction": "send", "method": "tools/call", "raw": "..."}}
{"type": "tools_changed", "tools": [...]}
{"type": "progress", "token": "...", "progress": 5, "total": 10, "message": "Step 5/10"}
{"type": "log", "level": "info", "message": "..."}
{"type": "reload", "tool_count": 3}
{"type": "connection", "status": "connected"}
```

Backend subscribes to `engine.SubscribeTrace()` and process manager callbacks, pushes through WebSocket.

### Hot Reload

File watcher runs in dev mode. On change: engine reloads, pushes `reload` event through WebSocket, frontend refreshes tool list.

### Package Structure

```
internal/playground/
  server.go       — HTTP server setup, embed.FS, mux
  handlers.go     — REST endpoint handlers
  ws.go           — WebSocket hub, connection management, event broadcast
```

### WebSocket Library

Use `golang.org/x/net/websocket` (already a transitive dependency) or `nhooyr.io/websocket` (modern, context-aware). Recommend `nhooyr.io/websocket` for cleaner API and proper context cancellation.

---

## Playground Frontend

React SPA with Tailwind, built by Vite, embedded into Go binary via `//go:embed`.

### Layout

Two-panel layout with top bar.

**Top Bar:**
- Connection status (green/red dot)
- File name being tested
- Feature count: "4 tools, 2 resources, 1 prompt" — updates live
- Reload button → `POST /api/reload`
- Uptime timer

**Left Panel — Interaction Space:**

Feature picker at top with three tabs: Tools, Resources, Prompts. Each shows registered items. Selecting one opens an interaction form below.

**Tool form:** Auto-generated from input schema.
- `string` → text input
- `integer`/`number` → number input
- `boolean` → checkbox
- `object`/`array` → JSON textarea with validation
- Required fields marked with asterisk
- "Call" button. Spinner while in-flight. Progress bar if progress notifications arrive.

**Resource form:** URI input (pre-filled for static, editable for templates). "Read" button.

**Prompt form:** Auto-generated from argument list. "Get" button.

**Interaction history** below the form, newest at bottom, chat-style scroll:
- Each entry: request (tool name + args) → result (formatted content)
- Errors in red with error code
- Tool list changes as inline system messages: "login enabled create_record, delete_record"
- New tools animate into the picker with a 2-second highlight badge

**Right Panel — Protocol Flow:**

Scrolling log of every JSON-RPC message, timestamped from session start.

- Outgoing (client → server): blue, `→` prefix
- Incoming (server → client): green, `←` prefix
- Notifications: yellow, `⚡` prefix
- Errors: red

Compact one-line summary per entry. Click to expand full JSON-RPC message.

Auto-scrolls to bottom unless user has scrolled up. Clear button top-right.

### Frontend Structure

```
internal/playground/frontend/
  package.json
  vite.config.ts
  tailwind.config.ts
  index.html
  src/
    App.tsx            — layout, panels, top bar
    components/
      TopBar.tsx       — connection status, counts, reload
      FeaturePicker.tsx — tabbed tool/resource/prompt list
      ToolForm.tsx     — auto-generated form from JSON schema
      ResourceForm.tsx — URI input form
      PromptForm.tsx   — argument form
      ResultView.tsx   — formatted result display
      TracePanel.tsx   — protocol flow log
      TraceEntry.tsx   — single expandable trace entry
      ProgressBar.tsx  — animated progress indicator
      History.tsx      — interaction history container
    hooks/
      useWebSocket.ts  — WS connection, reconnect, event parsing
      useApi.ts        — REST API calls with loading states
    types.ts           — shared TypeScript types matching backend JSON
```

### Build & Embed

```
# In Makefile:
playground-frontend:
    cd internal/playground/frontend && npm run build

# In Go:
//go:embed frontend/dist/*
var playgroundFS embed.FS
```

The React app builds to `frontend/dist/`. The Go binary embeds it. `pmcp playground` serves it at `/`.

---

## V1 Scope

**Ship:**
- Test engine (`internal/testengine/`)
- `pmcp test <file> list`
- `pmcp test <file> call <tool> --args '{...}'`
- `pmcp playground <file>` with interactive tool/resource/prompt calling and live protocol trace
- Hot reload visibility in playground
- Dynamic tool list tracking with visual feedback

**Save for V2:**
- `pmcp test <file> scenario <file.json>` — scenario runner with assertions
- CI integration mode (`--ci` flag, exit codes, JUnit output)
- Multi-agent view in playground
- Prompt completion testing in playground

---

## Build Order

1. **Test engine** (`internal/testengine/`) — shared foundation
2. **CLI** (`pmcp test list`, `pmcp test call`) — validates engine API, ships fast
3. **Playground backend** (`internal/playground/`) — HTTP + WebSocket serving the engine
4. **Playground frontend** (`internal/playground/frontend/`) — React SPA
5. **Polish** — error states, loading states, reconnection, edge cases

---

## Dependencies

New:
- `nhooyr.io/websocket` — WebSocket server for playground
- React, Tailwind, Vite — frontend (embedded, not a Go dependency)

Existing (already in go.mod):
- `github.com/modelcontextprotocol/go-sdk/mcp` — Client, InMemoryTransport, LoggingTransport
- `github.com/fsnotify/fsnotify` — file watching for hot reload

---

## Error Handling

- Engine start failure (bad file, missing runtime): clear error message, non-zero exit
- Tool call failure: show error in result, full trace still captured
- WebSocket disconnect: frontend auto-reconnects with exponential backoff
- Process crash: engine detects via `pm.OnCrash()`, pushes error event, playground shows crash state
- Hot reload failure: log error, keep running with old tools, push error event

---

## Testing Strategy

- `internal/testengine/` unit tests: mock process manager, verify trace capture, tool list tracking
- `internal/playground/` handler tests: HTTP test server, verify REST responses
- E2E: start engine with Python echo fixture, call tool, verify result and trace
- Frontend: manual testing for V1, component tests in V2
