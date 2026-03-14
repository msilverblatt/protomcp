# MCP Go SDK Integration Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace protomcp's custom MCP handler and transport layer with the official `modelcontextprotocol/go-sdk`, getting 100% MCP spec coverage while preserving protomcp's multi-SDK process management, hot reload, middleware, and sideband performance features.

**Architecture:** The official `mcp.Server` handles all MCP protocol concerns (initialize, tools, resources, prompts, sampling, completions, logging, ping, pagination, content types, session management). protomcp registers proxy handlers that forward requests to SDK tool processes via protobuf over unix socket. Official transports (`StdioTransport`, `StreamableHTTPHandler`) replace custom implementations.

**Tech Stack:** `github.com/modelcontextprotocol/go-sdk/mcp` (MCP protocol), protobuf (internal Go↔SDK communication), fsnotify (hot reload + file watching)

---

## File Structure

### Files to DELETE
- `internal/mcp/handler.go` — custom MCP handler (replaced by `mcp.Server`)
- `internal/mcp/types.go` — custom MCP types (replaced by `mcp` package types)
- `internal/transport/http.go` — custom HTTP transport (replaced by `mcp.StreamableHTTPHandler`)
- `internal/transport/sse.go` — custom SSE transport (replaced by `mcp.StreamableHTTPHandler`)
- `internal/transport/transport.go` — custom transport interface (no longer needed)

### Files to CREATE
- `internal/bridge/bridge.go` — Core bridge: creates `mcp.Server`, registers proxy handlers, manages lifecycle
- `internal/bridge/tools.go` — Tool proxy: forwards tools/list and tools/call to SDK process
- `internal/bridge/resources.go` — Resource proxy: forwards resources/list, resources/read, etc.
- `internal/bridge/prompts.go` — Prompt proxy: forwards prompts/list, prompts/get
- `internal/bridge/sampling.go` — Sampling proxy: forwards SDK process sampling requests to MCP client
- `internal/bridge/bridge_test.go` — Unit tests for bridge
- `internal/bridge/tools_test.go` — Tool proxy tests
- `internal/bridge/resources_test.go` — Resource proxy tests
- `internal/bridge/prompts_test.go` — Prompt proxy tests
- `internal/bridge/sampling_test.go` — Sampling proxy tests

### Files to MODIFY
- `cmd/protomcp/main.go` — Rewire: use bridge + official transports instead of custom handler + transport
- `internal/process/manager.go` — Extend: add resource/prompt/completion/sampling message handling
- `internal/middleware/chain.go` — Update: middleware wraps `mcp.Server` method handlers instead of custom handler
- `internal/middleware/logging.go` — Update: adapt to new handler signature
- `internal/middleware/errors.go` — Update: adapt to new handler signature
- `internal/middleware/auth.go` — Update: adapt to HTTP middleware for Streamable HTTP
- `internal/middleware/custom.go` — Update: adapt to new handler signature
- `internal/transport/stdio.go` — Simplify: thin wrapper around `mcp.StdioTransport` or delete
- `internal/transport/ws.go` — Keep as-is (custom transport)
- `internal/transport/grpc.go` — Keep as-is (custom transport)
- `proto/protomcp.proto` — Extend: add resource, prompt, completion, sampling messages
- `sdk/python/src/protomcp/runner.py` — Extend: handle resource/prompt/completion messages
- `sdk/python/src/protomcp/` — Add: resource, prompt, completion, sampling registration APIs
- `sdk/typescript/src/runner.ts` — Extend: same
- `sdk/typescript/src/` — Add: same
- `sdk/go/protomcp/runner.go` — Extend: same
- `sdk/go/protomcp/` — Add: same
- `sdk/rust/src/runner.rs` — Extend: same
- `sdk/rust/src/` — Add: same
- `tests/` — Update: all integration/benchmark tests to work with new architecture
- `go.mod` — Add: `github.com/modelcontextprotocol/go-sdk`

### Files UNCHANGED
- `internal/process/manager.go` (core process lifecycle — only extended, not rewritten)
- `internal/envelope/envelope.go` — protobuf I/O stays the same
- `internal/reload/watcher.go` — hot reload stays the same
- All SDK transport/tool/middleware/logging code that talks protobuf

---

## Chunk 1: Foundation — Add dependency, create bridge, proxy tools

