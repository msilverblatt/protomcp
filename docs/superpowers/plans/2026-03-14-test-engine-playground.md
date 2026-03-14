# Test Engine & Playground Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `pmcp test` CLI and `pmcp playground` web UI, both powered by a shared test engine that acts as a full MCP client with protocol tracing.

**Architecture:** A shared `internal/testengine/` package creates an MCP client connected to the tool process through the full protocol stack (bridge → mcp.Server → InMemoryTransport → mcp.Client). The CLI and playground are thin interfaces on top.

**Tech Stack:** Go (test engine, playground backend), React + Tailwind + Vite (playground frontend), nhooyr.io/websocket (WebSocket server), official MCP Go SDK (mcp.Client, InMemoryTransport, LoggingTransport)

---

## File Structure

### Files to CREATE

**Test Engine:**
- `internal/testengine/engine.go` — Engine struct, New(), Start(), Stop(), MCP operation wrappers
- `internal/testengine/trace.go` — TraceLog, TraceEntry, trace io.Writer that parses SDK log lines
- `internal/testengine/backend.go` — toolBackend adapter (wraps process.Manager + toollist.Manager for bridge.FullBackend)
- `internal/testengine/engine_test.go` — unit tests

**CLI:**
- `internal/cli/test.go` — RunTestList(), RunTestCall() functions
- `internal/cli/format.go` — table formatting helpers for CLI output

**Playground Backend:**
- `internal/playground/server.go` — HTTP server, embed.FS, mux setup
- `internal/playground/handlers.go` — REST endpoint handlers
- `internal/playground/ws.go` — WebSocket hub, connection management, event broadcast

**Playground Frontend:**
- `internal/playground/frontend/package.json`
- `internal/playground/frontend/vite.config.ts`
- `internal/playground/frontend/tailwind.config.ts`
- `internal/playground/frontend/index.html`
- `internal/playground/frontend/src/App.tsx`
- `internal/playground/frontend/src/types.ts`
- `internal/playground/frontend/src/hooks/useWebSocket.ts`
- `internal/playground/frontend/src/hooks/useApi.ts`
- `internal/playground/frontend/src/components/TopBar.tsx`
- `internal/playground/frontend/src/components/FeaturePicker.tsx`
- `internal/playground/frontend/src/components/ToolForm.tsx`
- `internal/playground/frontend/src/components/ResourceForm.tsx`
- `internal/playground/frontend/src/components/PromptForm.tsx`
- `internal/playground/frontend/src/components/ResultView.tsx`
- `internal/playground/frontend/src/components/TracePanel.tsx`
- `internal/playground/frontend/src/components/TraceEntry.tsx`
- `internal/playground/frontend/src/components/ProgressBar.tsx`
- `internal/playground/frontend/src/components/History.tsx`

### Files to MODIFY

- `internal/config/config.go` — Add `test` and `playground` commands, `TestSubcommand`, `TestToolName`, `TestArgs` fields
- `cmd/protomcp/main.go` — Add `test` and `playground` command dispatch
- `Makefile` — Add `playground-frontend` build target
- `go.mod` — Add `nhooyr.io/websocket`

### Files UNCHANGED

- `internal/bridge/` — used as-is by the test engine
- `internal/process/` — used as-is by the test engine
- `internal/toollist/` — used as-is by the test engine backend adapter
- `internal/reload/` — used as-is for hot reload in the engine

---

## Chunk 1: Test Engine Foundation

### Task 1: Add nhooyr.io/websocket dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

```bash
cd /Users/msilverblatt/hotmcp-sdk-integration && go get nhooyr.io/websocket@latest
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy && go build ./cmd/... ./internal/...
```
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add nhooyr.io/websocket for playground"
```

---

### Task 2: Create TraceLog and trace writer

**Files:**
- Create: `internal/testengine/trace.go`
- Test: `internal/testengine/trace_test.go`

- [ ] **Step 1: Write the trace test**

```go
package testengine

import (
	"strings"
	"testing"
	"time"
)

func TestTraceWriter(t *testing.T) {
	tl := NewTraceLog()
	w := tl.Writer()

	// Simulate LoggingTransport output
	w.Write([]byte("write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n"))
	w.Write([]byte("read: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2025-03-26\"}}\n"))

	entries := tl.Entries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Direction != "send" {
		t.Errorf("expected direction 'send', got %q", entries[0].Direction)
	}
	if entries[0].Method != "initialize" {
		t.Errorf("expected method 'initialize', got %q", entries[0].Method)
	}
	if entries[1].Direction != "recv" {
		t.Errorf("expected direction 'recv', got %q", entries[1].Direction)
	}
}

