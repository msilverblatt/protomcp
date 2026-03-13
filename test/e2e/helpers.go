package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// StartProtomcp starts the protomcp binary with the given args.
func StartProtomcp(t *testing.T, args ...string) (io.Writer, *bufio.Scanner, func()) {
	t.Helper()
	cmd := exec.Command("../../bin/pmcp", args...)
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
func SendRequest(t *testing.T, w io.Writer, r *bufio.Scanner, method string, params interface{}) mcp.JSONRPCResponse {
	t.Helper()
	id := json.RawMessage(`1`)
	req := mcp.JSONRPCRequest{
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

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(r.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	return resp
}