This chunk gets the basic architecture working: official `mcp.Server` handling tool list/call by proxying to the SDK process. Everything else (resources, prompts, etc.) comes in later chunks.

### Task 1: Add go-sdk dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

```bash
cd /Users/msilverblatt/hotmcp && go get github.com/modelcontextprotocol/go-sdk@latest
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy && go build ./...
```
Expected: builds successfully (no code changes yet, just dependency added)

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add official MCP Go SDK"
```

---

### Task 2: Create bridge package with tool proxy

**Files:**
- Create: `internal/bridge/bridge.go`
- Create: `internal/bridge/tools.go`
- Test: `internal/bridge/bridge_test.go`

- [ ] **Step 1: Write the bridge core**

Create `internal/bridge/bridge.go`:

```go
package bridge

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// ProcessBackend is the interface for communicating with an SDK tool process.
// Implemented by process.Manager.
type ProcessBackend interface {
	ActiveTools() []*pb.ToolDefinition
	CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error)
}

// Bridge connects an mcp.Server to a ProcessBackend.
// It registers proxy handlers that forward MCP requests to the SDK process.
type Bridge struct {
	Server  *mcp.Server
	backend ProcessBackend
	logger  *slog.Logger
}

// New creates a Bridge with an mcp.Server that proxies to the given backend.
func New(backend ProcessBackend, logger *slog.Logger) *Bridge {
	opts := &mcp.ServerOptions{
		Logger: logger,
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "protomcp", Version: "1.0.0"},
		opts,
	)

	b := &Bridge{
		Server:  server,
		backend: backend,
		logger:  logger,
	}

	return b
}

// SyncTools reads tool definitions from the backend and registers them
// with the mcp.Server. Called on startup and after hot reload.
func (b *Bridge) SyncTools() {
	syncTools(b.Server, b.backend)
}
```

- [ ] **Step 2: Write the tool proxy**

Create `internal/bridge/tools.go`:

```go
package bridge

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// syncTools clears existing tools and re-registers them from the backend.
func syncTools(server *mcp.Server, backend ProcessBackend) {
	// Remove all existing tools
	// (The official SDK doesn't have a RemoveAll, so we track and remove by name)
	tools := backend.ActiveTools()

	for _, t := range tools {
		tool := convertToolDef(t)
		handler := makeToolHandler(backend, t.Name)
		server.AddTool(tool, handler)
	}
}

func convertToolDef(t *pb.ToolDefinition) *mcp.Tool {
	tool := &mcp.Tool{
		Name:        t.Name,
		Description: t.Description,
	}

	// Parse input schema (field is `InputSchema any`, accepts json.RawMessage)
	if t.InputSchemaJson != "" {
		tool.InputSchema = json.RawMessage(t.InputSchemaJson)
	} else {
		// SDK panics if InputSchema is nil or not type "object"
		tool.InputSchema = json.RawMessage(`{"type":"object"}`)
	}

	// Parse output schema
	if t.OutputSchemaJson != "" {
		tool.OutputSchema = json.RawMessage(t.OutputSchemaJson)
	}

	// Set title (top-level field on Tool, not on Annotations)
	if t.Title != "" {
		tool.Title = t.Title
	}

	// Set annotations
	if t.ReadOnlyHint || t.DestructiveHint || t.IdempotentHint || t.OpenWorldHint {
		tool.Annotations = &mcp.ToolAnnotations{}
		if t.ReadOnlyHint {
			tool.Annotations.ReadOnlyHint = true
		}
		if t.DestructiveHint {
			v := true
			tool.Annotations.DestructiveHint = &v
		}
		if t.IdempotentHint {
			tool.Annotations.IdempotentHint = true
		}
		if t.OpenWorldHint {
			v := true
			tool.Annotations.OpenWorldHint = &v
		}
	}

	return tool
}

