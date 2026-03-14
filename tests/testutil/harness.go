// Package testutil provides shared test utilities for protomcp tests.
package testutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// RepoRoot returns the absolute path to the repository root by walking up from
// this source file.
func RepoRoot() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// FixturePath returns the absolute path to a fixture file relative to the repo root.
func FixturePath(relPath string) string {
	return filepath.Join(RepoRoot(), relPath)
}

// SetupPythonPath configures PYTHONPATH so that the generated protobuf code is
// importable by Python fixture scripts.
func SetupPythonPath() {
	root := RepoRoot()
	pythonPath := filepath.Join(root, "sdk", "python", "src") +
		string(os.PathListSeparator) +
		filepath.Join(root, "sdk", "python", "gen")
	existing := os.Getenv("PYTHONPATH")
	if existing != "" {
		pythonPath = pythonPath + string(os.PathListSeparator) + existing
	}
	os.Setenv("PYTHONPATH", pythonPath)
}

// JSONRPCRequest is a minimal JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a minimal JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// MCP response types for test assertions.

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type ToolsCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// PMCPResult holds a parsed JSON-RPC response plus the raw bytes and the round
// trip latency.
type PMCPResult struct {
	Resp    JSONRPCResponse
	Raw     []byte
	Latency time.Duration
}

// StdioPMCP wraps a running pmcp process and provides helpers for sending
// JSON-RPC requests over its stdio transport.
type StdioPMCP struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Reader *bufio.Scanner
	mu     sync.Mutex
	nextID int
}

// StartPMCP builds (if needed) and starts the pmcp binary with the given args.
func StartPMCP(tb testing.TB, args ...string) *StdioPMCP {
	tb.Helper()
	binPath := filepath.Join(RepoRoot(), "bin", "pmcp")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		tb.Fatalf("pmcp binary not found at %s; run 'make build' first", binPath)
	}

	cmd := exec.Command(binPath, args...)
	cmd.Stderr = nil

	stdin, err := cmd.StdinPipe()
	if err != nil {
		tb.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		tb.Fatalf("StdoutPipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		tb.Fatalf("start pmcp: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 512*1024*1024), 512*1024*1024)

	tb.Cleanup(func() {
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	})

	return &StdioPMCP{
		Cmd:    cmd,
		Stdin:  stdin,
		Reader: scanner,
	}
}

// Send sends a JSON-RPC request and reads the response synchronously.
func (p *StdioPMCP) Send(tb testing.TB, method string, params interface{}) PMCPResult {
	tb.Helper()
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.mu.Unlock()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", id)),
		Method:  method,
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		req.Params = raw
	}

	data, _ := json.Marshal(req)
	start := time.Now()

	p.mu.Lock()
	_, err := p.Stdin.Write(append(data, '\n'))
	p.mu.Unlock()
	if err != nil {
		tb.Fatalf("write to pmcp: %v", err)
	}

	if !p.Reader.Scan() {
		tb.Fatalf("no response from pmcp for %s: %v", method, p.Reader.Err())
	}
	elapsed := time.Since(start)

	raw := p.Reader.Bytes()
	var resp JSONRPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		tb.Fatalf("unmarshal response: %v (raw: %s)", err, raw)
	}

	return PMCPResult{Resp: resp, Raw: bytes.Clone(raw), Latency: elapsed}
}

// SendNotification sends a JSON-RPC notification (no ID, no response).
func (p *StdioPMCP) SendNotification(tb testing.TB, method string, params interface{}) {
	tb.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, _ := json.Marshal(req)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Stdin.Write(append(data, '\n'))
}

// Initialize sends a proper MCP initialize handshake (initialize + initialized notification).
func (p *StdioPMCP) Initialize(tb testing.TB) {
	tb.Helper()
	r := p.Send(tb, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "test", "version": "1.0.0"},
	})
	if r.Resp.Error != nil {
		tb.Fatalf("initialize failed: %s", r.Resp.Error.Message)
	}
	p.SendNotification(tb, "notifications/initialized", map[string]interface{}{})
}
