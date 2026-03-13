// Package testutil provides a shared test harness for spawning a tool process
// via the process.Manager, sending JSON-RPC requests through the MCP handler +
// transport, and collecting results.
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

	"github.com/msilverblatt/protomcp/internal/mcp"
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

// PMCPResult holds a parsed JSON-RPC response plus the raw bytes and the round
// trip latency.
type PMCPResult struct {
	Resp    mcp.JSONRPCResponse
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
// The binary is expected at <repo>/bin/pmcp; callers should run `make build`
// before running these tests.
func StartPMCP(tb testing.TB, args ...string) *StdioPMCP {
	tb.Helper()
	binPath := filepath.Join(RepoRoot(), "bin", "pmcp")
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		tb.Fatalf("pmcp binary not found at %s; run 'make build' first", binPath)
	}

	cmd := exec.Command(binPath, args...)
	cmd.Stderr = nil // suppress stderr in tests

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
	scanner.Buffer(make([]byte, 32*1024*1024), 32*1024*1024)

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

	req := mcp.JSONRPCRequest{
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
	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		tb.Fatalf("unmarshal response: %v (raw: %s)", err, raw)
	}

	return PMCPResult{Resp: resp, Raw: bytes.Clone(raw), Latency: elapsed}
}

// SendRaw sends raw bytes followed by a newline to the pmcp stdin.
func (p *StdioPMCP) SendRaw(tb testing.TB, data []byte) {
	tb.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Stdin.Write(append(data, '\n'))
}

// Initialize sends the MCP initialize handshake.
func (p *StdioPMCP) Initialize(tb testing.TB) {
	tb.Helper()
	r := p.Send(tb, "initialize", nil)
	if r.Resp.Error != nil {
		tb.Fatalf("initialize failed: %s", r.Resp.Error.Message)
	}
}