func makeToolHandler(backend ProcessBackend, name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// req.Params.Arguments is json.RawMessage — use directly, don't re-marshal
		argsJSON := "{}"
		if len(req.Params.Arguments) > 0 {
			argsJSON = string(req.Params.Arguments)
		}

		resp, err := backend.CallTool(ctx, name, argsJSON)
		if err != nil {
			return nil, err
		}

		result := &mcp.CallToolResult{
			IsError: resp.IsError,
		}

		// Parse result_json into content items
		if resp.ResultJson != "" {
			var items []json.RawMessage
			if err := json.Unmarshal([]byte(resp.ResultJson), &items); err == nil {
				for _, item := range items {
					var typeCheck struct {
						Type string `json:"type"`
					}
					if json.Unmarshal(item, &typeCheck) == nil {
						switch typeCheck.Type {
						case "text":
							var tc mcp.TextContent
							if json.Unmarshal(item, &tc) == nil {
								result.Content = append(result.Content, &tc)
							}
						case "image":
							var ic mcp.ImageContent
							if json.Unmarshal(item, &ic) == nil {
								result.Content = append(result.Content, &ic)
							}
						case "audio":
							var ac mcp.AudioContent
							if json.Unmarshal(item, &ac) == nil {
								result.Content = append(result.Content, &ac)
							}
						case "resource":
							var er mcp.EmbeddedResource
							if json.Unmarshal(item, &er) == nil {
								result.Content = append(result.Content, &er)
							}
						default:
							// Unknown content type, wrap as text
							result.Content = append(result.Content, &mcp.TextContent{Text: string(item)})
						}
					}
				}
			}
		}

		return result, nil
	}
}
```

- [ ] **Step 3: Write unit test**

Create `internal/bridge/bridge_test.go`:

```go
package bridge