func TestTraceLogSubscribe(t *testing.T) {
	tl := NewTraceLog()
	ch := tl.Subscribe()
	defer tl.Unsubscribe(ch)

	w := tl.Writer()
	w.Write([]byte("write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n"))

	select {
	case entry := <-ch:
		if entry.Method != "ping" {
			t.Errorf("expected method 'ping', got %q", entry.Method)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for trace entry")
	}
}

func TestTraceLogClear(t *testing.T) {
	tl := NewTraceLog()
	w := tl.Writer()
	w.Write([]byte("write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n"))
	tl.Clear()
	if len(tl.Entries()) != 0 {
		t.Errorf("expected 0 entries after clear, got %d", len(tl.Entries()))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test -v ./internal/testengine/ -run TestTrace
```
Expected: FAIL — types not defined

- [ ] **Step 3: Write the implementation**

```go
package testengine

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

// TraceEntry represents a single JSON-RPC message captured from the protocol.
type TraceEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "send" or "recv"
	Raw       string    `json:"raw"`       // full JSON-RPC message
	Method    string    `json:"method"`    // parsed method name (empty for responses)
}

// TraceLog collects protocol trace entries and broadcasts to subscribers.
type TraceLog struct {
	mu      sync.Mutex
	entries []TraceEntry
	subs    []chan TraceEntry
}

// NewTraceLog creates an empty TraceLog.
func NewTraceLog() *TraceLog {
	return &TraceLog{}
}

// Writer returns an io.Writer that parses LoggingTransport output lines
// into TraceEntry structs. Each line has the format:
//   write: <json>
//   read: <json>
func (t *TraceLog) Writer() io.Writer {
	return &traceWriter{log: t}
}

// Entries returns a snapshot of all trace entries.
func (t *TraceLog) Entries() []TraceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]TraceEntry{}, t.entries...)
}

// Subscribe returns a channel that receives new trace entries as they arrive.
func (t *TraceLog) Subscribe() chan TraceEntry {
	t.mu.Lock()
	defer t.mu.Unlock()
	ch := make(chan TraceEntry, 64)
	t.subs = append(t.subs, ch)
	return ch
}

// Unsubscribe removes a subscriber channel.
func (t *TraceLog) Unsubscribe(ch chan TraceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, s := range t.subs {
		if s == ch {
			t.subs = append(t.subs[:i], t.subs[i+1:]...)
			close(ch)
			return
		}
	}
}

// Clear removes all stored entries.
func (t *TraceLog) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = nil
}

func (t *TraceLog) append(entry TraceEntry) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, entry)
	for _, ch := range t.subs {
		select {
		case ch <- entry:
		default: // drop if subscriber is slow
		}
	}
}

// traceWriter implements io.Writer by parsing LoggingTransport log lines.
type traceWriter struct {
	log *TraceLog
	buf []byte
}

func (w *traceWriter) Write(p []byte) (n int, err error) {
	w.buf = append(w.buf, p...)
	scanner := bufio.NewScanner(strings.NewReader(string(w.buf)))
	var consumed int
	for scanner.Scan() {
		line := scanner.Text()
		consumed += len(line) + 1 // +1 for newline

		var direction, raw string
		if strings.HasPrefix(line, "write: ") {
			direction = "send"
			raw = strings.TrimPrefix(line, "write: ")
		} else if strings.HasPrefix(line, "read: ") {
			direction = "recv"
			raw = strings.TrimPrefix(line, "read: ")
		} else {
			continue
		}

		method := parseMethod(raw)
		w.log.append(TraceEntry{
			Timestamp: time.Now(),
			Direction: direction,
			Raw:       raw,
			Method:    method,
		})
	}
	w.buf = w.buf[consumed:]
	return len(p), nil
}

// parseMethod extracts the "method" field from a JSON-RPC message, if present.
func parseMethod(raw string) string {
	var msg struct {
		Method string `json:"method"`
	}
	json.Unmarshal([]byte(raw), &msg)
	return msg.Method
}
```

- [ ] **Step 4: Run tests**

```bash
go test -v ./internal/testengine/ -run TestTrace
```
Expected: all 3 tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/testengine/
git commit -m "feat: add TraceLog — protocol trace capture with pub/sub"
```

---

### Task 3: Create backend adapter

**Files:**
- Create: `internal/testengine/backend.go`

- [ ] **Step 1: Write the backend adapter**

This adapts `process.Manager` + `toollist.Manager` to satisfy `bridge.FullBackend`. It's the same pattern as `toolBackend` in `cmd/protomcp/main.go` but extended for all backend interfaces.

