# Critical Bug Fixes Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all critical bugs found in the deep review — broken hot reload, dead custom middleware, dropped messages, JSON injection, broken auth, and fictional Go version.

**Architecture:** All fixes are in the Go binary (`cmd/protomcp/`, `internal/`) and the Go/Rust SDKs. The core problem is that the process manager's `readLoop` only routes messages by `request_id` or dumps them into a 4-slot `handshakeCh` — progress, logs, tool list management, and custom middleware all rely on unsolicited messages that get silently dropped. The fix adds a proper message dispatcher with typed channels. Auth requires injecting HTTP headers into context in each network transport.

**Tech Stack:** Go 1.23, Rust (prost/tokio), protobuf

---

## File Map

| File | Action | Purpose |
|------|--------|---------|
| `go.mod` | Modify | Fix fictional `go 1.25.6` → `go 1.23` |
| `cmd/protomcp/main.go` | Modify | Wire custom middleware into chain, update `allTools` on reload |
| `internal/process/manager.go` | Modify | Add typed dispatch channels for progress, logs, tool list mgmt; route unsolicited messages properly |
| `internal/transport/sse.go` | Modify | Inject auth headers into context |
| `internal/transport/http.go` | Modify | Inject auth headers into context |
| `internal/transport/ws.go` | Modify | Inject auth headers into context |
| `internal/transport/grpc.go` | Modify | Inject auth headers from gRPC metadata into context |
| `internal/cancel/tracker.go` | Modify | Add `context.Context` cancellation support |
| `internal/mcp/handler.go` | Modify | Use cancellable context in `handleToolsCall` |
| `sdk/go/protomcp/runner.go` | Modify | Fix JSON escaping in result text |
| `sdk/rust/src/runner.rs` | Modify | Fix JSON escaping in result text |
| `internal/process/manager_test.go` | Create | Tests for new message dispatch |
| `internal/transport/auth_inject_test.go` | Create | Tests for auth header injection on all transports |
| `internal/cancel/tracker_test.go` | Modify | Test context cancellation |

---

## Chunk 1: Foundation Fixes (go.mod, JSON escaping, cancellation)

### Task 1: Fix fictional Go version in go.mod

**Files:**
- Modify: `go.mod:3`

- [ ] **Step 1: Fix the go directive**

Change `go 1.25.6` to `go 1.23`:

```
go 1.23
```

- [ ] **Step 2: Verify**

Run: `go vet ./...`
Expected: no errors about Go version

- [ ] **Step 3: Commit**

```bash
git add go.mod
git commit -m "fix: change fictional go 1.25.6 to go 1.23 in go.mod"
```

---

### Task 2: Fix JSON escaping in Go SDK result text

The Go SDK builds JSON via `fmt.Sprintf` with `%s` — if the result text contains `"`, `\`, or newlines, the JSON is invalid.

**Files:**
- Modify: `sdk/go/protomcp/runner.go:187`
- Test: `sdk/go/protomcp/runner_test.go`

- [ ] **Step 1: Write failing test**

Add to `sdk/go/protomcp/runner_test.go`:

```go
func TestResultJSONEscaping(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"quotes", `He said "hello"`},
		{"backslash", `path\to\file`},
		{"newline", "line1\nline2"},
		{"tab", "col1\tcol2"},
		{"unicode", "emoji: 🎉"},
		{"all", "He said \"hi\"\npath\\to\\file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildResultJSON(tc.input)
			var parsed []map[string]interface{}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Fatalf("invalid JSON for input %q: %v\nGot: %s", tc.input, err, result)
			}
			if parsed[0]["text"] != tc.input {
				t.Fatalf("round-trip failed: got %q, want %q", parsed[0]["text"], tc.input)
			}
		})
	}
}
```

- [ ] **Step 2: Extract `buildResultJSON` helper and fix the escaping**

In `sdk/go/protomcp/runner.go`, extract the JSON construction into a helper function that uses `json.Marshal` for proper escaping:

```go
func buildResultJSON(text string) string {
	content := []map[string]string{{"type": "text", "text": text}}
	data, err := json.Marshal(content)
	if err != nil {
		// Fallback: this should never fail for a string, but be safe
		return `[{"type":"text","text":""}]`
	}
	return string(data)
}
```

Then replace line 187:
```go
// OLD:
ResultJson: fmt.Sprintf(`[{"type":"text","text":"%s"}]`, result.ResultText),
// NEW:
ResultJson: buildResultJSON(result.ResultText),
```

Also replace line 135 (the "Tool not found" case):
```go
// OLD:
ResultJson: fmt.Sprintf(`[{"type":"text","text":"Tool not found: %s"}]`, req.Name),
// NEW:
ResultJson: buildResultJSON(fmt.Sprintf("Tool not found: %s", req.Name)),
```

And line 149 (the "Invalid arguments JSON" case):
```go
// OLD:
ResultJson: fmt.Sprintf(`[{"type":"text","text":"Invalid arguments JSON: %s"}]`, err.Error()),
// NEW:
ResultJson: buildResultJSON(fmt.Sprintf("Invalid arguments JSON: %s", err.Error())),
```

- [ ] **Step 3: Run tests**

Run: `cd sdk/go && go test ./... -v -run TestResultJSON`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add sdk/go/protomcp/runner.go sdk/go/protomcp/runner_test.go
git commit -m "fix: use json.Marshal for result text to prevent JSON injection in Go SDK"
```