import (
	"context"
	"encoding/json"
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// mockBackend implements ProcessBackend for testing.
type mockBackend struct {
	tools    []*pb.ToolDefinition
	callResp *pb.CallToolResponse
	callErr  error
}

func (m *mockBackend) ActiveTools() []*pb.ToolDefinition {
	return m.tools
}

func (m *mockBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return m.callResp, m.callErr
}

func TestBridgeNew(t *testing.T) {
	backend := &mockBackend{
		tools: []*pb.ToolDefinition{
			{Name: "echo", Description: "echoes input", InputSchemaJson: `{"type":"object"}`},
		},
	}
	b := New(backend, nil)
	if b.Server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestSyncTools(t *testing.T) {
	backend := &mockBackend{
		tools: []*pb.ToolDefinition{
			{Name: "echo", Description: "echoes input", InputSchemaJson: `{"type":"object","properties":{"message":{"type":"string"}}}`},
			{Name: "add", Description: "adds numbers", InputSchemaJson: `{"type":"object"}`},
		},
	}
	b := New(backend, nil)
	b.SyncTools()
	// Server should now have 2 tools registered
	// (We verify via integration test since Server doesn't expose tool count directly)
}

func TestConvertToolDef(t *testing.T) {
	def := &pb.ToolDefinition{
		Name:            "test",
		Description:     "a test tool",
		InputSchemaJson: `{"type":"object"}`,
		ReadOnlyHint:    true,
		DestructiveHint: true,
		Title:           "Test Tool",
	}
	tool := convertToolDef(def)
	if tool.Name != "test" {
		t.Errorf("expected name 'test', got %q", tool.Name)
	}
	if tool.Title != "Test Tool" {
		t.Errorf("expected title 'Test Tool', got %q", tool.Title)
	}
	if tool.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint")
	}
}

func TestMakeToolHandler(t *testing.T) {
	resultJSON := `[{"type":"text","text":"hello"}]`
	backend := &mockBackend{
		callResp: &pb.CallToolResponse{
			ResultJson: resultJSON,
			IsError:    false,
		},
	}

	handler := makeToolHandler(backend, "echo")
	result, err := handler(context.Background(), &mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "hello" {
		t.Errorf("expected 'hello', got %q", tc.Text)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
cd /Users/msilverblatt/hotmcp && go test -v ./internal/bridge/
```
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/bridge/
git commit -m "feat: add bridge package — proxy tool calls through official MCP Go SDK"
```

---

### Task 3: Rewire main.go to use bridge + official transports

**Files:**
- Modify: `cmd/protomcp/main.go`

This is the critical integration task. The main.go currently:
1. Creates process manager
2. Creates custom `mcp.Handler`
3. Creates custom transport
4. Wires callbacks
5. Starts transport

New flow:
1. Creates process manager (same)
2. Creates `bridge.New(backend)` which creates `mcp.Server`
3. Calls `bridge.SyncTools()` to register tools
4. Uses `mcp.StdioTransport` or `mcp.NewStreamableHTTPHandler()` directly
5. Calls `server.Run(ctx, transport)`

- [ ] **Step 1: Read current main.go fully**

Read `/Users/msilverblatt/hotmcp/cmd/protomcp/main.go` to understand the complete wiring.

- [ ] **Step 2: Rewrite main.go**

Replace the handler/transport wiring section. Keep:
- Config parsing
- Process manager creation and startup
- Tool list manager
- Hot reload watcher
- Middleware (adapt to work with mcp.Server)

Replace:
- `mcp.NewHandler(backend)` → `bridge.New(backend, logger)`
- `createTransport(cfg)` → official SDK transports
- `tp.Start(ctx, chain)` → `server.Run(ctx, transport)`
- Custom notification callbacks → use `mcp.Server` built-in notification support

Wire process manager callbacks:
- `pm.OnProgress()` → use `ServerSession.SendNotificationToClient()` or similar
- `pm.OnLog()` → use `ServerSession.Log()`
- `pm.OnEnableTools()`/`pm.OnDisableTools()` → call `bridge.SyncTools()` then `server.RemoveTool()`/`server.AddTool()`

- [ ] **Step 3: Build and fix compilation errors**

```bash
go build ./cmd/protomcp/
```

Iterate until it compiles. The official SDK types won't match 1:1 with the old custom types — adapt as needed.

- [ ] **Step 4: Run existing integration tests**

```bash
go test -v -timeout 60s ./tests/...
```

Fix any failures. The core tool list/call flow should work since we're proxying through the same `ProcessBackend`.

- [ ] **Step 5: Commit**

```bash
git add cmd/protomcp/ internal/bridge/
git commit -m "feat: rewire main.go to use official MCP Go SDK via bridge"
```

---

### Task 4: Update middleware to work with new architecture

**Files:**
- Modify: `internal/middleware/chain.go`
- Modify: `internal/middleware/logging.go`
- Modify: `internal/middleware/errors.go`
- Modify: `internal/middleware/auth.go`
- Modify: `internal/middleware/custom.go`

The middleware currently wraps `func(ctx, JSONRPCRequest) (*JSONRPCResponse, error)`. With the official SDK, middleware needs to work differently:

- **For Streamable HTTP**: Use standard `http.Handler` middleware (wraps the `StreamableHTTPHandler`)
- **For stdio**: Use `mcp.ServerOptions` handlers or `MethodHandler` middleware
- **Auth middleware**: Becomes HTTP middleware on the Streamable HTTP handler
- **Logging middleware**: Can use `ServerOptions.Logger`
- **Custom middleware (tool process)**: Still intercepts before/after tool calls via `ProcessBackend`

The official SDK has its own middleware concept via `MethodHandler` and `Middleware`:

```go
type MethodHandler func(ctx context.Context, method string, req Request) (result Result, err error)
type Middleware func(MethodHandler) MethodHandler
```

- [ ] **Step 1: Adapt middleware to official SDK patterns**

Update `chain.go` to use the official SDK's `MethodHandler` type for server-level middleware, and `http.Handler` wrapping for HTTP-level middleware (auth, logging).

- [ ] **Step 2: Adapt auth middleware for HTTP**

Convert auth from JSON-RPC middleware to HTTP middleware that wraps `StreamableHTTPHandler`.

- [ ] **Step 3: Adapt custom middleware**

Custom middleware (tool process interceptors) should hook into the bridge's tool handler — intercept before/after `CallTool`. This stays at the bridge level, not the MCP level.

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/middleware/
```

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/
git commit -m "refactor: adapt middleware to official MCP Go SDK architecture"
```

---

### Task 5: Delete old handler and transport code

**Files:**
- Delete: `internal/mcp/handler.go`
- Delete: `internal/mcp/types.go`
- Delete: `internal/transport/http.go`
- Delete: `internal/transport/sse.go`
- Delete: `internal/transport/transport.go`
- Modify: `internal/transport/stdio.go` (simplify or delete)

- [ ] **Step 1: Delete the files**

```bash
rm internal/mcp/handler.go internal/mcp/types.go
rm internal/transport/http.go internal/transport/sse.go internal/transport/transport.go
```

- [ ] **Step 2: Fix all compilation errors**

```bash
go build ./...
```

Update any remaining imports of `internal/mcp` to use the official `mcp` package or the bridge package. This will touch test files, benchmark files, etc.

- [ ] **Step 3: Run full test suite**

```bash
go test -v -timeout 300s ./...
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "refactor: remove custom MCP handler and transport — replaced by official SDK"
```

---

### Task 6: Verify tools still work end-to-end

**Files:**
- Test: `tests/` (existing integration tests)

- [ ] **Step 1: Run the echo tool manually**

```bash
cd /Users/msilverblatt/hotmcp && go build -o bin/pmcp ./cmd/protomcp/
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hello"}}}' | bin/pmcp dev tests/bench/fixtures/sdk_echo_tool.py
```

Expected: valid JSON-RPC responses with protocol version `2025-03-26` (or later), tool list, and echo result.

- [ ] **Step 2: Run benchmark tests**

```bash
go test -v -timeout 300s -run TestDetailedComparison ./tests/bench/comparison/
```

Expected: protomcp still beats FastMCP (performance should be same or better since we removed a layer).

- [ ] **Step 3: Test hot reload**

Start the dev server, modify the tool file, verify tools are re-synced.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "test: verify end-to-end tool flow with official MCP SDK"
```

---

### Note: Ping, Logging, Pagination, Rich Content

These MCP spec features are handled automatically by the official `mcp.Server` and require no custom code:

- **Ping**: Built into `mcp.Server` — responds to `ping` requests automatically.
- **Logging/setLevel**: Use `ServerSession.Log()` to send log messages. The server tracks per-session log level via `notifications/setLevel`. Wire `OnLog` callback from process manager to `ServerSession.Log()` in the bridge.
- **Pagination**: `mcp.Server` supports `cursor`-based pagination on `tools/list`, `resources/list`, `prompts/list` natively. No custom code needed.
- **Rich Content Types**: All content types (TextContent, ImageContent, AudioContent, EmbeddedResource, ResourceLink) are defined in the `mcp` package. The bridge's content parsing in Task 2 already handles these.

These will be verified in the e2e compliance test (Task 19).

---

## Chunk 2: Resources, Prompts, Completions

Now that the bridge is working for tools, extend it for resources, prompts, and completions. This requires proto changes AND SDK changes.

### Task 7: Extend proto for resources and prompts

**Files:**
- Modify: `proto/protomcp.proto`

- [ ] **Step 1: Add resource messages to proto**

Add to `proto/protomcp.proto` inside the `Envelope.msg` oneof:

```protobuf
// Resources
ListResourcesRequest list_resources_request = 30;
ResourceListResponse resource_list_response = 31;
ListResourceTemplatesRequest list_resource_templates_request = 32;
ResourceTemplateListResponse resource_template_list_response = 33;
ReadResourceRequest read_resource_request = 34;
ReadResourceResponse read_resource_response = 35;
ResourceChangedNotification resource_changed = 36;

// Prompts
ListPromptsRequest list_prompts_request = 40;
PromptListResponse prompt_list_response = 41;
GetPromptRequest get_prompt_request = 42;
GetPromptResponse get_prompt_response = 43;

// Completions
CompletionRequest completion_request = 50;
CompletionResponse completion_response = 51;
```

And the message definitions:

```protobuf
message ResourceDefinition {
  string uri = 1;
  string name = 2;
  string description = 3;
  string mime_type = 4;
  int64 size = 5;
}

message ResourceTemplateDefinition {
  string uri_template = 1;
  string name = 2;
  string description = 3;
  string mime_type = 4;
}

message ListResourcesRequest {}
message ResourceListResponse {
  repeated ResourceDefinition resources = 1;
}

message ListResourceTemplatesRequest {}
message ResourceTemplateListResponse {
  repeated ResourceTemplateDefinition templates = 1;
}

message ReadResourceRequest {
  string uri = 1;
}

message ResourceContent {
  string uri = 1;
  string mime_type = 2;
  string text = 3;
  bytes blob = 4;
}

message ReadResourceResponse {
  repeated ResourceContent contents = 1;
}

message ResourceChangedNotification {
  string uri = 1;
}

message PromptArgument {
  string name = 1;
  string description = 2;
  bool required = 3;
}

message PromptDefinition {
  string name = 1;
  string description = 2;
  repeated PromptArgument arguments = 3;
}

message ListPromptsRequest {}
message PromptListResponse {
  repeated PromptDefinition prompts = 1;
}

message GetPromptRequest {
  string name = 1;
  string arguments_json = 2;
}

message PromptMessage {
  string role = 1;
  string content_json = 2;
}

message GetPromptResponse {
  string description = 1;
  repeated PromptMessage messages = 2;
}

message CompletionRequest {
  string ref_type = 1;
  string ref_name = 2;
  string argument_name = 3;
  string argument_value = 4;
}

message CompletionResponse {
  repeated string values = 1;
  int32 total = 2;
  bool has_more = 3;
}
```

- [ ] **Step 2: Regenerate proto**

```bash
PATH="$HOME/go/bin:$PATH" make proto
```

- [ ] **Step 3: Sync Rust proto**

```bash
cp proto/protomcp.proto sdk/rust/proto/protomcp.proto
```

- [ ] **Step 4: Build to verify**

```bash
go build ./...
```

- [ ] **Step 5: Commit**

```bash
git add proto/ gen/ sdk/rust/proto/
git commit -m "proto: add resource, prompt, and completion messages"
```

---

### Task 8: Extend process manager for resources/prompts/completions

**Files:**
- Modify: `internal/process/manager.go`

- [ ] **Step 1: Add resource/prompt/completion methods to Manager**

Add methods mirroring the existing `CallTool` pattern:

```go
func (m *Manager) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error)
func (m *Manager) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error)
func (m *Manager) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error)
func (m *Manager) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error)
func (m *Manager) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error)
func (m *Manager) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error)
```

Each sends the corresponding protobuf request and waits for the response, same pattern as `CallTool`.

- [ ] **Step 2: Handle new message types in the read loop**

In the Manager's background read loop, add cases for the new response message types so they route to the correct waiting goroutine.

- [ ] **Step 3: Update the handshake**

During handshake, SDK process should also send resource and prompt definitions alongside tools. Update the handshake flow to receive `ResourceListResponse` and `PromptListResponse` after `ToolListResponse`.

- [ ] **Step 4: Test**

```bash
go test -v ./internal/process/
```

- [ ] **Step 5: Commit**

```bash
git add internal/process/
git commit -m "feat: extend process manager for resources, prompts, completions"
```

---

### Task 9: Add resource and prompt proxies to bridge

**Files:**
- Create: `internal/bridge/resources.go`
- Create: `internal/bridge/prompts.go`
- Modify: `internal/bridge/bridge.go`

- [ ] **Step 1: Write resource proxy**

Create `internal/bridge/resources.go`:

```go
package bridge

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ResourceBackend is the interface for resource operations.
type ResourceBackend interface {
	ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error)
	ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error)
	ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error)
}

