package comparison_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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

// ---------------------------------------------------------------------------
// stdioMCPProcess: generic wrapper for any MCP server over stdio
// ---------------------------------------------------------------------------

type stdioMCPProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	nextID  int
	pid     int
}

func startStdioProcess(t *testing.T, name string, args ...string) *stdioMCPProcess {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Stderr = io.Discard

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	})

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 32*1024*1024), 32*1024*1024)

	return &stdioMCPProcess{
		cmd:     cmd,
		stdin:   stdin,
		scanner: scanner,
		pid:     cmd.Process.Pid,
	}
}

func (p *stdioMCPProcess) send(t *testing.T, method string, params interface{}) (mcp.JSONRPCResponse, time.Duration) {
	t.Helper()
	p.nextID++
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(fmt.Sprintf("%d", p.nextID)),
		Method:  method,
	}
	if params != nil {
		raw, _ := json.Marshal(params)
		req.Params = raw
	}
	data, _ := json.Marshal(req)

	start := time.Now()
	if _, err := p.stdin.Write(append(data, '\n')); err != nil {
		t.Fatalf("write to process: %v", err)
	}
	if !p.scanner.Scan() {
		t.Fatalf("no response for %s (id=%d): %v", method, p.nextID, p.scanner.Err())
	}
	elapsed := time.Since(start)

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal(p.scanner.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, p.scanner.Bytes())
	}
	return resp, elapsed
}

func (p *stdioMCPProcess) sendNotification(t *testing.T, method string) {
	t.Helper()
	notif, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	})
	p.stdin.Write(append(notif, '\n'))
}

func (p *stdioMCPProcess) initialize(t *testing.T) time.Duration {
	t.Helper()
	_, elapsed := p.send(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "bench", "version": "1.0"},
	})
	p.sendNotification(t, "notifications/initialized")
	return elapsed
}

// getRSS returns the resident set size of a process in KB (macOS/Linux).
func getRSS(pid int) (int64, error) {
	out, err := exec.Command("ps", "-o", "rss=", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return 0, err
	}
	var rss int64
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &rss)
	return rss, nil
}

// ---------------------------------------------------------------------------
// latencyStats: statistics helper
// ---------------------------------------------------------------------------

type latencyStats struct {
	N      int
	Min    time.Duration
	Max    time.Duration
	Mean   time.Duration
	P25    time.Duration
	P50    time.Duration
	P75    time.Duration
	P90    time.Duration
	P95    time.Duration
	P99    time.Duration
	StdDev time.Duration
	RPS    float64
}

func computeStats(latencies []time.Duration) latencyStats {
	n := len(latencies)
	if n == 0 {
		return latencyStats{}
	}
	sorted := make([]time.Duration, n)
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var total time.Duration
	for _, l := range sorted {
		total += l
	}
	mean := total / time.Duration(n)

	// Standard deviation
	var sumSq float64
	for _, l := range sorted {
		diff := float64(l - mean)
		sumSq += diff * diff
	}
	stddev := time.Duration(math.Sqrt(sumSq / float64(n)))

	return latencyStats{
		N:      n,
		Min:    sorted[0],
		Max:    sorted[n-1],
		Mean:   mean,
		P25:    sorted[n*25/100],
		P50:    sorted[n*50/100],
		P75:    sorted[n*75/100],
		P90:    sorted[n*90/100],
		P95:    sorted[n*95/100],
		P99:    sorted[n*99/100],
		StdDev: stddev,
		RPS:    float64(n) / total.Seconds(),
	}
}

func logStats(t *testing.T, label string, s latencyStats) {
	t.Logf("  %s:", label)
	t.Logf("    n=%d  min=%v  p25=%v  p50=%v  p75=%v  p90=%v  p95=%v  p99=%v  max=%v",
		s.N, s.Min, s.P25, s.P50, s.P75, s.P90, s.P95, s.P99, s.Max)
	t.Logf("    mean=%v  stddev=%v  rps=%.0f", s.Mean, s.StdDev, s.RPS)
}