---

### Task 3: Fix JSON escaping in Rust SDK result text

Same bug as Task 2 but in Rust.

**Files:**
- Modify: `sdk/rust/src/runner.rs:110`
- Test: `sdk/rust/src/runner.rs` (inline test module)

- [ ] **Step 1: Add test**

Add to the `#[cfg(test)] mod tests` block in `sdk/rust/src/runner.rs` (create the module if it doesn't exist — or add a new test file `sdk/rust/src/result_json_test.rs`):

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_result_json_escaping() {
        let cases = vec![
            ("simple", "hello"),
            ("quotes", r#"He said "hello""#),
            ("backslash", r"path\to\file"),
            ("newline", "line1\nline2"),
        ];
        for (name, input) in cases {
            let json_str = build_result_json(input);
            let parsed: serde_json::Value = serde_json::from_str(&json_str)
                .unwrap_or_else(|e| panic!("{}: invalid JSON: {} — got: {}", name, e, json_str));
            assert_eq!(parsed[0]["text"].as_str().unwrap(), input, "case: {}", name);
        }
    }
}
```

- [ ] **Step 2: Extract helper and fix**

In `sdk/rust/src/runner.rs`, add:

```rust
fn build_result_json(text: &str) -> String {
    let content = serde_json::json!([{"type": "text", "text": text}]);
    content.to_string()
}
```

Replace line 110:
```rust
// OLD:
result_json: format!(r#"[{{"type":"text","text":"{}"}}]"#, result.result_text),
// NEW:
result_json: build_result_json(&result.result_text),
```

Also replace the "Tool not found" case (~line 99-100):
```rust
// OLD:
format!("Tool not found: {}", req.name),
// NEW (the ToolResult::error already wraps it, but check the ResultJson construction)
```

Actually, trace the `ToolResult::error` path — the `result_text` field gets formatted the same way on line 110. The fix on line 110 handles all cases since all paths converge there.

- [ ] **Step 3: Run tests**

Run: `cd sdk/rust && cargo test`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add sdk/rust/src/runner.rs
git commit -m "fix: use serde_json for result text to prevent JSON injection in Rust SDK"
```

---

### Task 4: Add context cancellation to cancel.Tracker

Currently `cancel.Tracker` sets a boolean but never actually cancels a `context.Context`. The handler needs to create a cancellable context per tool call and have the tracker cancel it.

**Files:**
- Modify: `internal/cancel/tracker.go`
- Modify: `internal/mcp/handler.go:139-155`
- Test: `internal/cancel/tracker_test.go`

- [ ] **Step 1: Write failing test**

Create or update `internal/cancel/tracker_test.go`:

```go
package cancel

import (
	"context"
	"testing"
	"time"
)

func TestTracker_CancelContext(t *testing.T) {
	tr := NewTracker()
	ctx, reqID := tr.TrackCallWithContext(context.Background(), "req-1")

	// ctx should not be cancelled yet
	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled yet")
	default:
	}

	tr.Cancel("req-1")

	// ctx should now be cancelled
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled after Cancel()")
	}

	// Clean up
	tr.Complete(reqID)
}

func TestTracker_CompleteCleanup(t *testing.T) {
	tr := NewTracker()
	_, reqID := tr.TrackCallWithContext(context.Background(), "req-2")
	tr.Complete(reqID)

	// Cancelling after complete should be a no-op (no panic)
	tr.Cancel("req-2")
}
```

- [ ] **Step 2: Update tracker implementation**

```go
package cancel

import (
	"context"
	"sync"
)

type trackedCall struct {
	cancel context.CancelFunc
}

type Tracker struct {
	mu    sync.RWMutex
	calls map[string]*trackedCall
}

func NewTracker() *Tracker {
	return &Tracker{calls: make(map[string]*trackedCall)}
}

// TrackCallWithContext creates a cancellable child context for the given request ID.
// Returns the child context and the request ID (for convenience).
func (t *Tracker) TrackCallWithContext(parent context.Context, requestID string) (context.Context, string) {
	ctx, cancel := context.WithCancel(parent)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls[requestID] = &trackedCall{cancel: cancel}
	return ctx, requestID
}

// Cancel cancels the context for the given request ID.
func (t *Tracker) Cancel(requestID string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if call, exists := t.calls[requestID]; exists {
		call.cancel()
	}
}

// IsCancelled checks if a request has been cancelled.
func (t *Tracker) IsCancelled(requestID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.calls[requestID]
	return exists
}

// Complete removes tracking for the given request ID and releases resources.
func (t *Tracker) Complete(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if call, exists := t.calls[requestID]; exists {
		call.cancel() // ensure context is released
		delete(t.calls, requestID)
	}
}
```

Note: Remove the old `TrackCall` method and `cancelled` map. Keep backward compat by checking callers — the only caller is `handler.go`.

- [ ] **Step 3: Update handler.go to use cancellable context**

In `internal/mcp/handler.go`, update `handleToolsCall` (lines 139-155):

```go
func (h *Handler) handleToolsCall(ctx context.Context, req JSONRPCRequest) (*JSONRPCResponse, error) {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return h.invalidParams(req.ID, fmt.Sprintf("invalid params: %v", err))
	}

	argsJSON := "{}"
	if params.Arguments != nil {
		argsJSON = string(params.Arguments)
	}

	// Create cancellable context tracked by request ID
	reqIDStr := string(req.ID)
	if reqIDStr != "" {
		var callCtx context.Context
		callCtx, reqIDStr = h.cancelTracker.TrackCallWithContext(ctx, reqIDStr)
		defer h.cancelTracker.Complete(reqIDStr)
		ctx = callCtx
	}

	resp, err := h.backend.CallTool(ctx, params.Name, argsJSON)
	// ... rest unchanged
```

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp && go test ./internal/cancel/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cancel/tracker.go internal/cancel/tracker_test.go internal/mcp/handler.go
git commit -m "fix: cancel.Tracker now cancels context.Context on Cancel(), enabling real cancellation propagation"
```

---

## Chunk 2: Auth Header Injection in Transports

### Task 5: Inject auth headers into context for SSE transport

The auth middleware reads headers from context via `GetAuthHeader()` / `GetAPIKeyHeader()`, but no transport ever calls `WithAuthHeader()` / `WithAPIKeyHeader()` to inject them. Auth is completely non-functional on all network transports.

**Files:**
- Modify: `internal/transport/sse.go:64-76`
- Test: `internal/transport/auth_inject_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/transport/auth_inject_test.go`:

```go
package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/middleware"
)

func TestSSETransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewSSETransport("localhost", 0)

	var capturedCtx context.Context
	go tp.Start(context.Background(), func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		capturedCtx = ctx
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	})

	// Wait for listener
	for tp.listener == nil {
		// spin briefly
	}
	addr := tp.listener.Addr().String()

	body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	req, _ := http.NewRequest("POST", "http://"+addr+"/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-API-Key", "my-api-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer test-token" {
		t.Fatalf("expected auth header 'Bearer test-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "my-api-key" {
		t.Fatalf("expected api key 'my-api-key', got %q", got)
	}

	tp.Close()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transport/... -run TestSSETransport_InjectsAuthHeaders -v`
Expected: FAIL — auth header is empty

- [ ] **Step 3: Add auth header injection to SSE transport**

In `internal/transport/sse.go`, update the POST handler (line ~70-76):

```go
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req mcp.JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Inject auth headers into context for middleware
	ctx := r.Context()
	if v := r.Header.Get("Authorization"); v != "" {
		ctx = middleware.WithAuthHeader(ctx, v)
	}
	if v := r.Header.Get("X-API-Key"); v != "" {
		ctx = middleware.WithAPIKeyHeader(ctx, v)
	}

	resp, err := handler(ctx, req)
	// ... rest unchanged
```

Add import: `"github.com/msilverblatt/protomcp/internal/middleware"`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transport/... -run TestSSETransport_InjectsAuthHeaders -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/transport/sse.go internal/transport/auth_inject_test.go
git commit -m "fix: inject Authorization and X-API-Key headers into context in SSE transport"
```

---

### Task 6: Inject auth headers into context for HTTP transport

**Files:**
- Modify: `internal/transport/http.go:64-76`
- Test: `internal/transport/auth_inject_test.go` (add test)

- [ ] **Step 1: Write failing test**

Add to `internal/transport/auth_inject_test.go`:

```go
func TestHTTPTransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewHTTPTransport("localhost", 0)

	var capturedCtx context.Context
	go tp.Start(context.Background(), func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		capturedCtx = ctx
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	})

	for tp.listener == nil {}
	addr := tp.listener.Addr().String()

	body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	req, _ := http.NewRequest("POST", "http://"+addr+"/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer http-token")
	req.Header.Set("X-API-Key", "http-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer http-token" {
		t.Fatalf("expected 'Bearer http-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "http-key" {
		t.Fatalf("expected 'http-key', got %q", got)
	}

	tp.Close()
}
```

- [ ] **Step 2: Add auth header injection to HTTP transport**

In `internal/transport/http.go`, same pattern as SSE — add to the POST handler after `json.NewDecoder`:

```go
ctx := r.Context()
if v := r.Header.Get("Authorization"); v != "" {
	ctx = middleware.WithAuthHeader(ctx, v)
}
if v := r.Header.Get("X-API-Key"); v != "" {
	ctx = middleware.WithAPIKeyHeader(ctx, v)
}
resp, err := handler(ctx, req)
```

Add import: `"github.com/msilverblatt/protomcp/internal/middleware"`

- [ ] **Step 3: Run tests**

Run: `go test ./internal/transport/... -run TestHTTPTransport_InjectsAuthHeaders -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/http.go internal/transport/auth_inject_test.go
git commit -m "fix: inject auth headers into context in HTTP transport"
```

---

### Task 7: Inject auth headers into context for WebSocket transport

**Files:**
- Modify: `internal/transport/ws.go:58-76`
- Test: `internal/transport/auth_inject_test.go` (add test)

- [ ] **Step 1: Write failing test**

Add to `internal/transport/auth_inject_test.go`:

```go
func TestWSTransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewWSTransport("localhost", 0)

	var capturedCtx context.Context
	var mu sync.Mutex
	go tp.Start(context.Background(), func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		mu.Lock()
		capturedCtx = ctx
		mu.Unlock()
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	})

	for tp.listener == nil {}
	addr := tp.listener.Addr().String()

	// WebSocket headers are set during the upgrade handshake
	wsConfig, err := websocket.NewConfig("ws://"+addr+"/ws", "http://"+addr)
	if err != nil {
		t.Fatal(err)
	}
	wsConfig.Header.Set("Authorization", "Bearer ws-token")
	wsConfig.Header.Set("X-API-Key", "ws-key")

	conn, err := websocket.DialConfig(wsConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	if err := websocket.Message.Send(conn, msg); err != nil {
		t.Fatal(err)
	}
	var reply string
	websocket.Message.Receive(conn, &reply)

	mu.Lock()
	defer mu.Unlock()
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer ws-token" {
		t.Fatalf("expected 'Bearer ws-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "ws-key" {
		t.Fatalf("expected 'ws-key', got %q", got)
	}

	tp.Close()
}
```

Add imports: `"sync"`, `"golang.org/x/net/websocket"`

- [ ] **Step 2: Add auth header injection to WebSocket transport**

The WebSocket transport is trickier — headers come from the upgrade request. The `websocket.Handler` wraps the HTTP handler, so we need to capture headers from the initial HTTP request.

In `internal/transport/ws.go`, update the `wsHandler` function (line ~51):

The WebSocket library's `conn.Request()` method gives access to the original HTTP request. Use it to inject headers into the context passed to `handler`:

```go
wsHandler := websocket.Handler(func(conn *websocket.Conn) {
	w.addConn(conn)
	defer func() {
		w.removeConn(conn)
		conn.Close()
	}()

	// Capture auth headers from the upgrade request
	upgradeReq := conn.Request()
	connCtx := ctx
	if v := upgradeReq.Header.Get("Authorization"); v != "" {
		connCtx = middleware.WithAuthHeader(connCtx, v)
	}
	if v := upgradeReq.Header.Get("X-API-Key"); v != "" {
		connCtx = middleware.WithAPIKeyHeader(connCtx, v)
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var raw []byte
		if err := websocket.Message.Receive(conn, &raw); err != nil {
			return
		}

		var req mcp.JSONRPCRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			continue
		}

		resp, err := handler(connCtx, req) // use connCtx with auth headers
		// ... rest unchanged
```

Add import: `"github.com/msilverblatt/protomcp/internal/middleware"`

- [ ] **Step 3: Run tests**

Run: `go test ./internal/transport/... -run TestWSTransport_InjectsAuthHeaders -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/ws.go internal/transport/auth_inject_test.go
git commit -m "fix: inject auth headers into context in WebSocket transport"
```

---

### Task 8: Inject auth headers into context for gRPC transport

**Files:**
- Modify: `internal/transport/grpc.go:76-82`
- Test: `internal/transport/auth_inject_test.go` (add test)

- [ ] **Step 1: Add auth header injection to gRPC transport**

gRPC uses metadata instead of HTTP headers. In `internal/transport/grpc.go`, update the `call` method:

```go
import (
	"google.golang.org/grpc/metadata"
	"github.com/msilverblatt/protomcp/internal/middleware"
)

func (s *grpcMCPServer) call(ctx context.Context, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	// Extract auth from gRPC metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			ctx = middleware.WithAuthHeader(ctx, vals[0])
		}
		if vals := md.Get("x-api-key"); len(vals) > 0 {
			ctx = middleware.WithAPIKeyHeader(ctx, vals[0])
		}
	}

	var jsonReq mcp.JSONRPCRequest
	if err := json.Unmarshal([]byte(req.GetValue()), &jsonReq); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid JSON-RPC payload: %v", err)
	}

	resp, err := s.handler(ctx, jsonReq)
	// ... rest unchanged
```

- [ ] **Step 2: Write test**

Add to `internal/transport/auth_inject_test.go`:

```go
func TestGRPCTransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewGRPCTransport("localhost", 0)

	var capturedCtx context.Context
	var mu sync.Mutex

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		mu.Lock()
		capturedCtx = ctx
		mu.Unlock()
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: json.RawMessage(`1`)}, nil
	})

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Connect with gRPC metadata
	conn, err := grpc.Dial(
		fmt.Sprintf("localhost:%d", tp.Port()),
		grpc.WithInsecure(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	md := metadata.New(map[string]string{
		"authorization": "Bearer grpc-token",
		"x-api-key":     "grpc-key",
	})
	callCtx := metadata.NewOutgoingContext(context.Background(), md)

	// Make a raw gRPC call using the service desc
	in := wrapperspb.String(`{"jsonrpc":"2.0","method":"tools/list","id":1}`)
	out := new(wrapperspb.StringValue)
	err = conn.Invoke(callCtx, "/protomcp.MCPService/Call", in, out)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer grpc-token" {
		t.Fatalf("expected 'Bearer grpc-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "grpc-key" {
		t.Fatalf("expected 'grpc-key', got %q", got)
	}

	cancel()
}
```

Note: The gRPC test needs `GRPCTransport` to expose the bound port. Add a `Port()` method to the struct if needed — or use the `listener.Addr()` pattern. This may require exposing the listener (adding a `listener` field to `GRPCTransport` and a `Port()` accessor).

Add imports: `"time"`, `"google.golang.org/grpc"`, `"google.golang.org/grpc/metadata"`, `"google.golang.org/protobuf/types/known/wrapperspb"`

- [ ] **Step 3: Run tests**

Run: `go test ./internal/transport/... -run TestGRPCTransport_InjectsAuthHeaders -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/transport/grpc.go internal/transport/auth_inject_test.go
git commit -m "fix: inject auth from gRPC metadata into context"
```

---

### Task 9: Use constant-time comparison for auth token validation

The auth middleware uses `==` for string comparison, which is vulnerable to timing side-channel attacks.

**Files:**
- Modify: `internal/middleware/auth.go:70-78`
- Test: `internal/middleware/auth_test.go`

- [ ] **Step 1: Fix auth comparison**

In `internal/middleware/auth.go`, add import `"crypto/subtle"` and replace:

```go
// OLD:
case "token":
	header := GetAuthHeader(ctx)
	expected := "Bearer " + c.value
	if header != expected {
		return nil, fmt.Errorf("unauthorized: invalid or missing Bearer token")
	}
case "apikey":
	header := GetAPIKeyHeader(ctx)
	if header != c.value {
		return nil, fmt.Errorf("unauthorized: invalid or missing API key")
	}

// NEW:
case "token":
	header := GetAuthHeader(ctx)
	expected := "Bearer " + c.value
	if subtle.ConstantTimeCompare([]byte(header), []byte(expected)) != 1 {
		return nil, fmt.Errorf("unauthorized: invalid or missing Bearer token")
	}
case "apikey":
	header := GetAPIKeyHeader(ctx)
	if subtle.ConstantTimeCompare([]byte(header), []byte(c.value)) != 1 {
		return nil, fmt.Errorf("unauthorized: invalid or missing API key")
	}
```

- [ ] **Step 2: Run existing auth tests**

Run: `go test ./internal/middleware/... -run TestAuth -v`
Expected: PASS (existing tests should still pass)

- [ ] **Step 3: Commit**

```bash
git add internal/middleware/auth.go
git commit -m "fix: use constant-time comparison for auth token validation"
```

---

## Chunk 3: Wiring Features (custom middleware, allTools reload, message dispatch)

### Task 10: Fix allTools not updating on reload

**Files:**
- Modify: `cmd/protomcp/main.go:79,125-136`

- [ ] **Step 1: Make `toolBackend.allTools` update on reload**

The fix is simple — update `backend.allTools` in the reload callback. Since `toolBackend` is accessed from multiple goroutines (file watcher goroutine sets `allTools`, transport goroutine reads via `ActiveTools()`), we need a mutex.

In `cmd/protomcp/main.go`, update the `toolBackend` struct:

```go
type toolBackend struct {
	pm       *process.Manager
	tlm      *toollist.Manager
	mu       sync.RWMutex
	allTools []*pb.ToolDefinition
}

func (b *toolBackend) ActiveTools() []*pb.ToolDefinition {
	b.mu.RLock()
	defer b.mu.RUnlock()
	activeNames := b.tlm.GetActive()
	nameSet := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		nameSet[n] = true
	}
	var result []*pb.ToolDefinition
	for _, t := range b.allTools {
		if nameSet[t.Name] {
			result = append(result, t)
		}
	}
	return result
}

func (b *toolBackend) UpdateTools(tools []*pb.ToolDefinition) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allTools = tools
}
```

Add import `"sync"`.

Then in the reload callback (line ~125):

```go
w, err := reload.NewWatcher(cfg.File, nil, func() {
	slog.Info("file changed, reloading...")
	newTools, err := pm.Reload(ctx)
	if err != nil {
		slog.Error("reload failed", "error", err)
		return
	}
	// Update the backend's tool definitions
	backend.UpdateTools(newTools)
	newNames := make([]string, len(newTools))
	for i, t := range newTools {
		newNames[i] = t.Name
	}
	oldActive := tlm.GetActive()
	tlm.SetRegistered(newNames)
	newActive := tlm.GetActive()
	if !slicesEqual(oldActive, newActive) {
		slog.Info("tool list changed, notifying client")
		tp.SendNotification(mcp.ListChangedNotification())
	}
})
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/protomcp/`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add cmd/protomcp/main.go
git commit -m "fix: update allTools on reload so MCP client sees new tool definitions"
```

---

### Task 11: Wire custom middleware into the middleware chain

The `CustomMiddleware` function exists in `internal/middleware/custom.go` and the process manager collects `Middlewares()` during handshake, but `main.go` never connects them.

**Files:**
- Modify: `cmd/protomcp/main.go:92-116`

- [ ] **Step 1: Wire custom middleware after collecting registered middleware**

In `cmd/protomcp/main.go`, after constructing the backend and before creating the middleware chain, add the custom middleware:

```go
// 6. Apply middleware
middlewares := []middleware.Middleware{
	middleware.Logging(logger),
	middleware.ErrorFormatting(),
}

// Wire custom middleware from tool process (if any registered during handshake)
registeredMW := pm.Middlewares()
if len(registeredMW) > 0 {
	var customMWs []middleware.RegisteredMW
	for _, rmw := range registeredMW {
		customMWs = append(customMWs, middleware.RegisteredMW{
			Name:     rmw.Name,
			Priority: rmw.Priority,
		})
	}
	middlewares = append(middlewares, middleware.CustomMiddleware(pm, customMWs))
}

if len(cfg.Auth) > 0 {
	// ... auth middleware unchanged
}
```

Note: `process.Manager` already implements `MiddlewareDispatcher` because it has the `SendMiddlewareIntercept` method. Verify the interface match.

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/protomcp/`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add cmd/protomcp/main.go
git commit -m "fix: wire custom middleware from tool process into middleware chain"
```

---

### Task 12: Add message dispatch for progress, logs, and tool list management

The process manager's `readLoop` only handles messages with a `request_id` (routed to `pending`) or without (dumped to `handshakeCh`). Progress notifications, log messages, and tool list management messages from the tool process arrive without `request_id` and are silently dropped after handshake completes.

**Files:**
- Modify: `internal/process/manager.go:37-53,450-496`
- Modify: `cmd/protomcp/main.go`
- Test: `internal/process/manager_test.go`

- [ ] **Step 1: Add callback fields to Manager**

In `internal/process/manager.go`, add callback fields to the `Manager` struct:

```go
type Manager struct {
	cfg      ManagerConfig
	cmd      *exec.Cmd
	conn     net.Conn
	listener net.Listener
	mu       sync.Mutex
	pending  map[string]chan *pb.Envelope
	tools       []*pb.ToolDefinition
	middlewares []RegisteredMiddleware
	crashCh     chan error
	stopCh   chan struct{}
	readWg   sync.WaitGroup
	nextID   int

	handshakeCh chan *pb.Envelope

	// Callbacks for unsolicited messages from the tool process
	onProgress    func(*pb.ProgressNotification)
	onLog         func(*pb.LogMessage)
	onEnableTools func([]string)
	onDisableTools func([]string)
}
```

Add setter methods:

```go
func (m *Manager) OnProgress(fn func(*pb.ProgressNotification)) { m.onProgress = fn }
func (m *Manager) OnLog(fn func(*pb.LogMessage)) { m.onLog = fn }
func (m *Manager) OnEnableTools(fn func([]string)) { m.onEnableTools = fn }
func (m *Manager) OnDisableTools(fn func([]string)) { m.onDisableTools = fn }
```

- [ ] **Step 2: Update readLoop to dispatch unsolicited messages**

Replace the unsolicited message handling in `readLoop` (lines 476-482):

```go
reqID := env.GetRequestId()
if reqID == "" {
	// Route unsolicited messages by type
	switch msg := env.Msg.(type) {
	case *pb.Envelope_ToolList:
		// Handshake/reload tool list
		select {
		case m.handshakeCh <- env:
		default:
		}
	case *pb.Envelope_RegisterMiddleware:
		// Handshake middleware registration
		select {
		case m.handshakeCh <- env:
		default:
		}
	case *pb.Envelope_ReloadResponse:
		// Handshake complete signal
		select {
		case m.handshakeCh <- env:
		default:
		}
	case *pb.Envelope_Progress:
		if m.onProgress != nil {
			m.onProgress(msg.Progress)
		}
	case *pb.Envelope_Log:
		if m.onLog != nil {
			m.onLog(msg.Log)
		}
	case *pb.Envelope_EnableTools:
		if m.onEnableTools != nil {
			m.onEnableTools(msg.EnableTools.ToolNames)
		}
	case *pb.Envelope_DisableTools:
		if m.onDisableTools != nil {
			m.onDisableTools(msg.DisableTools.ToolNames)
		}
	default:
		// Unknown unsolicited message — try handshake channel as fallback
		select {
		case m.handshakeCh <- env:
		default:
		}
	}
	continue
}
```

- [ ] **Step 3: Wire callbacks in main.go**

In `cmd/protomcp/main.go`, after creating `pm` and the transport, wire the callbacks:

```go
// Wire progress forwarding
pm.OnProgress(func(msg *pb.ProgressNotification) {
	notif := mcp.ProgressNotification(msg)
	tp.SendNotification(notif)
})

// Wire server log forwarding
pm.OnLog(func(msg *pb.LogMessage) {
	notif := mcp.LogNotification(msg)
	tp.SendNotification(notif)
})

// Wire tool list management from tool process
pm.OnEnableTools(func(names []string) {
	tlm.Enable(names)
	tp.SendNotification(mcp.ListChangedNotification())
})
pm.OnDisableTools(func(names []string) {
	tlm.Disable(names)
	tp.SendNotification(mcp.ListChangedNotification())
})
```

This also requires adding `ProgressNotification` and `LogNotification` helper functions to the `mcp` package. Add to `internal/mcp/handler.go` (or a new `internal/mcp/notifications.go`):

```go
func ProgressNotification(msg *pb.ProgressNotification) JSONRPCNotification {
	params := map[string]any{
		"progressToken": msg.ProgressToken,
		"progress":      msg.Progress,
	}
	if msg.Total > 0 {
		params["total"] = msg.Total
	}
	if msg.Message != "" {
		params["message"] = msg.Message
	}
	data, _ := json.Marshal(params)
	return JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/progress",
		Params:  data,
	}
}

func LogNotification(msg *pb.LogMessage) JSONRPCNotification {
	params := map[string]any{"level": msg.Level}
	if msg.Logger != "" {
		params["logger"] = msg.Logger
	}
	if msg.DataJson != "" {
		var data any
		if err := json.Unmarshal([]byte(msg.DataJson), &data); err == nil {
			params["data"] = data
		} else {
			params["data"] = msg.DataJson
		}
	}
	d, _ := json.Marshal(params)
	return JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/message",
		Params:  d,
	}
}
```

You'll need to add a `Params` field to `JSONRPCNotification` if it doesn't exist. Check the `mcp` types file.

- [ ] **Step 4: Verify compilation**

Run: `go build ./cmd/protomcp/`
Expected: builds successfully

- [ ] **Step 5: Commit**

```bash
git add internal/process/manager.go cmd/protomcp/main.go internal/mcp/
git commit -m "fix: dispatch progress, log, and tool list messages from tool process instead of dropping them"
```

---

## Chunk 4: Documentation Accuracy

### Task 13: Fix README comparison table accuracy

The README comparison table makes claims about FastMCP and MCP SDKs that are inaccurate. Fix to be honest.

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update comparison table**

Replace the comparison table with an honest version:

```markdown
## Comparison