func syncResources(server *mcp.Server, backend ResourceBackend) {
	ctx := context.Background()
	resources, err := backend.ListResources(ctx)
	if err != nil {
		return
	}
	for _, r := range resources {
		res := &mcp.Resource{
			URI:         r.Uri,
			Name:        r.Name,
			Description: r.Description,
			MIMEType:    r.MimeType,
		}
		handler := makeResourceHandler(backend, r.Uri)
		server.AddResource(res, handler)
	}

	templates, err := backend.ListResourceTemplates(ctx)
	if err != nil {
		return
	}
	for _, t := range templates {
		tmpl := &mcp.ResourceTemplate{
			URITemplate: t.UriTemplate,
			Name:        t.Name,
			Description: t.Description,
			MIMEType:    t.MimeType,
		}
		handler := makeResourceHandler(backend, t.UriTemplate)
		server.AddResourceTemplate(tmpl, handler)
	}
}

func makeResourceHandler(backend ResourceBackend, uri string) mcp.ResourceHandler {
	return func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		resp, err := backend.ReadResource(ctx, req.Params.URI)
		if err != nil {
			return nil, err
		}
		result := &mcp.ReadResourceResult{}
		for _, c := range resp.Contents {
			rc := &mcp.ResourceContents{
				URI:      c.Uri,
				MIMEType: c.MimeType,
			}
			if len(c.Blob) > 0 {
				rc.Blob = c.Blob
			} else {
				rc.Text = c.Text
			}
			result.Contents = append(result.Contents, rc)
		}
		return result, nil
	}
}
```

- [ ] **Step 2: Write prompt proxy**

Create `internal/bridge/prompts.go` following the same pattern.

- [ ] **Step 3: Update bridge.go to call sync functions**

Add `SyncResources()` and `SyncPrompts()` methods, called from `bridge.New()` or explicitly after process start.

- [ ] **Step 4: Add completion handler**

Wire `ServerOptions.CompletionHandler` to proxy through the process manager.

- [ ] **Step 5: Test**

```bash
go test -v ./internal/bridge/
```

- [ ] **Step 6: Commit**

```bash
git add internal/bridge/
git commit -m "feat: add resource, prompt, and completion proxies to bridge"
```

---

### Task 10: Extend Python SDK with resource/prompt/completion support

**Files:**
- Modify: `sdk/python/src/protomcp/runner.py`
- Create: `sdk/python/src/protomcp/resource.py`
- Create: `sdk/python/src/protomcp/prompt.py`
- Create: `sdk/python/src/protomcp/completion.py`

- [ ] **Step 1: Add resource registration API**

Create `resource.py` with `@resource` and `@resource_template` decorators, a resource registry (same pattern as tool registry), and `notify_resource_changed()`.

- [ ] **Step 2: Add prompt registration API**

Create `prompt.py` with `@prompt` decorator, prompt registry, and `Message`/`Arg` types.

- [ ] **Step 3: Add completion support**

Create `completion.py` — completions are attached to `Arg` objects on prompts/resources. The completion handler in the runner looks up the right arg and calls its provider.

- [ ] **Step 4: Update runner to handle new message types**

In `runner.py`, add cases for `ListResourcesRequest`, `ReadResourceRequest`, `ListPromptsRequest`, `GetPromptRequest`, `CompletionRequest`.

- [ ] **Step 5: Test with a fixture**

Create a test fixture that registers a resource and prompt, verify they appear in `resources/list` and `prompts/list`.

- [ ] **Step 6: Commit**

```bash
git add sdk/python/
git commit -m "feat(python): add resource, prompt, and completion support"
```

---

### Task 11: Extend TypeScript SDK (same pattern as Python)

**Files:**
- Modify: `sdk/typescript/src/runner.ts`
- Create: `sdk/typescript/src/resource.ts`
- Create: `sdk/typescript/src/prompt.ts`

Follow exact same pattern as Task 10 for TypeScript.

- [ ] **Commit**

```bash
git add sdk/typescript/
git commit -m "feat(typescript): add resource, prompt, and completion support"
```

---

### Task 12: Extend Go SDK (same pattern)

Follow same pattern for Go SDK.

- [ ] **Commit**

```bash
git add sdk/go/
git commit -m "feat(go): add resource, prompt, and completion support"
```

---

### Task 13: Extend Rust SDK (same pattern)

Follow same pattern for Rust SDK.

- [ ] **Commit**

```bash
git add sdk/rust/
git commit -m "feat(rust): add resource, prompt, and completion support"
```

---

## Chunk 3: Sampling, Roots, and Bidirectional Communication

### Task 14: Add sampling support (SDK process → Go → MCP client)

**Files:**
- Modify: `proto/protomcp.proto`
- Modify: `internal/process/manager.go`
- Create: `internal/bridge/sampling.go`
- Modify: `internal/bridge/bridge.go`

- [ ] **Step 1: Add sampling proto messages**

```protobuf
// In Envelope.msg oneof:
SamplingRequest sampling_request = 60;
SamplingResponse sampling_response = 61;
```

```protobuf
message SamplingRequest {
  string request_id = 1;
  string messages_json = 2;
  string model_preferences_json = 3;
  string system_prompt = 4;
  int32 max_tokens = 5;
  string session_id = 6;
}

