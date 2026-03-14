package bench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// BenchmarkLatency_ProcessManager measures per-call latency through the
// process manager (protobuf over unix socket).
func BenchmarkLatency_ProcessManager(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("latency-pm-%d.sock", os.Getpid()))

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

	// Warm up.
	for i := 0; i < 20; i++ {
		pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
	}

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		resp, err := pm.CallTool(ctx, "echo", `{"message":"latency-test"}`)
		elapsed := time.Since(start)
		if err != nil {
			b.Fatalf("CallTool failed: %v", err)
		}
		if resp.IsError {
			b.Fatalf("unexpected error")
		}
		latencies = append(latencies, elapsed)
	}
	b.StopTimer()

	if len(latencies) > 0 {
		reportLatencyPercentiles(b, "ProcessManager", latencies)
	}
}

// BenchmarkLatency_Stdio measures per-call latency through the full stdio
// transport pipeline.
func BenchmarkLatency_Stdio(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
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

	// Warm up.
	for i := 0; i < 20; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": "warmup"},
		})
		req, _ := json.Marshal(mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
			Method:  "tools/call",
			Params:  params,
		})
		p.SendRaw(b, req)
		if !p.Reader.Scan() {
			b.Fatal("no warmup response")
		}
	}

	latencies := make([]time.Duration, 0, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		params, _ := json.Marshal(map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": "latency-test"},
		})
		req, _ := json.Marshal(mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			ID:      json.RawMessage(fmt.Sprintf("%d", i+100)),
			Method:  "tools/call",
			Params:  params,
		})

		start := time.Now()
		p.SendRaw(b, req)
		if !p.Reader.Scan() {
			b.Fatal("no response")
		}
		elapsed := time.Since(start)
		latencies = append(latencies, elapsed)
	}
	b.StopTimer()

	if len(latencies) > 0 {
		reportLatencyPercentiles(b, "Stdio", latencies)
	}
}

// TestLatencyDistribution is a test (not benchmark) that reports detailed
// latency distribution for a fixed number of calls.
func TestLatencyDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping latency distribution test in short mode")
	}

	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("latency-dist-%d.sock", os.Getpid()))

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
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	// Warm up.
	for i := 0; i < 50; i++ {
		pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
	}

	const n = 500
	latencies := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		resp, err := pm.CallTool(ctx, "echo", `{"message":"latency-dist"}`)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if resp.IsError {
			t.Errorf("call %d error", i)
		}
		latencies = append(latencies, elapsed)
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	t.Logf("Latency distribution over %d calls:", n)
	t.Logf("  min:  %v", latencies[0])
	t.Logf("  p25:  %v", latencies[n*25/100])
	t.Logf("  p50:  %v", latencies[n*50/100])
	t.Logf("  p75:  %v", latencies[n*75/100])
	t.Logf("  p90:  %v", latencies[n*90/100])
	t.Logf("  p95:  %v", latencies[n*95/100])
	t.Logf("  p99:  %v", latencies[n*99/100])
	t.Logf("  max:  %v", latencies[n-1])

	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	t.Logf("  mean: %v", total/time.Duration(n))
}

func reportLatencyPercentiles(b *testing.B, label string, latencies []time.Duration) {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	n := len(latencies)
	if n == 0 {
		return
	}
	b.Logf("%s latency (n=%d): p50=%v p95=%v p99=%v min=%v max=%v",
		label, n,
		latencies[n*50/100],
		latencies[n*95/100],
		latencies[n*99/100],
		latencies[0],
		latencies[n-1],
	)
}