```go
package testengine

import (
	"context"
	"sync"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/internal/toollist"
)

// backend adapts process.Manager + toollist.Manager to bridge.FullBackend.
type backend struct {
	pm       *process.Manager
	tlm      *toollist.Manager
	mu       sync.RWMutex
	allTools []*pb.ToolDefinition
}

func newBackend(pm *process.Manager, tlm *toollist.Manager, tools []*pb.ToolDefinition) *backend {
	return &backend{pm: pm, tlm: tlm, allTools: tools}
}

func (b *backend) ActiveTools() []*pb.ToolDefinition {
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

func (b *backend) UpdateTools(tools []*pb.ToolDefinition) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allTools = tools
}

func (b *backend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return b.pm.CallTool(ctx, name, argsJSON)
}

func (b *backend) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error) {
	return b.pm.ListResources(ctx)
}

func (b *backend) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error) {
	return b.pm.ListResourceTemplates(ctx)
}

func (b *backend) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error) {
	return b.pm.ReadResource(ctx, uri)
}

func (b *backend) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error) {
	return b.pm.ListPrompts(ctx)
}

func (b *backend) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error) {
	return b.pm.GetPrompt(ctx, name, argsJSON)
}

func (b *backend) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error) {
	return b.pm.Complete(ctx, refType, refName, argName, argValue)
}

func (b *backend) SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error {
	return b.pm.SendSamplingResponse(reqID, resp)
}

func (b *backend) OnSampling(fn func(*pb.SamplingRequest, string)) {
	b.pm.OnSampling(fn)
}

func (b *backend) SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error {
	return b.pm.SendListRootsResponse(reqID, resp)
}

func (b *backend) OnListRoots(fn func(string)) {
	b.pm.OnListRoots(fn)
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/testengine/
```
Expected: builds successfully

- [ ] **Step 3: Commit**

```bash
git add internal/testengine/backend.go
git commit -m "feat: add backend adapter — bridges process.Manager to bridge.FullBackend"
```

---

### Task 4: Create the Engine

**Files:**
- Create: `internal/testengine/engine.go`
- Test: `internal/testengine/engine_test.go`

This is the core. The engine wires everything together: process manager → bridge → mcp.Server → InMemoryTransport → mcp.Client.

- [ ] **Step 1: Write the engine**