message SamplingResponse {
  string request_id = 1;
  string role = 2;
  string content_json = 3;
  string model = 4;
  string stop_reason = 5;
  string error = 6;
}
```

- [ ] **Step 2: Handle sampling requests in process manager read loop**

When the SDK process sends a `SamplingRequest`, the manager should emit it via a callback (similar to `OnProgress`).

- [ ] **Step 3: Wire sampling in bridge**

In `internal/bridge/sampling.go`, when the manager emits a sampling request:
1. Use the `ServerSession` to call `CreateMessage()` on the MCP client
2. Wait for the response
3. Send `SamplingResponse` back to the SDK process via the manager

- [ ] **Step 4: Add `ctx.sample()` to Python SDK**

Add a `sample()` method on `ToolContext` that sends `SamplingRequest` proto and waits for `SamplingResponse`.

- [ ] **Step 5: Add to other SDKs**

Same pattern for TS, Go, Rust.

- [ ] **Step 6: Test with mock client**

- [ ] **Step 7: Commit**

```bash
git add proto/ gen/ internal/ sdk/
git commit -m "feat: add sampling support — SDK processes can request LLM calls"
```

---

### Task 15: Add roots support

**Files:**
- Modify: `internal/bridge/bridge.go`

- [ ] **Step 1: Wire roots**

Two pieces:
1. Use `mcp.ServerOptions.RootsListChangedHandler` to handle `notifications/roots/list_changed` from the client. When received, call `ServerSession.ListRoots()` to fetch the updated root list, then forward to SDK processes via proto message.
2. Add `ctx.get_roots()` to SDKs — sends a proto request to the process manager, which calls `ServerSession.ListRoots()` on the active session and returns the result.

- [ ] **Step 2: Test**

- [ ] **Step 3: Commit**

```bash
git add internal/ sdk/ proto/ gen/
git commit -m "feat: add roots support — SDK processes can access client filesystem roots"
```

---

## Chunk 4: Documentation, Examples, and Final Verification

### Task 16: Create per-feature examples

**Files:**
- Create: `examples/resources/` (one per SDK)
- Create: `examples/prompts/` (one per SDK)
- Create: `examples/sampling/` (one per SDK)
- Create: `examples/kitchen-sink/` (one per SDK)

- [ ] **Step 1: Python resource example**
- [ ] **Step 2: Python prompt example**
- [ ] **Step 3: Python sampling example**
- [ ] **Step 4: Python kitchen sink (all features)**
- [ ] **Step 5: Repeat for TS, Go, Rust**
- [ ] **Step 6: Commit**

```bash
git add examples/
git commit -m "docs: add per-feature examples for all 4 SDKs"
```

---

### Task 17: Write feature documentation

**Files:**
- Create: `docs/resources.md`
- Create: `docs/prompts.md`
- Create: `docs/completions.md`
- Create: `docs/sampling.md`
- Create: `docs/roots.md`
- Create: `docs/transports.md`
- Create: `docs/authentication.md`
- Modify: `docs/feature-roadmap.md` — update to show 100% coverage

- [ ] **Step 1: Write each doc**

Each doc should cover: what the feature is, how to use it in each SDK, configuration options, example code.

- [ ] **Step 2: Update feature roadmap**

Mark everything as implemented. Update SDK feature matrix.

- [ ] **Step 3: Commit**

```bash
git add docs/
git commit -m "docs: add feature guides for resources, prompts, sampling, completions, roots"
```

---

### Task 18: Create MCP compliance doc

**Files:**
- Create: `docs/mcp-compliance.md`

- [ ] **Step 1: Write compliance mapping**

Map every MCP spec method/notification to its protomcp implementation. This is the "100% coverage" proof.

- [ ] **Step 2: Commit**

```bash
git add docs/mcp-compliance.md
git commit -m "docs: add MCP spec compliance mapping"
```

---

### Task 19: Full integration test suite

**Files:**
- Create: `tests/e2e/full_mcp_test.go`

- [ ] **Step 1: Write comprehensive e2e test**

Test that exercises every MCP method in sequence:
1. Initialize (verify protocol version)
2. Ping
3. tools/list, tools/call
4. resources/list, resources/read, resources/subscribe
5. prompts/list, prompts/get
6. completion/complete
7. sampling/createMessage (with mock client)
8. roots/list
9. logging/setLevel, verify log filtering

- [ ] **Step 2: Run it**

```bash
go test -v -timeout 120s ./tests/e2e/ -run TestFullMCPCompliance
```

- [ ] **Step 3: Run existing benchmarks to verify no performance regression**

```bash
go test -v -timeout 600s -run TestDetailedComparison ./tests/bench/comparison/
```

- [ ] **Step 4: Commit**

```bash
git add tests/
git commit -m "test: add full MCP compliance integration test"
```

---

### Task 20: Update README

**Files:**
- Modify: `README.md` (if exists)

- [ ] **Step 1: Update feature matrix**

Show 100% MCP spec coverage. Highlight that protomcp uses the official Go SDK.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README with full MCP spec coverage"
```
