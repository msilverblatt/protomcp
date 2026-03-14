package bench_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

func init() {
	testutil.SetupPythonPath()
}

// BenchmarkThroughput_ProcessManager benchmarks raw tool call throughput
// through the process manager (bypassing the transport layer).
func BenchmarkThroughput_ProcessManager(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("throughput-%d.sock", os.Getpid()))

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  1,
		CallTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	_, err := pm.Start(ctx)
	if err != nil {
		b.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := pm.CallTool(ctx, "echo", `{"message":"bench"}`)
		if err != nil {
			b.Fatalf("CallTool failed: %v", err)
		}
		if resp.IsError {
			b.Fatalf("unexpected error")
		}
	}
}

// BenchmarkThroughput_Stdio benchmarks throughput through the full stdio
// transport pipeline (pmcp binary).
func BenchmarkThroughput_Stdio(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	// Use testing.T wrapper for StartPMCP which requires *testing.T.
	// Benchmarks can use b as *testing.B which embeds common.
	p := testutil.StartPMCP(b, "dev", fixture)

	// Initialize.
	initReq, _ := json.Marshal(mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`0`),
		Method:  "initialize",
	})
	p.SendRaw(b, initReq)
	if !p.Reader.Scan() {
		b.Fatal("no init response")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": "bench"},
		})
		req, _ := json.Marshal(mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
			Method:  "tools/call",
			Params:  params,
		})
		p.SendRaw(b, req)
		if !p.Reader.Scan() {
			b.Fatal("no response")
		}
	}
}

// BenchmarkThroughput_HTTP benchmarks throughput through the HTTP transport.
func BenchmarkThroughput_HTTP(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	port := findFreePort(b)
	p := testutil.StartPMCP(b, "dev", fixture,
		"--transport", "http",
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
	)
	_ = p

	// Wait for HTTP server to be ready.
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTP(b, addr, 5*time.Second)

	// Initialize.
	sendHTTP(b, addr, mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`0`),
		Method:  "initialize",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": "bench"},
		})
		sendHTTP(b, addr, mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
			Method:  "tools/call",
			Params:  params,
		})
	}
}

// BenchmarkPayloadSizes benchmarks different payload sizes through the process
// manager to measure serialization/deserialization overhead.
func BenchmarkPayloadSizes(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100B", 100},
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("payload-%d.sock", os.Getpid()))

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  1,
		CallTimeout: 30 * time.Second,
	})

	ctx := context.Background()
	_, err := pm.Start(ctx)
	if err != nil {
		b.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	for _, sz := range sizes {
		payload := strings.Repeat("X", sz.size)
		argsJSON := fmt.Sprintf(`{"message":"%s"}`, payload)

		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(int64(sz.size))
			for i := 0; i < b.N; i++ {
				resp, err := pm.CallTool(ctx, "echo", argsJSON)
				if err != nil {
					b.Fatalf("CallTool failed: %v", err)
				}
				if resp.IsError {
					b.Fatalf("unexpected error")
				}
			}
		})
	}
}

// --- helpers ---

func findFreePort(tb testing.TB) int {
	tb.Helper()
	// Listen on :0 to get a free port from the OS, then close the listener.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("findFreePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitForHTTP(tb testing.TB, addr string, timeout time.Duration) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(addr)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	tb.Fatalf("HTTP server at %s not ready after %v", addr, timeout)
}

func sendHTTP(tb testing.TB, addr string, req mcp.JSONRPCRequest) mcp.JSONRPCResponse {
	tb.Helper()
	data, _ := json.Marshal(req)
	resp, err := http.Post(addr+"/", "application/json", bytes.NewReader(data))
	if err != nil {
		tb.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	var jsonResp mcp.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		tb.Fatalf("decode response: %v", err)
	}
	return jsonResp
}