```go
package testengine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msilverblatt/protomcp/internal/bridge"
	"github.com/msilverblatt/protomcp/internal/config"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/internal/toollist"
)

// CallResult wraps an MCP tool call result with metadata.
type CallResult struct {
	Result        *mcp.CallToolResult `json:"result"`
	Duration      time.Duration       `json:"duration_ms"`
	ToolsEnabled  []string            `json:"tools_enabled,omitempty"`
	ToolsDisabled []string            `json:"tools_disabled,omitempty"`
}

// Option configures the Engine.
type Option func(*engineConfig)

type engineConfig struct {
	runtime     string
	socketPath  string
	callTimeout time.Duration
	logger      *slog.Logger
}

// WithRuntime overrides the auto-detected runtime command.
func WithRuntime(cmd string) Option {
	return func(c *engineConfig) { c.runtime = cmd }
}

// WithCallTimeout sets the timeout for tool calls.
func WithCallTimeout(d time.Duration) Option {
	return func(c *engineConfig) { c.callTimeout = d }
}

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *engineConfig) { c.logger = l }
}

// Engine is a test engine that acts as a full MCP client.
type Engine struct {
	file    string
	cfg     engineConfig
	pm      *process.Manager
	br      *bridge.Bridge
	be      *backend
	tlm     *toollist.Manager
	client  *mcp.Client
	session *mcp.ClientSession
	trace   *TraceLog

	// Tool list change tracking
	toolsChangedMu   sync.Mutex
	toolsChangedFn   func([]*mcp.Tool)
	lastEnabled       []string
	lastDisabled      []string
}

// New creates a new test engine for the given file.
func New(file string, opts ...Option) *Engine {
	cfg := engineConfig{
		callTimeout: 5 * time.Minute,
		logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Engine{
		file:  file,
		cfg:   cfg,
		trace: NewTraceLog(),
	}
}

// Start starts the tool process, wires the bridge, and connects the MCP client.
func (e *Engine) Start(ctx context.Context) error {
	// 1. Determine runtime
	var runtimeCmd string
	var runtimeArgs []string
	if e.cfg.runtime != "" {
		runtimeCmd = e.cfg.runtime
		runtimeArgs = []string{e.file}
	} else {
		runtimeCmd, runtimeArgs = config.RuntimeCommand(e.file)
	}

	// 2. Start process manager
	e.pm = process.NewManager(process.ManagerConfig{
		File:        e.file,
		RuntimeCmd:  runtimeCmd,
		RuntimeArgs: runtimeArgs,
		MaxRetries:  3,
		CallTimeout: e.cfg.callTimeout,
	})

	tools, err := e.pm.Start(ctx)
	if err != nil {
		return fmt.Errorf("start tool process: %w", err)
	}

	// 3. Create tool list manager
	e.tlm = toollist.New()
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}
	e.tlm.SetRegistered(toolNames)

	// 4. Create backend adapter
	e.be = newBackend(e.pm, e.tlm, tools)

	// 5. Create bridge (registers proxy handlers on mcp.Server)
	e.br = bridge.New(e.be, e.cfg.logger)
	e.br.SyncTools()
	e.br.SyncResources()
	e.br.SyncPrompts()

	// 6. Wire tool list change callbacks
	e.pm.OnEnableTools(func(names []string) {
		e.tlm.Enable(names)
		e.toolsChangedMu.Lock()
		e.lastEnabled = append(e.lastEnabled, names...)
		e.toolsChangedMu.Unlock()
		e.br.SyncTools()
	})
	e.pm.OnDisableTools(func(names []string) {
		e.tlm.Disable(names)
		e.toolsChangedMu.Lock()
		e.lastDisabled = append(e.lastDisabled, names...)
		e.toolsChangedMu.Unlock()
		e.br.SyncTools()
	})

	// 7. Create in-memory transport pair
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Wrap client transport with LoggingTransport for protocol tracing
	tracingTransport := &mcp.LoggingTransport{
		Transport: clientTransport,
		Writer:    e.trace.Writer(),
	}

	// 8. Connect server first (required ordering)
	serverDone := make(chan error, 1)
	go func() {
		_, err := e.br.Server.Connect(ctx, serverTransport, nil)
		serverDone <- err
	}()

	// 9. Create and connect MCP client
	e.client = mcp.NewClient(&mcp.Implementation{
		Name:    "pmcp-test",
		Version: "1.0.0",
	}, &mcp.ClientOptions{
		ToolListChangedHandler: func(ctx context.Context, req *mcp.ToolListChangedRequest) {
			// Re-fetch tool list and notify subscribers
			if e.session != nil {
				result, err := e.session.ListTools(ctx, nil)
				if err == nil && e.toolsChangedFn != nil {
					e.toolsChangedFn(result.Tools)
				}
			}
		},
	})

	e.session, err = e.client.Connect(ctx, tracingTransport, nil)
	if err != nil {
		e.pm.Stop()
		return fmt.Errorf("connect MCP client: %w", err)
	}

	// Check server connected OK
	select {
	case sErr := <-serverDone:
		if sErr != nil {
			e.session.Close()
			e.pm.Stop()
			return fmt.Errorf("connect MCP server: %w", sErr)
		}
	case <-time.After(5 * time.Second):
		e.session.Close()
		e.pm.Stop()
		return fmt.Errorf("server connect timed out")
	}

	return nil
}

// Stop shuts down the engine.
func (e *Engine) Stop() {
	if e.session != nil {
		e.session.Close()
	}
	if e.pm != nil {
		e.pm.Stop()
	}
}

// Trace returns the protocol trace log.
func (e *Engine) Trace() *TraceLog {
	return e.trace
}

// OnToolsChanged registers a callback for tool list changes.
func (e *Engine) OnToolsChanged(fn func([]*mcp.Tool)) {
	e.toolsChangedFn = fn
}

// ListTools returns all registered tools via MCP protocol.
func (e *Engine) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	result, err := e.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool calls a tool by name with the given arguments.
func (e *Engine) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	// Clear tracked changes
	e.toolsChangedMu.Lock()
	e.lastEnabled = nil
	e.lastDisabled = nil
	e.toolsChangedMu.Unlock()

	start := time.Now()
	result, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	duration := time.Since(start)

	if err != nil {
		return nil, err
	}

	e.toolsChangedMu.Lock()
	enabled := append([]string{}, e.lastEnabled...)
	disabled := append([]string{}, e.lastDisabled...)
	e.toolsChangedMu.Unlock()

	return &CallResult{
		Result:        result,
		Duration:      duration,
		ToolsEnabled:  enabled,
		ToolsDisabled: disabled,
	}, nil
}

// ListResources returns all registered resources via MCP protocol.
func (e *Engine) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	result, err := e.session.ListResources(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ReadResource reads a resource by URI.
func (e *Engine) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	return e.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
}

// ListPrompts returns all registered prompts via MCP protocol.
func (e *Engine) ListPrompts(ctx context.Context) ([]*mcp.Prompt, error) {
	result, err := e.session.ListPrompts(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Prompts, nil
}

// GetPrompt gets a prompt by name with arguments.
func (e *Engine) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	return e.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
}

// Reload triggers a hot reload of the tool process.
func (e *Engine) Reload(ctx context.Context) error {
	newTools, err := e.pm.Reload(ctx)
	if err != nil {
		return err
	}
	e.be.UpdateTools(newTools)
	newNames := make([]string, len(newTools))
	for i, t := range newTools {
		newNames[i] = t.Name
	}
	e.tlm.SetRegistered(newNames)
	e.br.SyncTools()
	e.br.SyncResources()
	e.br.SyncPrompts()
	return nil
}
```