| Feature | pmcp | FastMCP (Python) | MCP SDKs |
|---------|------|------------------|----------|
| Language support | Any (Python, TS, Go, Rust, ...) | Python only | One SDK per language |
| Hot reload | Built-in (all languages) | SSE via uvicorn `--reload` | Manual (e.g., nodemon) |
| Dynamic tool lists | Built-in | Programmatic add/remove | Via `listChanged` notification |
| Custom middleware | Built-in (before/after hooks) | Dependency injection | Manual |
| Authentication | Built-in (token, apikey) | Manual | Manual |
| Validation | `pmcp validate` | No | No |
| Transports | stdio, SSE, HTTP, WS, gRPC | stdio, SSE | Varies by SDK |
| Structured output | Yes | Typed returns | Varies |
| Single binary | Yes (Go) | No (Python runtime) | No (per-language) |
```

Remove the "Async tasks" row — while the proto and task manager exist, the feature is not complete enough to advertise.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "fix: make comparison table accurate and honest about competitor capabilities"
```

---

### Task 14: Remove dead code

**Files:**
- Delete or mark: `internal/progress/progress.go` (now superseded by direct callback in manager)
- Delete or mark: `internal/serverlog/forwarder.go` (now superseded by direct callback in manager)

- [ ] **Step 1: Remove dead modules**

After Task 12 wires progress and log forwarding directly through callbacks, the `progress.Proxy` and `serverlog.Forwarder` types are dead code (they were never called before, and the new approach bypasses them). Remove both files.

If any tests reference them, remove those too.

- [ ] **Step 2: Verify**

Run: `go build ./...`
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git rm internal/progress/progress.go internal/serverlog/forwarder.go
git commit -m "chore: remove dead progress.Proxy and serverlog.Forwarder (superseded by direct callbacks)"
```

---

### Task 15: Run full test suite and fix any breakage

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/msilverblatt/hotmcp && go test ./... -v`

- [ ] **Step 2: Run Go SDK tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/go && go test ./... -v`

- [ ] **Step 3: Run Rust SDK tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/rust && cargo test`

- [ ] **Step 4: Run Python SDK tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/python && uv run pytest -v`

- [ ] **Step 5: Run TypeScript SDK tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/typescript && npm run build && npx vitest run`

- [ ] **Step 6: Fix any failures and commit**

```bash
git add -A
git commit -m "fix: resolve test failures from critical fixes"
```