func logComparison(t *testing.T, metric string, pmcp, fast time.Duration) {
	if pmcp < fast {
		t.Logf("    %-20s protomcp %v vs FastMCP %v  (protomcp %.1fx faster)", metric, pmcp, fast, float64(fast)/float64(pmcp))
	} else if fast < pmcp {
		t.Logf("    %-20s protomcp %v vs FastMCP %v  (FastMCP %.1fx faster)", metric, pmcp, fast, float64(pmcp)/float64(fast))
	} else {
		t.Logf("    %-20s protomcp %v vs FastMCP %v  (equal)", metric, pmcp, fast)
	}
}

// ---------------------------------------------------------------------------
// TestComparisonProtomcpVsDirect: protomcp overhead measurement
// ---------------------------------------------------------------------------

func TestComparisonProtomcpVsDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping comparison test in short mode")
	}

	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("comparison-%d.sock", os.Getpid()))

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

	for i := 0; i < 50; i++ {
		pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
	}

	const n = 500
	latencies := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		resp, err := pm.CallTool(ctx, "echo", `{"message":"compare"}`)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if resp.IsError {
			t.Errorf("unexpected error")
		}
		latencies = append(latencies, elapsed)
	}

	s := computeStats(latencies)
	t.Logf("protomcp process.Manager overhead (n=%d):", n)
	logStats(t, "process.Manager → protobuf/unix socket → Python", s)

	runtime.GC()
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	t.Logf("  Go heap: %dKB", memStats.Alloc/1024)
}

// ---------------------------------------------------------------------------
// TestDetailedComparison: comprehensive head-to-head
// ---------------------------------------------------------------------------

func TestDetailedComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping comparison test in short mode")
	}

	// Check FastMCP
	checkCmd := exec.Command("python3", "-c", "import fastmcp; print(fastmcp.__version__)")
	versionOut, err := checkCmd.Output()
	if err != nil {
		t.Skip("FastMCP not installed, skipping comparison")
	}
	fastmcpVersion := strings.TrimSpace(string(versionOut))

	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║          PROTOMCP vs FASTMCP — DETAILED COMPARISON         ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")
	t.Logf("")
	t.Logf("FastMCP version: %s", fastmcpVersion)
	t.Logf("Go version:      %s", runtime.Version())
	t.Logf("OS/Arch:         %s/%s", runtime.GOOS, runtime.GOARCH)
	t.Logf("")

	// ---------------------------------------------------------------
	// 1. COLD STARTUP TIME
	// ---------------------------------------------------------------
	t.Run("1_ColdStartup", func(t *testing.T) {
		const trials = 5

		t.Log("────────────────────────────────────────")
		t.Log("1. COLD STARTUP TIME")
		t.Log("   Time from process spawn to first successful tool call")
		t.Log("────────────────────────────────────────")

		// protomcp cold starts
		pmcpStartups := make([]time.Duration, 0, trials)
		for i := 0; i < trials; i++ {
			fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
			socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("startup-cmp-%d-%d.sock", os.Getpid(), i))

			start := time.Now()

			pm := process.NewManager(process.ManagerConfig{
				File:        fixture,
				RuntimeCmd:  "python3",
				RuntimeArgs: []string{fixture},
				SocketPath:  socketPath,
				MaxRetries:  1,
				CallTimeout: 30 * time.Second,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err := pm.Start(ctx)
			if err != nil {
				cancel()
				t.Fatalf("protomcp start failed: %v", err)
			}
			resp, err := pm.CallTool(ctx, "echo", `{"message":"startup"}`)
			elapsed := time.Since(start)
			if err != nil || resp.IsError {
				pm.Stop()
				cancel()
				t.Fatalf("protomcp first call failed")
			}
			pmcpStartups = append(pmcpStartups, elapsed)
			pm.Stop()
			cancel()
		}

		// FastMCP cold starts
		fastStartups := make([]time.Duration, 0, trials)
		for i := 0; i < trials; i++ {
			fixture := testutil.FixturePath("tests/bench/comparison/fastmcp_echo.py")
			start := time.Now()

			p := startStdioProcess(t, "python3", fixture)
			p.initialize(t)
			resp, _ := p.send(t, "tools/call", map[string]interface{}{
				"name":      "echo",
				"arguments": map[string]string{"message": "startup"},
			})
			elapsed := time.Since(start)
			if resp.Error != nil {
				t.Fatalf("FastMCP first call failed: %s", resp.Error.Message)
			}
			fastStartups = append(fastStartups, elapsed)
			// cleanup happens via t.Cleanup
		}

		pmcpS := computeStats(pmcpStartups)
		fastS := computeStats(fastStartups)
		logStats(t, "protomcp", pmcpS)
		logStats(t, "FastMCP", fastS)
		t.Logf("")
		logComparison(t, "startup p50", pmcpS.P50, fastS.P50)
	})

	// ---------------------------------------------------------------
	// Spawn persistent processes for remaining tests
	// ---------------------------------------------------------------
	pmcpFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	pmcpProc := testutil.StartPMCP(t, "dev", pmcpFixture)
	pmcpProc.Initialize(t)

	fastFixture := testutil.FixturePath("tests/bench/comparison/fastmcp_echo.py")
	fastProc := startStdioProcess(t, "python3", fastFixture)
	fastProc.initialize(t)

	// Warm up both
	for i := 0; i < 100; i++ {
		pmcpProc.Send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "warmup"},
		})
		fastProc.send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "warmup"},
		})
	}

	// ---------------------------------------------------------------
	// 2. TOOLS/LIST LATENCY
	// ---------------------------------------------------------------
	t.Run("2_ToolsList", func(t *testing.T) {
		const n = 500

		t.Log("────────────────────────────────────────")
		t.Log("2. TOOLS/LIST LATENCY")
		t.Log("   Time to list all registered tools")
		t.Log("────────────────────────────────────────")

		pmcpLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			r := pmcpProc.Send(t, "tools/list", nil)
			elapsed := time.Since(start)
			if r.Resp.Error != nil {
				t.Fatalf("protomcp tools/list failed")
			}
			pmcpLats = append(pmcpLats, elapsed)
		}

		fastLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			_, elapsed := fastProc.send(t, "tools/list", nil)
			fastLats = append(fastLats, elapsed)
		}

		pmcpS := computeStats(pmcpLats)
		fastS := computeStats(fastLats)
		logStats(t, "protomcp", pmcpS)
		logStats(t, "FastMCP", fastS)
		t.Logf("")
		logComparison(t, "tools/list p50", pmcpS.P50, fastS.P50)
	})

	// ---------------------------------------------------------------
	// 3. ECHO LATENCY (baseline tool call)
	// ---------------------------------------------------------------
	t.Run("3_EchoLatency", func(t *testing.T) {
		const n = 1000

		t.Log("────────────────────────────────────────")
		t.Log("3. ECHO LATENCY (n=1000)")
		t.Log("   Simple echo tool — measures framework overhead")
		t.Log("────────────────────────────────────────")

		pmcpLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": fmt.Sprintf("echo-%d", i)},
			})
			elapsed := time.Since(start)
			if r.Resp.Error != nil {
				t.Fatalf("protomcp echo %d failed: %s", i, r.Resp.Error.Message)
			}
			pmcpLats = append(pmcpLats, elapsed)
		}

		fastLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": fmt.Sprintf("echo-%d", i)},
			})
			if resp.Error != nil {
				t.Fatalf("FastMCP echo %d failed: %s", i, resp.Error.Message)
			}
			fastLats = append(fastLats, elapsed)
		}

		pmcpS := computeStats(pmcpLats)
		fastS := computeStats(fastLats)
		logStats(t, "protomcp", pmcpS)
		logStats(t, "FastMCP", fastS)
		t.Logf("")
		logComparison(t, "echo p50", pmcpS.P50, fastS.P50)
		logComparison(t, "echo p99", pmcpS.P99, fastS.P99)
		logComparison(t, "echo mean", pmcpS.Mean, fastS.Mean)
	})

	// ---------------------------------------------------------------
	// 4. PAYLOAD SIZE SCALING
	// ---------------------------------------------------------------
	t.Run("4_PayloadSizes", func(t *testing.T) {
		sizes := []struct {
			label string
			bytes int
		}{
			{"10B", 10},
			{"100B", 100},
			{"1KB", 1024},
			{"10KB", 10 * 1024},
			{"50KB", 50 * 1024},
			{"100KB", 100 * 1024},
		}

		t.Log("────────────────────────────────────────")
		t.Log("4. PAYLOAD SIZE SCALING")
		t.Log("   Echo with increasing message sizes")
		t.Log("────────────────────────────────────────")

		const n = 200

		for _, sz := range sizes {
			payload := strings.Repeat("X", sz.bytes)

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": payload},
				})
				elapsed := time.Since(start)
				if r.Resp.Error != nil {
					t.Fatalf("protomcp echo %s failed", sz.label)
				}
				pmcpLats = append(pmcpLats, elapsed)
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": payload},
				})
				if resp.Error != nil {
					t.Fatalf("FastMCP echo %s failed", sz.label)
				}
				fastLats = append(fastLats, elapsed)
			}

			pmcpS := computeStats(pmcpLats)
			fastS := computeStats(fastLats)

			t.Logf("")
			t.Logf("  Payload %s:", sz.label)
			t.Logf("    protomcp  p50=%-12v p99=%-12v mean=%-12v rps=%.0f", pmcpS.P50, pmcpS.P99, pmcpS.Mean, pmcpS.RPS)
			t.Logf("    FastMCP   p50=%-12v p99=%-12v mean=%-12v rps=%.0f", fastS.P50, fastS.P99, fastS.Mean, fastS.RPS)
			if pmcpS.P50 < fastS.P50 {
				t.Logf("    → protomcp %.1fx faster", float64(fastS.P50)/float64(pmcpS.P50))
			} else {
				t.Logf("    → FastMCP %.1fx faster", float64(pmcpS.P50)/float64(fastS.P50))
			}
		}
	})

	// ---------------------------------------------------------------
	// 5. RESPONSE SIZE SCALING
	// ---------------------------------------------------------------
	t.Run("5_ResponseSizes", func(t *testing.T) {
		sizes := []struct {
			label string
			bytes int
		}{
			{"10B", 10},
			{"100B", 100},
			{"1KB", 1024},
			{"10KB", 10 * 1024},
			{"50KB", 50 * 1024},
		}

		t.Log("────────────────────────────────────────")
		t.Log("5. RESPONSE SIZE SCALING")
		t.Log("   Generate tool returns N bytes")
		t.Log("────────────────────────────────────────")

		const n = 200

		for _, sz := range sizes {
			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "generate", "arguments": map[string]int{"size": sz.bytes},
				})
				elapsed := time.Since(start)
				if r.Resp.Error != nil {
					t.Fatalf("protomcp generate %s failed", sz.label)
				}
				pmcpLats = append(pmcpLats, elapsed)
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "generate", "arguments": map[string]int{"size": sz.bytes},
				})
				if resp.Error != nil {
					t.Fatalf("FastMCP generate %s failed", sz.label)
				}
				fastLats = append(fastLats, elapsed)
			}

			pmcpS := computeStats(pmcpLats)
			fastS := computeStats(fastLats)

			t.Logf("")
			t.Logf("  Response %s:", sz.label)
			t.Logf("    protomcp  p50=%-12v p99=%-12v mean=%-12v rps=%.0f", pmcpS.P50, pmcpS.P99, pmcpS.Mean, pmcpS.RPS)
			t.Logf("    FastMCP   p50=%-12v p99=%-12v mean=%-12v rps=%.0f", fastS.P50, fastS.P99, fastS.Mean, fastS.RPS)
			if pmcpS.P50 < fastS.P50 {
				t.Logf("    → protomcp %.1fx faster", float64(fastS.P50)/float64(pmcpS.P50))
			} else {
				t.Logf("    → FastMCP %.1fx faster", float64(pmcpS.P50)/float64(fastS.P50))
			}
		}
	})

	// ---------------------------------------------------------------
	// 6. CPU-BOUND TOOL (computation overhead)
	// ---------------------------------------------------------------
	t.Run("6_CPUBound", func(t *testing.T) {
		iterCounts := []int{1, 10, 100, 1000}

		t.Log("────────────────────────────────────────")
		t.Log("6. CPU-BOUND TOOL")
		t.Log("   SHA-256 hash N iterations — isolates framework vs tool work")
		t.Log("────────────────────────────────────────")

		const n = 100

		for _, iters := range iterCounts {
			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "compute", "arguments": map[string]int{"iterations": iters},
				})
				elapsed := time.Since(start)
				if r.Resp.Error != nil {
					t.Fatalf("protomcp compute %d failed", iters)
				}
				pmcpLats = append(pmcpLats, elapsed)
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "compute", "arguments": map[string]int{"iterations": iters},
				})
				if resp.Error != nil {
					t.Fatalf("FastMCP compute %d failed", iters)
				}
				fastLats = append(fastLats, elapsed)
			}

			pmcpS := computeStats(pmcpLats)
			fastS := computeStats(fastLats)

			t.Logf("")
			t.Logf("  %d iterations:", iters)
			t.Logf("    protomcp  p50=%-12v p99=%-12v mean=%-12v", pmcpS.P50, pmcpS.P99, pmcpS.Mean)
			t.Logf("    FastMCP   p50=%-12v p99=%-12v mean=%-12v", fastS.P50, fastS.P99, fastS.Mean)

			// Calculate framework overhead: total - (estimated tool work)
			// Since both run the same Python code, the difference is framework overhead
			if pmcpS.P50 < fastS.P50 {
				overhead := fastS.P50 - pmcpS.P50
				t.Logf("    → protomcp %.1fx faster (FastMCP adds %v framework overhead)", float64(fastS.P50)/float64(pmcpS.P50), overhead)
			} else {
				overhead := pmcpS.P50 - fastS.P50
				t.Logf("    → FastMCP %.1fx faster (protomcp adds %v framework overhead)", float64(pmcpS.P50)/float64(fastS.P50), overhead)
			}
		}
	})

	// ---------------------------------------------------------------
	// 7. JSON SERIALIZATION OVERHEAD
	// ---------------------------------------------------------------
	t.Run("7_JSONSerialization", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("7. JSON SERIALIZATION OVERHEAD")
		t.Log("   Parse and re-serialize JSON of varying complexity")
		t.Log("────────────────────────────────────────")

		payloads := []struct {
			label string
			data  interface{}
		}{
			{"simple", map[string]string{"key": "value"}},
			{"nested", map[string]interface{}{
				"users": []map[string]interface{}{
					{"name": "Alice", "age": 30, "tags": []string{"admin", "active"}},
					{"name": "Bob", "age": 25, "tags": []string{"user"}},
				},
				"meta": map[string]interface{}{"page": 1, "total": 100},
			}},
			{"array_100", func() interface{} {
				items := make([]map[string]interface{}, 100)
				for i := range items {
					items[i] = map[string]interface{}{
						"id": i, "name": fmt.Sprintf("item-%d", i), "value": float64(i) * 1.5,
					}
				}
				return items
			}()},
		}

		const n = 200

		for _, pl := range payloads {
			jsonData, _ := json.Marshal(pl.data)
			dataStr := string(jsonData)

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "parse_json", "arguments": map[string]string{"data": dataStr},
				})
				elapsed := time.Since(start)
				if r.Resp.Error != nil {
					t.Fatalf("protomcp parse_json %s failed", pl.label)
				}
				pmcpLats = append(pmcpLats, elapsed)
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "parse_json", "arguments": map[string]string{"data": dataStr},
				})
				if resp.Error != nil {
					t.Fatalf("FastMCP parse_json %s failed", pl.label)
				}
				fastLats = append(fastLats, elapsed)
			}

			pmcpS := computeStats(pmcpLats)
			fastS := computeStats(fastLats)

			t.Logf("")
			t.Logf("  %s (%d bytes):", pl.label, len(jsonData))
			t.Logf("    protomcp  p50=%-12v p99=%-12v mean=%-12v", pmcpS.P50, pmcpS.P99, pmcpS.Mean)
			t.Logf("    FastMCP   p50=%-12v p99=%-12v mean=%-12v", fastS.P50, fastS.P99, fastS.Mean)
			if pmcpS.P50 < fastS.P50 {
				t.Logf("    → protomcp %.1fx faster", float64(fastS.P50)/float64(pmcpS.P50))
			} else {
				t.Logf("    → FastMCP %.1fx faster", float64(pmcpS.P50)/float64(fastS.P50))
			}
		}
	})

	// ---------------------------------------------------------------
	// 8. ERROR HANDLING LATENCY
	// ---------------------------------------------------------------
	t.Run("8_ErrorHandling", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("8. ERROR HANDLING LATENCY")
		t.Log("   Call nonexistent tool — measures error path overhead")
		t.Log("────────────────────────────────────────")

		const n = 200

		pmcpLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "nonexistent_tool_xyz", "arguments": map[string]string{},
			})
			elapsed := time.Since(start)
			pmcpLats = append(pmcpLats, elapsed)
		}

		fastLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "nonexistent_tool_xyz", "arguments": map[string]string{},
			})
			elapsed := time.Since(start)
			fastLats = append(fastLats, elapsed)
		}

		pmcpS := computeStats(pmcpLats)
		fastS := computeStats(fastLats)
		logStats(t, "protomcp", pmcpS)
		logStats(t, "FastMCP", fastS)
		t.Logf("")
		logComparison(t, "error p50", pmcpS.P50, fastS.P50)
	})

	// ---------------------------------------------------------------
	// 9. SUSTAINED LOAD (latency stability)
	// ---------------------------------------------------------------
	t.Run("9_SustainedLoad", func(t *testing.T) {
		const totalCalls = 2000
		const windowSize = 200

		t.Log("────────────────────────────────────────")
		t.Logf("9. SUSTAINED LOAD (%d calls, p50 per %d-call window)", totalCalls, windowSize)
		t.Log("   Checks for latency drift / GC pauses under load")
		t.Log("────────────────────────────────────────")

		pmcpAllLats := make([]time.Duration, 0, totalCalls)
		for i := 0; i < totalCalls; i++ {
			start := time.Now()
			r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": fmt.Sprintf("sustained-%d", i)},
			})
			elapsed := time.Since(start)
			if r.Resp.Error != nil {
				t.Fatalf("protomcp sustained call %d failed", i)
			}
			pmcpAllLats = append(pmcpAllLats, elapsed)
		}

		fastAllLats := make([]time.Duration, 0, totalCalls)
		for i := 0; i < totalCalls; i++ {
			resp, elapsed := fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": fmt.Sprintf("sustained-%d", i)},
			})
			if resp.Error != nil {
				t.Fatalf("FastMCP sustained call %d failed", i)
			}
			fastAllLats = append(fastAllLats, elapsed)
		}

		t.Logf("")
		t.Logf("  %-10s  %-16s %-16s %-16s %-16s", "Window", "protomcp p50", "protomcp p99", "FastMCP p50", "FastMCP p99")
		t.Logf("  %-10s  %-16s %-16s %-16s %-16s", "──────", "────────────", "────────────", "───────────", "───────────")

		for start := 0; start < totalCalls; start += windowSize {
			end := start + windowSize
			if end > totalCalls {
				end = totalCalls
			}
			pmcpWindow := computeStats(pmcpAllLats[start:end])
			fastWindow := computeStats(fastAllLats[start:end])

			t.Logf("  %4d-%4d   %-16v %-16v %-16v %-16v",
				start, end,
				pmcpWindow.P50, pmcpWindow.P99,
				fastWindow.P50, fastWindow.P99,
			)
		}

		pmcpTotal := computeStats(pmcpAllLats)
		fastTotal := computeStats(fastAllLats)
		t.Logf("")
		t.Logf("  Overall:")
		logStats(t, "protomcp", pmcpTotal)
		logStats(t, "FastMCP", fastTotal)

		// Check for latency drift: compare first window p50 to last window p50
		pmcpFirst := computeStats(pmcpAllLats[:windowSize])
		pmcpLast := computeStats(pmcpAllLats[totalCalls-windowSize:])
		fastFirst := computeStats(fastAllLats[:windowSize])
		fastLast := computeStats(fastAllLats[totalCalls-windowSize:])

		t.Logf("")
		t.Logf("  Latency drift (first window → last window p50):")
		t.Logf("    protomcp: %v → %v (%.1f%% change)", pmcpFirst.P50, pmcpLast.P50,
			100*(float64(pmcpLast.P50)-float64(pmcpFirst.P50))/float64(pmcpFirst.P50))
		t.Logf("    FastMCP:  %v → %v (%.1f%% change)", fastFirst.P50, fastLast.P50,
			100*(float64(fastLast.P50)-float64(fastFirst.P50))/float64(fastFirst.P50))
	})

	// ---------------------------------------------------------------
	// 10. MEMORY USAGE
	// ---------------------------------------------------------------
	t.Run("10_Memory", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("10. MEMORY USAGE")
		t.Log("    RSS of each server process (after sustained load)")
		t.Log("────────────────────────────────────────")

		// protomcp: the pmcp binary RSS
		pmcpRSS, err := getRSS(pmcpProc.Cmd.Process.Pid)
		if err != nil {
			t.Logf("  Could not read protomcp RSS: %v", err)
		}

		// FastMCP: the python3 process RSS
		fastRSS, err := getRSS(fastProc.pid)
		if err != nil {
			t.Logf("  Could not read FastMCP RSS: %v", err)
		}

		// Go heap (protomcp orchestrator process — us)
		runtime.GC()
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		t.Logf("")
		t.Logf("  protomcp (pmcp binary) RSS: %d KB", pmcpRSS)
		t.Logf("  FastMCP  (python3)     RSS: %d KB", fastRSS)
		t.Logf("  Go test orchestrator heap:  %d KB", memStats.Alloc/1024)

		if pmcpRSS > 0 && fastRSS > 0 {
			if pmcpRSS < fastRSS {
				t.Logf("  → protomcp uses %.1fx less memory", float64(fastRSS)/float64(pmcpRSS))
			} else {
				t.Logf("  → FastMCP uses %.1fx less memory", float64(pmcpRSS)/float64(fastRSS))
			}
		}
	})

	// ---------------------------------------------------------------
	// FINAL SUMMARY TABLE
	// ---------------------------------------------------------------
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════╗")
	t.Log("║                     SUMMARY TABLE                          ║")
	t.Log("╚══════════════════════════════════════════════════════════════╝")
	t.Log("")

	// Re-run quick 500-call echo to get final numbers for summary
	const summaryN = 500
	pmcpFinal := make([]time.Duration, 0, summaryN)
	for i := 0; i < summaryN; i++ {
		start := time.Now()
		pmcpProc.Send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "final"},
		})
		pmcpFinal = append(pmcpFinal, time.Since(start))
	}
	fastFinal := make([]time.Duration, 0, summaryN)
	for i := 0; i < summaryN; i++ {
		start := time.Now()
		fastProc.send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "final"},
		})
		fastFinal = append(fastFinal, time.Since(start))
	}

	ps := computeStats(pmcpFinal)
	fs := computeStats(fastFinal)

	pmcpRSS, _ := getRSS(pmcpProc.Cmd.Process.Pid)
	fastRSS, _ := getRSS(fastProc.pid)

	t.Logf("  %-25s  %-14s  %-14s  %-10s", "Metric", "protomcp", "FastMCP", "Winner")
	t.Logf("  %-25s  %-14s  %-14s  %-10s", "─────────────────────────", "──────────────", "──────────────", "──────────")
	summaryRow(t, "Echo p50 latency", ps.P50, fs.P50)
	summaryRow(t, "Echo p95 latency", ps.P95, fs.P95)
	summaryRow(t, "Echo p99 latency", ps.P99, fs.P99)
	summaryRow(t, "Echo mean latency", ps.Mean, fs.Mean)
	t.Logf("  %-25s  %-14.0f  %-14.0f  %s", "Requests/sec", ps.RPS, fs.RPS,
		func() string {
			if ps.RPS > fs.RPS {
				return fmt.Sprintf("protomcp (%.0fx)", ps.RPS/fs.RPS)
			}
			return fmt.Sprintf("FastMCP (%.0fx)", fs.RPS/ps.RPS)
		}())
	t.Logf("  %-25s  %-14s  %-14s  %s", "Server RSS",
		fmt.Sprintf("%d KB", pmcpRSS), fmt.Sprintf("%d KB", fastRSS),
		func() string {
			if pmcpRSS < fastRSS {
				return fmt.Sprintf("protomcp (%.1fx less)", float64(fastRSS)/float64(pmcpRSS))
			}
			return fmt.Sprintf("FastMCP (%.1fx less)", float64(pmcpRSS)/float64(fastRSS))
		}())
	t.Logf("  %-25s  %-14v  %-14v", "Latency stddev", ps.StdDev, fs.StdDev)
}

func summaryRow(t *testing.T, metric string, pmcp, fast time.Duration) {
	winner := ""
	if pmcp < fast {
		winner = fmt.Sprintf("protomcp (%.0fx)", float64(fast)/float64(pmcp))
	} else if fast < pmcp {
		winner = fmt.Sprintf("FastMCP (%.0fx)", float64(pmcp)/float64(fast))
	} else {
		winner = "tie"
	}
	t.Logf("  %-25s  %-14v  %-14v  %s", metric, pmcp, fast, winner)
}