Note: This file needs `import "sync"` — add it to the import block.

- [ ] **Step 2: Write the E2E test**

```go
// In engine_test.go
package testengine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func repoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func init() {
	root := repoRoot()
	pythonPath := filepath.Join(root, "sdk", "python", "src") +
		string(os.PathListSeparator) +
		filepath.Join(root, "sdk", "python", "gen")
	existing := os.Getenv("PYTHONPATH")
	if existing != "" {
		pythonPath = pythonPath + string(os.PathListSeparator) + existing
	}
	os.Setenv("PYTHONPATH", pythonPath)
}

func TestEngineListTools(t *testing.T) {
	fixture := filepath.Join(repoRoot(), "test", "e2e", "fixtures", "simple_tool.py")
	e := New(fixture)
	ctx := context.Background()

	if err := e.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	defer e.Stop()

	tools, err := e.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least 1 tool")
	}

	// Verify trace captured the protocol exchange
	entries := e.Trace().Entries()
	if len(entries) < 2 {
		t.Errorf("expected at least 2 trace entries (init + list), got %d", len(entries))
	}
}

func TestEngineCallTool(t *testing.T) {
	fixture := filepath.Join(repoRoot(), "test", "e2e", "fixtures", "simple_tool.py")
	e := New(fixture)
	ctx := context.Background()

	if err := e.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	defer e.Stop()

	result, err := e.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.Result.IsError {
		t.Fatalf("tool call returned error")
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}
```

- [ ] **Step 3: Build the binary (needed for process manager)**

```bash
go build -o bin/pmcp ./cmd/protomcp/
```

- [ ] **Step 4: Run tests**

```bash
go test -v -timeout 30s ./internal/testengine/
```
Expected: all tests pass

- [ ] **Step 5: Commit**

```bash
git add internal/testengine/
git commit -m "feat: add test engine — MCP client with protocol tracing"
```

---

## Chunk 2: CLI Commands

### Task 5: Add `test` and `playground` commands to config parser

**Files:**
- Modify: `internal/config/config.go`

- [ ] **Step 1: Add new fields to Config**

Add to the `Config` struct:
```go
// Test command fields
TestSubcommand string // "list", "call", "scenario"
TestToolName   string // tool name for "call"
TestArgs       string // --args JSON string for "call"
ShowTrace      bool   // --trace flag (default true)
```

- [ ] **Step 2: Add "test" and "playground" to command validation**

Change the command check from:
```go
if cmd != "dev" && cmd != "run" && cmd != "validate" {
```
to:
```go
if cmd != "dev" && cmd != "run" && cmd != "validate" && cmd != "test" && cmd != "playground" {
```

Update the error message and usage string accordingly.

- [ ] **Step 3: Parse test subcommands**

After the file argument parsing, if `cfg.Command == "test"`, parse the next positional arg as the subcommand ("list", "call", "scenario"). For "call", parse the next arg as the tool name. Add `--args` flag parsing in the flag loop.

Add `--trace` flag (default `true`, `--trace=false` disables).

- [ ] **Step 4: Verify**

```bash
go build ./cmd/... ./internal/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add test and playground commands to config parser"
```

---

### Task 6: Create CLI formatters and test command handlers

**Files:**
- Create: `internal/cli/test.go`
- Create: `internal/cli/format.go`

- [ ] **Step 1: Write the format helpers**

