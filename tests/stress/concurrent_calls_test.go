package stress_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

func init() {
	testutil.SetupPythonPath()
}

// TestConcurrentCalls sends N concurrent tools/call requests and verifies all
// responses return correctly with no corruption or dropped messages.
func TestConcurrentCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	const concurrency = 100

	// Send all requests first, then read all responses. This hammers the proxy
	// with a pipeline of concurrent in-flight requests.
	type jsonrpcReq struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	for i := 0; i < concurrency; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": fmt.Sprintf("msg-%d", i)},
		})
		req := jsonrpcReq{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
			Method:  "tools/call",
			Params:  params,
		}
		data, _ := json.Marshal(req)
		p.SendRaw(t, data)
	}

	// Read all responses.
	latencies := make([]time.Duration, 0, concurrency)
	start := time.Now()
	for i := 0; i < concurrency; i++ {
		if !p.Reader.Scan() {
			t.Fatalf("missing response %d/%d: %v", i+1, concurrency, p.Reader.Err())
		}
		elapsed := time.Since(start)
		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(p.Reader.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response %d: %v", i, err)
		}
		if resp.Error != nil {
			t.Errorf("response %d returned error: %s", i, resp.Error.Message)
		}
		latencies = append(latencies, elapsed)
	}

	// Compute latency percentiles.
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p50 := latencies[len(latencies)*50/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	t.Logf("Concurrent calls: %d", concurrency)
	t.Logf("  p50 latency: %v", p50)
	t.Logf("  p95 latency: %v", p95)
	t.Logf("  p99 latency: %v", p99)
	t.Logf("  total time:  %v", latencies[len(latencies)-1])
}

// TestConcurrentCallsResponseIntegrity fires concurrent requests and checks
// that each response corresponds to the correct request (no cross-talk).
func TestConcurrentCallsResponseIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	const n = 50

	// Send n requests with unique messages.
	for i := 0; i < n; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": fmt.Sprintf("unique-%d", i)},
		})
		req := struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
			Method:  "tools/call",
			Params:  params,
		}
		data, _ := json.Marshal(req)
		p.SendRaw(t, data)
	}

	// Read all responses and verify ID ordering (stdio transport responds in order).
	for i := 0; i < n; i++ {
		if !p.Reader.Scan() {
			t.Fatalf("missing response %d/%d", i+1, n)
		}
		var resp mcp.JSONRPCResponse
		if err := json.Unmarshal(p.Reader.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Error != nil {
			t.Errorf("response %d error: %s", i, resp.Error.Message)
			continue
		}

		expectedID := fmt.Sprintf("%d", i+1)
		if string(resp.ID) != expectedID {
			t.Errorf("response ID mismatch: got %s, want %s", resp.ID, expectedID)
		}
	}
}

// TestBurstCalls sends multiple bursts and checks stability.
func TestBurstCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	const bursts = 10
	const perBurst = 20

	for b := 0; b < bursts; b++ {
		for i := 0; i < perBurst; i++ {
			params, _ := json.Marshal(map[string]interface{}{
				"name":      "echo",
				"arguments": map[string]string{"message": fmt.Sprintf("burst-%d-%d", b, i)},
			})
			req := struct {
				JSONRPC string          `json:"jsonrpc"`
				ID      json.RawMessage `json:"id"`
				Method  string          `json:"method"`
				Params  json.RawMessage `json:"params"`
			}{
				JSONRPC: "2.0",
				ID:      json.RawMessage(fmt.Sprintf("%d", b*perBurst+i+1)),
				Method:  "tools/call",
				Params:  params,
			}
			data, _ := json.Marshal(req)
			p.SendRaw(t, data)
		}
		// Read all responses for this burst.
		for i := 0; i < perBurst; i++ {
			if !p.Reader.Scan() {
				t.Fatalf("burst %d: missing response %d", b, i)
			}
			var resp mcp.JSONRPCResponse
			if err := json.Unmarshal(p.Reader.Bytes(), &resp); err != nil {
				t.Fatalf("burst %d: unmarshal: %v", b, err)
			}
			if resp.Error != nil {
				t.Errorf("burst %d response %d error: %s", b, i, resp.Error.Message)
			}
		}
	}

	t.Logf("Completed %d bursts of %d calls each (%d total)", bursts, perBurst, bursts*perBurst)
}
