package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/msilverblatt/protomcp/tests/testutil"
)

var (
	pmcpBinary     string
	pmcpBinaryOnce sync.Once
)

func getPMCPBinary(t *testing.T) string {
	t.Helper()
	pmcpBinaryOnce.Do(func() {
		pmcpBinary = filepath.Join(testutil.RepoRoot(), "bin", "pmcp")
		// Build the binary if it doesn't exist
		cmd := exec.Command("go", "build", "-o", pmcpBinary, "./cmd/protomcp")
		cmd.Dir = testutil.RepoRoot()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed to build pmcp: %v\n%s", err, out)
		}
	})
	return pmcpBinary
}

// StartProtomcp starts the protomcp binary with the given args.
func StartProtomcp(t *testing.T, args ...string) (io.Writer, *bufio.Scanner, func()) {
	t.Helper()
	cmd := exec.Command(getPMCPBinary(t), args...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start protomcp: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	cleanup := func() {
		stdin.Close()
		cmd.Process.Kill()
		cmd.Wait()
	}

	return stdin, scanner, cleanup
}

// SendRequest sends a JSON-RPC request and reads the response.
func SendRequest(t *testing.T, w io.Writer, r *bufio.Scanner, method string, params interface{}) testutil.JSONRPCResponse {
	t.Helper()
	id := json.RawMessage(`1`)
	req := testutil.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}
	if params != nil {
		p, _ := json.Marshal(params)
		req.Params = p
	}
	data, _ := json.Marshal(req)
	w.Write(append(data, '\n'))

	if !r.Scan() {
		t.Fatalf("no response from protomcp for method %q: %v", method, r.Err())
	}

	var resp testutil.JSONRPCResponse
	if err := json.Unmarshal(r.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	return resp
}

// SendNotification sends a JSON-RPC notification (no id, no response expected).
func SendNotification(t *testing.T, w io.Writer, method string, params interface{}) {
	t.Helper()
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	data, _ := json.Marshal(req)
	w.Write(append(data, '\n'))
}

// SendRequestSkipNotifications sends a request and reads the response,
// skipping any JSON-RPC notifications (messages without an "id" field).
func SendRequestSkipNotifications(t *testing.T, w io.Writer, r *bufio.Scanner, method string, params interface{}) testutil.JSONRPCResponse {
	t.Helper()
	id := json.RawMessage(`1`)
	req := testutil.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
	}
	if params != nil {
		p, _ := json.Marshal(params)
		req.Params = p
	}
	data, _ := json.Marshal(req)
	w.Write(append(data, '\n'))

	for {
		if !r.Scan() {
			t.Fatalf("no response from protomcp for method %q: %v", method, r.Err())
		}
		line := r.Bytes()

		var check map[string]json.RawMessage
		if json.Unmarshal(line, &check) == nil {
			if _, hasID := check["id"]; !hasID {
				continue // skip notification
			}
		}

		var resp testutil.JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		return resp
	}
}

// InitializeSession sends a proper MCP initialize handshake.
func InitializeSession(t *testing.T, w io.Writer, r *bufio.Scanner) testutil.JSONRPCResponse {
	t.Helper()
	resp := SendRequest(t, w, r, "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]interface{}{"name": "e2e-test", "version": "1.0.0"},
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	SendNotification(t, w, "notifications/initialized", map[string]interface{}{})
	return resp
}