`internal/cli/format.go`:
```go
package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func FormatToolTable(tools []*mcp.Tool) string {
	var sb strings.Builder
	sb.WriteString("  Tools\n")
	sb.WriteString("  " + strings.Repeat("─", 64) + "\n")
	for _, t := range tools {
		params := formatParams(t.InputSchema)
		sb.WriteString(fmt.Sprintf("  %-18s %-30s %s\n", t.Name, t.Description, params))
	}
	return sb.String()
}

func FormatResourceTable(resources []*mcp.Resource) string {
	if len(resources) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n  Resources\n")
	sb.WriteString("  " + strings.Repeat("─", 64) + "\n")
	for _, r := range resources {
		sb.WriteString(fmt.Sprintf("  %-18s %-30s %s\n", r.URI, r.Description, r.MIMEType))
	}
	return sb.String()
}

func FormatPromptTable(prompts []*mcp.Prompt) string {
	if len(prompts) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n  Prompts\n")
	sb.WriteString("  " + strings.Repeat("─", 64) + "\n")
	for _, p := range prompts {
		args := formatPromptArgs(p.Arguments)
		sb.WriteString(fmt.Sprintf("  %-18s %-30s %s\n", p.Name, p.Description, args))
	}
	return sb.String()
}

func formatParams(schema any) string {
	if schema == nil {
		return ""
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	var s struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if json.Unmarshal(data, &s) != nil {
		return ""
	}
	reqSet := make(map[string]bool)
	for _, r := range s.Required {
		reqSet[r] = true
	}
	var parts []string
	for name, prop := range s.Properties {
		req := "optional"
		if reqSet[name] {
			req = "required"
		}
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", name, prop.Type, req))
	}
	return strings.Join(parts, ", ")
}

func formatPromptArgs(args []*mcp.PromptArgument) string {
	var parts []string
	for _, a := range args {
		suffix := ""
		if a.Required {
			suffix = " (required)"
		}
		parts = append(parts, a.Name+suffix)
	}
	return strings.Join(parts, ", ")
}
```

- [ ] **Step 2: Write the test command handlers**

`internal/cli/test.go`:
```go
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/msilverblatt/protomcp/internal/testengine"
)

func RunTestList(ctx context.Context, file, format string) error {
	e := testengine.New(file)
	if err := e.Start(ctx); err != nil {
		return err
	}
	defer e.Stop()

	tools, _ := e.ListTools(ctx)
	resources, _ := e.ListResources(ctx)
	prompts, _ := e.ListPrompts(ctx)

	if format == "json" {
		out := map[string]any{
			"tools":     tools,
			"resources": resources,
			"prompts":   prompts,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(FormatToolTable(tools))
	fmt.Print(FormatResourceTable(resources))
	fmt.Print(FormatPromptTable(prompts))
	fmt.Printf("\n  %d tools, %d resources, %d prompts\n", len(tools), len(resources), len(prompts))
	return nil
}

func RunTestCall(ctx context.Context, file, toolName, argsJSON, format string, showTrace bool) error {
	e := testengine.New(file)
	if err := e.Start(ctx); err != nil {
		return err
	}
	defer e.Stop()

	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Errorf("invalid --args JSON: %w", err)
		}
	}

	result, err := e.CallTool(ctx, toolName, args)
	if err != nil {
		return fmt.Errorf("call %s: %w", toolName, err)
	}

	if format == "json" {
		out := map[string]any{
			"result":         result.Result,
			"duration_ms":    result.Duration.Milliseconds(),
			"tools_enabled":  result.ToolsEnabled,
			"tools_disabled": result.ToolsDisabled,
			"trace":          e.Trace().Entries(),
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
	fmt.Printf("\n  Result (%dms)\n", result.Duration.Milliseconds())
	fmt.Println("  " + "────────────────────────────────────────────────────────────────")
	for _, c := range result.Result.Content {
		data, _ := json.Marshal(c)
		fmt.Printf("  %s\n", data)
	}

	if showTrace {
		fmt.Println("\n  Protocol Trace")
		fmt.Println("  " + "────────────────────────────────────────────────────────────────")
		entries := e.Trace().Entries()
		if len(entries) > 0 {
			base := entries[0].Timestamp
			for _, entry := range entries {
				elapsed := entry.Timestamp.Sub(base)
				arrow := "→"
				if entry.Direction == "recv" {
					arrow = "←"
				}
				summary := entry.Method
				if summary == "" {
					summary = "(response)"
				}
				fmt.Printf("  %-7s %s %s\n", fmt.Sprintf("%dms", elapsed.Milliseconds()), arrow, summary)
			}
		}
	}

	if len(result.ToolsEnabled) > 0 || len(result.ToolsDisabled) > 0 {
		fmt.Println("\n  Tool list changes")
		fmt.Println("  " + "────────────────────────────────────────────────────────────────")
		for _, name := range result.ToolsEnabled {
			fmt.Printf("  + %-18s (enabled)\n", name)
		}
		for _, name := range result.ToolsDisabled {
			fmt.Printf("  - %-18s (disabled)\n", name)
		}
	} else {
		fmt.Println("\n  Tool list: no changes")
	}

	return nil
}
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/cli/
```

- [ ] **Step 4: Commit**

```bash
git add internal/cli/
git commit -m "feat: add pmcp test CLI — list and call commands"
```

---

### Task 7: Wire test and playground commands in main.go

**Files:**
- Modify: `cmd/protomcp/main.go`

- [ ] **Step 1: Add command dispatch**

In main.go, after the existing `cfg.Command == "validate"` check, add:

```go
if cfg.Command == "test" {
    runTest(ctx, cfg)
    return
}

if cfg.Command == "playground" {
    runPlayground(ctx, cfg)
    return
}
```

Add the `runTest` function:
```go
func runTest(ctx context.Context, cfg *config.Config) {
    var err error
    switch cfg.TestSubcommand {
    case "list":
        err = cli.RunTestList(ctx, cfg.File, cfg.Format)
    case "call":
        err = cli.RunTestCall(ctx, cfg.File, cfg.TestToolName, cfg.TestArgs, cfg.Format, cfg.ShowTrace)
    case "scenario":
        fmt.Println("Scenario runner coming in a future release.")
        return
    default:
        fmt.Fprintf(os.Stderr, "unknown test subcommand: %s\n", cfg.TestSubcommand)
        os.Exit(1)
    }
    if err != nil {
        slog.Error("test failed", "error", err)
        os.Exit(1)
    }
}
```

Add a placeholder `runPlayground`:
```go
func runPlayground(ctx context.Context, cfg *config.Config) {
    fmt.Println("Playground coming soon. Use 'pmcp test' for now.")
}
```

- [ ] **Step 2: Build and test manually**

```bash
go build -o bin/pmcp ./cmd/protomcp/
bin/pmcp test test/e2e/fixtures/simple_tool.py list
bin/pmcp test test/e2e/fixtures/simple_tool.py call echo --args '{"message":"hello"}'
```

- [ ] **Step 3: Commit**

```bash
git add cmd/protomcp/ internal/config/
git commit -m "feat: wire pmcp test commands in main.go"
```

---

## Chunk 3: Playground Backend

### Task 8: Create playground HTTP server and REST handlers

**Files:**
- Create: `internal/playground/server.go`
- Create: `internal/playground/handlers.go`

- [ ] **Step 1: Write the server**

`internal/playground/server.go`:
```go
package playground

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/msilverblatt/protomcp/internal/testengine"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

// Server runs the playground HTTP server.
type Server struct {
	engine *testengine.Engine
	hub    *Hub
	logger *slog.Logger
}

// NewServer creates a playground server backed by the given engine.
func NewServer(engine *testengine.Engine, logger *slog.Logger) *Server {
	return &Server{
		engine: engine,
		hub:    NewHub(),
		logger: logger,
	}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/tools", s.handleListTools)
	mux.HandleFunc("GET /api/resources", s.handleListResources)
	mux.HandleFunc("GET /api/prompts", s.handleListPrompts)
	mux.HandleFunc("POST /api/call", s.handleCallTool)
	mux.HandleFunc("POST /api/resource/read", s.handleReadResource)
	mux.HandleFunc("POST /api/prompt/get", s.handleGetPrompt)
	mux.HandleFunc("POST /api/reload", s.handleReload)
	mux.HandleFunc("GET /api/trace", s.handleGetTrace)
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Serve embedded frontend
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return fmt.Errorf("embed frontend: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	// Wire trace events to WebSocket hub
	traceCh := s.engine.Trace().Subscribe()
	go func() {
		for entry := range traceCh {
			s.hub.Broadcast(Event{Type: "trace", Data: entry})
		}
	}()

	// Start hub
	go s.hub.Run(ctx)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	s.logger.Info("playground started", "addr", addr)
	return srv.ListenAndServe()
}
```

- [ ] **Step 2: Write the handlers**

`internal/playground/handlers.go` — REST endpoint implementations. Each handler calls the engine, marshals the result to JSON, and writes the response. The tool call handler captures timing and tool list changes.

- [ ] **Step 3: Verify build**

```bash
go build ./internal/playground/
```

Note: This will fail until the frontend dist directory exists. Create a placeholder:
```bash
mkdir -p internal/playground/frontend/dist
echo '<!doctype html><html><body>Playground loading...</body></html>' > internal/playground/frontend/dist/index.html
```

- [ ] **Step 4: Commit**

```bash
git add internal/playground/
git commit -m "feat: add playground backend — REST API + WebSocket event stream"
```

---

### Task 9: Create WebSocket hub

**Files:**
- Create: `internal/playground/ws.go`

- [ ] **Step 1: Write the WebSocket hub**

The hub manages WebSocket connections, broadcasts events, and handles connect/disconnect. Uses `nhooyr.io/websocket`.

```go
package playground

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// Event is a typed message sent to WebSocket clients.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub manages WebSocket connections and broadcasts events.
type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]context.CancelFunc
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]context.CancelFunc)}
}

func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn, cancel := range h.clients {
		cancel()
		conn.CloseNow()
	}
}

func (h *Hub) Add(conn *websocket.Conn, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = cancel
}

func (h *Hub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.clients[conn]; ok {
		cancel()
		delete(h.clients, conn)
	}
}

func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		conn.Write(context.Background(), websocket.MessageText, data)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow any origin for local dev
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	s.hub.Add(conn, cancel)
	defer s.hub.Remove(conn)

	// Send initial connection event
	s.hub.Broadcast(Event{Type: "connection", Data: map[string]string{"status": "connected"}})

	// Keep connection alive until context cancelled
	<-ctx.Done()
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/playground/
```

- [ ] **Step 3: Commit**

```bash
git add internal/playground/ws.go
git commit -m "feat: add WebSocket hub for playground event broadcasting"
```

---

### Task 10: Wire playground command in main.go

**Files:**
- Modify: `cmd/protomcp/main.go`

- [ ] **Step 1: Replace the placeholder `runPlayground`**

```go
func runPlayground(ctx context.Context, cfg *config.Config) {
    e := testengine.New(cfg.File, testengine.WithLogger(logger))
    if err := e.Start(ctx); err != nil {
        slog.Error("failed to start engine", "error", err)
        os.Exit(1)
    }
    defer e.Stop()

    addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
    srv := playground.NewServer(e, logger)
    if err := srv.ListenAndServe(ctx, addr); err != nil && err != http.ErrServerClosed {
        slog.Error("playground error", "error", err)
        os.Exit(1)
    }
}
```

- [ ] **Step 2: Build and test**

```bash
go build -o bin/pmcp ./cmd/protomcp/
bin/pmcp playground test/e2e/fixtures/simple_tool.py --port 3000
# Open http://localhost:3000 — should see placeholder HTML
# Ctrl+C to stop
```

- [ ] **Step 3: Commit**

```bash
git add cmd/protomcp/
git commit -m "feat: wire pmcp playground command"
```

---

## Chunk 4: Playground Frontend

### Task 11: Scaffold React + Tailwind + Vite frontend

**Files:**
- Create: `internal/playground/frontend/` (full React project)

- [ ] **Step 1: Initialize the project**

```bash
cd internal/playground/frontend
npm create vite@latest . -- --template react-ts
npm install
npm install -D tailwindcss @tailwindcss/vite
```

- [ ] **Step 2: Configure Tailwind**

Add Tailwind plugin to `vite.config.ts`:
```ts
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: { outDir: 'dist' },
})
```

Add `@import "tailwindcss";` to `src/index.css`.

- [ ] **Step 3: Create shared types**

`src/types.ts` — TypeScript types matching the backend JSON.

- [ ] **Step 4: Create hooks**

`src/hooks/useWebSocket.ts` — WebSocket connection with auto-reconnect.
`src/hooks/useApi.ts` — REST API wrapper with loading states.

- [ ] **Step 5: Create components**

Build each component from the spec: TopBar, FeaturePicker, ToolForm, ResourceForm, PromptForm, ResultView, TracePanel, TraceEntry, ProgressBar, History.

- [ ] **Step 6: Assemble in App.tsx**

Two-panel layout with top bar. Wire everything together.

- [ ] **Step 7: Build and verify**

```bash
cd internal/playground/frontend && npm run build
cd ../../.. && go build -o bin/pmcp ./cmd/protomcp/
bin/pmcp playground test/e2e/fixtures/simple_tool.py --port 3000
# Open http://localhost:3000 — should see the playground UI
```

- [ ] **Step 8: Commit**

```bash
git add internal/playground/frontend/
git commit -m "feat: add playground frontend — React + Tailwind interactive UI"
```

---

### Task 12: Add Makefile targets and update docs

**Files:**
- Modify: `Makefile`
- Modify: `README.md`

- [ ] **Step 1: Add Makefile targets**

```makefile
playground-frontend:
	cd internal/playground/frontend && npm run build

build: playground-frontend
	go build -o bin/pmcp ./cmd/protomcp
```

- [ ] **Step 2: Update README**

Add a "Testing & Playground" section to README.md showing `pmcp test list`, `pmcp test call`, and `pmcp playground` with a screenshot placeholder.

- [ ] **Step 3: Commit**

```bash
git add Makefile README.md
git commit -m "docs: add testing and playground to README and Makefile"
```
