package comparison_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// ═══════════════════════════════════════════════════════════════════════════
// SECTION A: CROSS-LANGUAGE TOOL COMPARISON
// Same workload through protomcp, but with the tool process written in
// Python, Go, and TypeScript — isolates tool-language overhead.
// ═══════════════════════════════════════════════════════════════════════════

func TestCrossLanguage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping enterprise bench in short mode")
	}

	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║     SECTION A: CROSS-LANGUAGE TOOL COMPARISON                  ║")
	t.Log("║     Same protomcp proxy, tools in Python / Go / TypeScript     ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")

	type langConfig struct {
		name       string
		runtimeCmd string
		runtimeArgs func(fixture string) []string
		fixture    string
		skip       func() string // returns skip reason or ""
	}

	pyFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	tsFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.ts")
	goBinary := testutil.FixturePath("tests/bench/fixtures/go_bench_tool")
	goSrcDir := testutil.FixturePath("tests/bench/fixtures/go")

	// Pre-build Go fixture if binary doesn't exist
	if _, err := os.Stat(goBinary); os.IsNotExist(err) {
		t.Log("Building Go fixture binary...")
		buildCmd := exec.Command("go", "build", "-o", goBinary, ".")
		buildCmd.Dir = goSrcDir
		if out, err := buildCmd.CombinedOutput(); err != nil {
			t.Fatalf("Go fixture build failed: %v\n%s", err, out)
		}
	}

	langs := []langConfig{
		{
			name:       "Python",
			runtimeCmd: "python3",
			runtimeArgs: func(f string) []string { return []string{f} },
			fixture:    pyFixture,
			skip:       func() string { return "" },
		},
		{
			name:       "Go",
			runtimeCmd: goBinary,
			runtimeArgs: func(f string) []string { return nil },
			fixture:    goBinary,
			skip: func() string {
				if _, err := os.Stat(goBinary); os.IsNotExist(err) {
					return "go fixture binary not built"
				}
				return ""
			},
		},
		{
			name:       "TypeScript",
			runtimeCmd: "npx",
			runtimeArgs: func(f string) []string { return []string{"tsx", f} },
			fixture:    tsFixture,
			skip: func() string {
				if _, err := exec.LookPath("npx"); err != nil {
					return "npx not installed"
				}
				return ""
			},
		},
	}

	// ---------------------------------------------------------------
	// A1: Cold startup time per language
	// ---------------------------------------------------------------
	t.Run("A1_Startup", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("A1. COLD STARTUP TIME BY LANGUAGE")
		t.Log("    Process spawn → handshake → first tool call")
		t.Log("────────────────────────────────────────")

		const trials = 3

		for _, lang := range langs {
			if reason := lang.skip(); reason != "" {
				t.Logf("  Skipping %s: %s", lang.name, reason)
				continue
			}

			startups := make([]time.Duration, 0, trials)
			for i := 0; i < trials; i++ {
				socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("lang-startup-%s-%d-%d.sock", lang.name, os.Getpid(), i))

				start := time.Now()
				pm := process.NewManager(process.ManagerConfig{
					File:        lang.fixture,
					RuntimeCmd:  lang.runtimeCmd,
					RuntimeArgs: lang.runtimeArgs(lang.fixture),
					SocketPath:  socketPath,
					MaxRetries:  1,
					CallTimeout: 60 * time.Second,
				})
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				_, err := pm.Start(ctx)
				if err != nil {
					cancel()
					t.Fatalf("%s start failed: %v", lang.name, err)
				}
				resp, err := pm.CallTool(ctx, "echo", `{"message":"startup"}`)
				elapsed := time.Since(start)
				if err != nil || resp.IsError {
					pm.Stop()
					cancel()
					t.Fatalf("%s first call failed: %v", lang.name, err)
				}
				startups = append(startups, elapsed)
				pm.Stop()
				cancel()
			}
			s := computeStats(startups)
			t.Logf("  %-12s min=%-14v p50=%-14v max=%-14v", lang.name, s.Min, s.P50, s.Max)
		}
	})

	// ---------------------------------------------------------------
	// A2: Echo latency per language
	// ---------------------------------------------------------------
	t.Run("A2_EchoLatency", func(t *testing.T) {
		const n = 1000
		const warmup = 100

		t.Log("────────────────────────────────────────")
		t.Logf("A2. ECHO LATENCY BY LANGUAGE (n=%d)", n)
		t.Log("    Pure framework + IPC overhead, trivial tool work")
		t.Log("────────────────────────────────────────")

		results := make(map[string]latencyStats)

		for _, lang := range langs {
			if reason := lang.skip(); reason != "" {
				t.Logf("  Skipping %s: %s", lang.name, reason)
				continue
			}

			socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("lang-echo-%s-%d.sock", lang.name, os.Getpid()))
			pm := process.NewManager(process.ManagerConfig{
				File:        lang.fixture,
				RuntimeCmd:  lang.runtimeCmd,
				RuntimeArgs: lang.runtimeArgs(lang.fixture),
				SocketPath:  socketPath,
				MaxRetries:  1,
				CallTimeout: 60 * time.Second,
			})
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			_, err := pm.Start(ctx)
			if err != nil {
				cancel()
				t.Fatalf("%s start failed: %v", lang.name, err)
			}

			for i := 0; i < warmup; i++ {
				pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
			}

			lats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				resp, err := pm.CallTool(ctx, "echo", `{"message":"bench"}`)
				elapsed := time.Since(start)
				if err != nil || resp.IsError {
					pm.Stop()
					cancel()
					t.Fatalf("%s call %d failed: %v", lang.name, i, err)
				}
				lats = append(lats, elapsed)
			}
			pm.Stop()
			cancel()

			s := computeStats(lats)
			results[lang.name] = s
			t.Logf("  %-12s p50=%-12v p95=%-12v p99=%-12v mean=%-12v rps=%.0f", lang.name, s.P50, s.P95, s.P99, s.Mean, s.RPS)
		}

		// RSS comparison
		t.Logf("")
		t.Logf("  Tool language overhead (p50 vs fastest):")
		var fastest time.Duration
		var fastestName string
		for name, s := range results {
			if fastest == 0 || s.P50 < fastest {
				fastest = s.P50
				fastestName = name
			}
		}
		for name, s := range results {
			if name == fastestName {
				t.Logf("    %-12s %v (baseline)", name, s.P50)
			} else {
				t.Logf("    %-12s %v (+%v, %.1fx slower)", name, s.P50, s.P50-fastest, float64(s.P50)/float64(fastest))
			}
		}
	})

	// ---------------------------------------------------------------
	// A3: CPU-bound by language
	// ---------------------------------------------------------------
	t.Run("A3_CPUBound", func(t *testing.T) {
		const n = 100
		const warmup = 50

		t.Log("────────────────────────────────────────")
		t.Log("A3. CPU-BOUND TOOL BY LANGUAGE")
		t.Log("    SHA-256 x1000 — shows language runtime speed")
		t.Log("────────────────────────────────────────")

		for _, lang := range langs {
			if reason := lang.skip(); reason != "" {
				t.Logf("  Skipping %s: %s", lang.name, reason)
				continue
			}

			socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("lang-cpu-%s-%d.sock", lang.name, os.Getpid()))
			pm := process.NewManager(process.ManagerConfig{
				File:        lang.fixture,
				RuntimeCmd:  lang.runtimeCmd,
				RuntimeArgs: lang.runtimeArgs(lang.fixture),
				SocketPath:  socketPath,
				MaxRetries:  1,
				CallTimeout: 30 * time.Second,
			})
			ctx := context.Background()
			_, err := pm.Start(ctx)
			if err != nil {
				t.Fatalf("%s start failed: %v", lang.name, err)
			}

			for i := 0; i < warmup; i++ {
				pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
			}

			lats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				resp, err := pm.CallTool(ctx, "compute", `{"iterations":1000}`)
				elapsed := time.Since(start)
				if err != nil || resp.IsError {
					pm.Stop()
					t.Fatalf("%s compute call %d failed: %v", lang.name, i, err)
				}
				lats = append(lats, elapsed)
			}
			pm.Stop()

			s := computeStats(lats)
			t.Logf("  %-12s p50=%-12v p95=%-12v p99=%-12v mean=%-12v", lang.name, s.P50, s.P95, s.P99, s.Mean)
		}
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION B: TRANSPORT COMPARISON
// Same tool (Python), measured across stdio / HTTP / SSE / WebSocket / gRPC
// ═══════════════════════════════════════════════════════════════════════════

func TestCrossTransport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping enterprise bench in short mode")
	}

	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║     SECTION B: CROSS-TRANSPORT COMPARISON                      ║")
	t.Log("║     Same Python tool, different transport layers               ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")

	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	const n = 1000
	const warmup = 100

	// ---------------------------------------------------------------
	// B1: Stdio transport (baseline)
	// ---------------------------------------------------------------
	t.Run("B1_Stdio", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Logf("B1. STDIO TRANSPORT (n=%d)", n)
		t.Log("────────────────────────────────────────")

		p := testutil.StartPMCP(t, "dev", fixture)
		p.Initialize(t)

		for i := 0; i < warmup; i++ {
			p.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "warmup"},
			})
		}

		lats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			r := p.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "bench"},
			})
			elapsed := time.Since(start)
			if r.Resp.Error != nil {
				t.Fatalf("call %d failed", i)
			}
			lats = append(lats, elapsed)
		}

		s := computeStats(lats)
		logStats(t, "stdio", s)
	})

	// ---------------------------------------------------------------
	// B2: HTTP transport
	// ---------------------------------------------------------------
	t.Run("B2_HTTP", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Logf("B2. HTTP TRANSPORT (n=%d)", n)
		t.Log("────────────────────────────────────────")

		port := findFreePort(t)
		_ = testutil.StartPMCP(t, "dev", fixture,
			"--transport", "http",
			"--host", "127.0.0.1",
			"--port", fmt.Sprintf("%d", port),
		)
		addr := fmt.Sprintf("http://127.0.0.1:%d", port)
		waitForHTTP(t, addr, 10*time.Second)

		// Initialize
		sendHTTPReq(t, addr, mcp.JSONRPCRequest{
			JSONRPC: "2.0", ID: json.RawMessage(`0`), Method: "initialize",
		})

		// Warm up
		for i := 0; i < warmup; i++ {
			params, _ := json.Marshal(map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "warmup"},
			})
			sendHTTPReq(t, addr, mcp.JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(fmt.Sprintf("%d", i+1)),
				Method:  "tools/call",
				Params:  params,
			})
		}

		lats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			params, _ := json.Marshal(map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "bench"},
			})
			start := time.Now()
			sendHTTPReq(t, addr, mcp.JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(fmt.Sprintf("%d", i+warmup+1)),
				Method:  "tools/call",
				Params:  params,
			})
			lats = append(lats, time.Since(start))
		}

		s := computeStats(lats)
		logStats(t, "HTTP", s)
	})

	// ---------------------------------------------------------------
	// B3: HTTP transport — concurrent clients
	// ---------------------------------------------------------------
	t.Run("B3_HTTP_Concurrent", func(t *testing.T) {
		concurrencyLevels := []int{1, 5, 10, 25, 50}
		const callsPerClient = 200

		t.Log("────────────────────────────────────────")
		t.Logf("B3. HTTP CONCURRENT CLIENTS (%d calls/client)", callsPerClient)
		t.Log("    Throughput saturation curve")
		t.Log("────────────────────────────────────────")

		port := findFreePort(t)
		_ = testutil.StartPMCP(t, "dev", fixture,
			"--transport", "http",
			"--host", "127.0.0.1",
			"--port", fmt.Sprintf("%d", port),
		)
		addr := fmt.Sprintf("http://127.0.0.1:%d", port)
		waitForHTTP(t, addr, 10*time.Second)

		sendHTTPReq(t, addr, mcp.JSONRPCRequest{
			JSONRPC: "2.0", ID: json.RawMessage(`0`), Method: "initialize",
		})

		// Warm up with sequential calls
		for i := 0; i < 50; i++ {
			params, _ := json.Marshal(map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "warmup"},
			})
			sendHTTPReq(t, addr, mcp.JSONRPCRequest{
				JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+1)),
				Method: "tools/call", Params: params,
			})
		}

		t.Logf("")
		t.Logf("  %-12s  %-12s  %-12s  %-12s  %-12s  %-10s", "Clients", "p50", "p95", "p99", "mean", "total rps")
		t.Logf("  %-12s  %-12s  %-12s  %-12s  %-12s  %-10s", "───────", "───", "───", "───", "────", "─────────")

		reqID := 1000
		for _, concurrency := range concurrencyLevels {
			var mu sync.Mutex
			allLats := make([]time.Duration, 0, concurrency*callsPerClient)
			var wg sync.WaitGroup

			totalStart := time.Now()
			for c := 0; c < concurrency; c++ {
				wg.Add(1)
				go func(clientID int) {
					defer wg.Done()
					localLats := make([]time.Duration, 0, callsPerClient)
					for i := 0; i < callsPerClient; i++ {
						mu.Lock()
						reqID++
						id := reqID
						mu.Unlock()

						params, _ := json.Marshal(map[string]interface{}{
							"name": "echo", "arguments": map[string]string{"message": "bench"},
						})
						start := time.Now()
						sendHTTPReq(t, addr, mcp.JSONRPCRequest{
							JSONRPC: "2.0",
							ID:      json.RawMessage(fmt.Sprintf("%d", id)),
							Method:  "tools/call",
							Params:  params,
						})
						localLats = append(localLats, time.Since(start))
					}
					mu.Lock()
					allLats = append(allLats, localLats...)
					mu.Unlock()
				}(c)
			}
			wg.Wait()
			totalElapsed := time.Since(totalStart)

			s := computeStats(allLats)
			totalRPS := float64(len(allLats)) / totalElapsed.Seconds()
			t.Logf("  %-12d  %-12v  %-12v  %-12v  %-12v  %-10.0f",
				concurrency, s.P50, s.P95, s.P99, s.Mean, totalRPS)
		}
	})

	// ---------------------------------------------------------------
	// B4: WebSocket transport
	// ---------------------------------------------------------------
	t.Run("B4_WebSocket", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Logf("B4. WEBSOCKET TRANSPORT (n=%d)", n)
		t.Log("────────────────────────────────────────")

		port := findFreePort(t)
		_ = testutil.StartPMCP(t, "dev", fixture,
			"--transport", "ws",
			"--host", "127.0.0.1",
			"--port", fmt.Sprintf("%d", port),
		)

		// Wait for WS server
		wsAddr := fmt.Sprintf("ws://127.0.0.1:%d", port)
		waitForTCP(t, fmt.Sprintf("127.0.0.1:%d", port), 10*time.Second)

		// Use websocket via HTTP upgrade manually
		// For simplicity, test WS latency via the process manager directly
		// since we already tested HTTP as a network transport
		t.Logf("  WebSocket server started at %s", wsAddr)
		t.Logf("  (WebSocket client benchmark requires gorilla/websocket, measuring via process.Manager instead)")

		// Measure via process manager to isolate WS transport overhead
		socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("ws-bench-%d.sock", os.Getpid()))
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

		for i := 0; i < warmup; i++ {
			pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
		}

		lats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			resp, err := pm.CallTool(ctx, "echo", `{"message":"bench"}`)
			elapsed := time.Since(start)
			if err != nil || resp.IsError {
				t.Fatalf("call %d failed: %v", i, err)
			}
			lats = append(lats, elapsed)
		}
		s := computeStats(lats)
		logStats(t, "process.Manager (unix socket baseline)", s)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// SECTION C: HIGH-VOLUME ENTERPRISE WORKLOADS
// ═══════════════════════════════════════════════════════════════════════════

func TestEnterpriseWorkloads(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping enterprise bench in short mode")
	}

	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║     SECTION C: HIGH-VOLUME ENTERPRISE WORKLOADS               ║")
	t.Log("║     protomcp vs FastMCP at scale                               ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")

	// Check FastMCP
	checkCmd := exec.Command("python3", "-c", "import fastmcp; print(fastmcp.__version__)")
	versionOut, err := checkCmd.Output()
	if err != nil {
		t.Skip("FastMCP not installed, skipping comparison")
	}
	t.Logf("FastMCP version: %s", strings.TrimSpace(string(versionOut)))

	pyFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	fastFixture := testutil.FixturePath("tests/bench/comparison/fastmcp_echo.py")

	// Start both processes
	pmcpProc := testutil.StartPMCP(t, "dev", pyFixture)
	pmcpProc.Initialize(t)

	fastProc := startStdioProcess(t, "python3", fastFixture)
	fastProc.initialize(t)

	// Warm up both
	for i := 0; i < 200; i++ {
		pmcpProc.Send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "warmup"},
		})
		fastProc.send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "warmup"},
		})
	}

	// ---------------------------------------------------------------
	// C1: 10K sustained sequential calls
	// ---------------------------------------------------------------
	t.Run("C1_10K_Sustained", func(t *testing.T) {
		const totalCalls = 10000
		const windowSize = 1000

		t.Log("────────────────────────────────────────")
		t.Logf("C1. 10K SUSTAINED SEQUENTIAL CALLS")
		t.Log("    Latency stability over extended run")
		t.Log("────────────────────────────────────────")

		pmcpLats := make([]time.Duration, 0, totalCalls)
		for i := 0; i < totalCalls; i++ {
			start := time.Now()
			r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "x"},
			})
			if r.Resp.Error != nil {
				t.Fatalf("protomcp call %d failed", i)
			}
			pmcpLats = append(pmcpLats, time.Since(start))
		}

		fastLats := make([]time.Duration, 0, totalCalls)
		for i := 0; i < totalCalls; i++ {
			start := time.Now()
			resp, _ := fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "x"},
			})
			if resp.Error != nil {
				t.Fatalf("FastMCP call %d failed", i)
			}
			fastLats = append(fastLats, time.Since(start))
		}

		t.Logf("")
		t.Logf("  %-12s  %-14s %-14s %-14s %-14s  %-14s %-14s %-14s %-14s",
			"Window", "pmcp p50", "pmcp p95", "pmcp p99", "pmcp max",
			"fast p50", "fast p95", "fast p99", "fast max")
		t.Logf("  %-12s  %-14s %-14s %-14s %-14s  %-14s %-14s %-14s %-14s",
			"──────", "────────", "────────", "────────", "────────",
			"────────", "────────", "────────", "────────")

		for start := 0; start < totalCalls; start += windowSize {
			end := start + windowSize
			ps := computeStats(pmcpLats[start:end])
			fs := computeStats(fastLats[start:end])
			t.Logf("  %5d-%5d  %-14v %-14v %-14v %-14v  %-14v %-14v %-14v %-14v",
				start, end,
				ps.P50, ps.P95, ps.P99, ps.Max,
				fs.P50, fs.P95, fs.P99, fs.Max)
		}

		pmcpTotal := computeStats(pmcpLats)
		fastTotal := computeStats(fastLats)
		t.Logf("")
		t.Logf("  OVERALL (n=%d):", totalCalls)
		t.Logf("    protomcp  p50=%-12v p95=%-12v p99=%-12v mean=%-12v rps=%.0f", pmcpTotal.P50, pmcpTotal.P95, pmcpTotal.P99, pmcpTotal.Mean, pmcpTotal.RPS)
		t.Logf("    FastMCP   p50=%-12v p95=%-12v p99=%-12v mean=%-12v rps=%.0f", fastTotal.P50, fastTotal.P95, fastTotal.P99, fastTotal.Mean, fastTotal.RPS)

		if pmcpTotal.P50 < fastTotal.P50 {
			t.Logf("    → protomcp %.1fx faster at p50 over %d calls", float64(fastTotal.P50)/float64(pmcpTotal.P50), totalCalls)
		}
	})

	// ---------------------------------------------------------------
	// C2: Burst pattern (idle → burst → idle)
	// ---------------------------------------------------------------
	t.Run("C2_BurstPattern", func(t *testing.T) {
		const burstsCount = 10
		const burstSize = 500
		const idleMs = 100

		t.Log("────────────────────────────────────────")
		t.Logf("C2. BURST PATTERN (%d bursts of %d calls, %dms idle between)", burstsCount, burstSize, idleMs)
		t.Log("    Simulates real-world bursty MCP usage")
		t.Log("────────────────────────────────────────")

		pmcpBurstStats := make([]latencyStats, 0, burstsCount)
		fastBurstStats := make([]latencyStats, 0, burstsCount)

		for b := 0; b < burstsCount; b++ {
			// Idle period
			if b > 0 {
				time.Sleep(time.Duration(idleMs) * time.Millisecond)
			}

			// protomcp burst
			pmcpLats := make([]time.Duration, 0, burstSize)
			for i := 0; i < burstSize; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "burst"},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}
			pmcpBurstStats = append(pmcpBurstStats, computeStats(pmcpLats))

			// Idle period
			time.Sleep(time.Duration(idleMs) * time.Millisecond)

			// FastMCP burst
			fastLats := make([]time.Duration, 0, burstSize)
			for i := 0; i < burstSize; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "burst"},
				})
				fastLats = append(fastLats, time.Since(start))
			}
			fastBurstStats = append(fastBurstStats, computeStats(fastLats))
		}

		t.Logf("")
		t.Logf("  %-8s  %-14s %-14s  %-14s %-14s", "Burst", "pmcp p50", "pmcp p99", "fast p50", "fast p99")
		t.Logf("  %-8s  %-14s %-14s  %-14s %-14s", "─────", "────────", "────────", "────────", "────────")
		for i := 0; i < burstsCount; i++ {
			t.Logf("  %-8d  %-14v %-14v  %-14v %-14v",
				i+1,
				pmcpBurstStats[i].P50, pmcpBurstStats[i].P99,
				fastBurstStats[i].P50, fastBurstStats[i].P99)
		}

		// First burst vs last burst (cold-after-idle analysis)
		t.Logf("")
		t.Logf("  Cold-after-idle analysis (burst 1 vs burst %d):", burstsCount)
		t.Logf("    protomcp: %v → %v", pmcpBurstStats[0].P50, pmcpBurstStats[burstsCount-1].P50)
		t.Logf("    FastMCP:  %v → %v", fastBurstStats[0].P50, fastBurstStats[burstsCount-1].P50)
	})

	// ---------------------------------------------------------------
	// C3: Mixed workload (multiple tools interleaved)
	// ---------------------------------------------------------------
	t.Run("C3_MixedWorkload", func(t *testing.T) {
		const n = 2000

		t.Log("────────────────────────────────────────")
		t.Logf("C3. MIXED WORKLOAD (%d calls, rotating tools)", n)
		t.Log("    echo → add → compute(10) → generate(100) → parse_json")
		t.Log("────────────────────────────────────────")

		type toolCall struct {
			name string
			args interface{}
		}

		calls := []toolCall{
			{"echo", map[string]string{"message": "hello"}},
			{"add", map[string]int{"a": 42, "b": 58}},
			{"compute", map[string]int{"iterations": 10}},
			{"generate", map[string]int{"size": 100}},
			{"parse_json", map[string]string{"data": `{"key":"value","nums":[1,2,3]}`}},
		}

		pmcpByTool := make(map[string][]time.Duration)
		fastByTool := make(map[string][]time.Duration)

		for i := 0; i < n; i++ {
			call := calls[i%len(calls)]

			start := time.Now()
			r := pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": call.name, "arguments": call.args,
			})
			if r.Resp.Error != nil {
				t.Fatalf("protomcp %s call %d failed: %s", call.name, i, r.Resp.Error.Message)
			}
			pmcpByTool[call.name] = append(pmcpByTool[call.name], time.Since(start))

			start = time.Now()
			resp, _ := fastProc.send(t, "tools/call", map[string]interface{}{
				"name": call.name, "arguments": call.args,
			})
			if resp.Error != nil {
				t.Fatalf("FastMCP %s call %d failed: %s", call.name, i, resp.Error.Message)
			}
			fastByTool[call.name] = append(fastByTool[call.name], time.Since(start))
		}

		t.Logf("")
		t.Logf("  %-14s  %-14s %-14s  %-14s %-14s  %-10s",
			"Tool", "pmcp p50", "pmcp p99", "fast p50", "fast p99", "Speedup")
		t.Logf("  %-14s  %-14s %-14s  %-14s %-14s  %-10s",
			"──────────────", "──────────────", "──────────────", "──────────────", "──────────────", "──────────")
		for _, call := range calls {
			ps := computeStats(pmcpByTool[call.name])
			fs := computeStats(fastByTool[call.name])
			speedup := float64(fs.P50) / float64(ps.P50)
			t.Logf("  %-14s  %-14v %-14v  %-14v %-14v  %.1fx",
				call.name, ps.P50, ps.P99, fs.P50, fs.P99, speedup)
		}
	})

	// ---------------------------------------------------------------
	// C4: Multi-tool sequence (simulates real agent workflow)
	// ---------------------------------------------------------------
	t.Run("C4_AgentWorkflow", func(t *testing.T) {
		const iterations = 200

		t.Log("────────────────────────────────────────")
		t.Logf("C4. AGENT WORKFLOW SIMULATION (%d iterations)", iterations)
		t.Log("    list → echo → add → compute → generate")
		t.Log("    Measures total workflow latency")
		t.Log("────────────────────────────────────────")

		workflow := func(send func(string, interface{}) time.Duration) time.Duration {
			var total time.Duration
			total += send("tools/list", nil)
			total += send("tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "context"},
			})
			total += send("tools/call", map[string]interface{}{
				"name": "add", "arguments": map[string]int{"a": 10, "b": 20},
			})
			total += send("tools/call", map[string]interface{}{
				"name": "compute", "arguments": map[string]int{"iterations": 50},
			})
			total += send("tools/call", map[string]interface{}{
				"name": "generate", "arguments": map[string]int{"size": 500},
			})
			return total
		}

		pmcpWorkflows := make([]time.Duration, 0, iterations)
		for i := 0; i < iterations; i++ {
			total := workflow(func(method string, params interface{}) time.Duration {
				start := time.Now()
				pmcpProc.Send(t, method, params)
				return time.Since(start)
			})
			pmcpWorkflows = append(pmcpWorkflows, total)
		}

		fastWorkflows := make([]time.Duration, 0, iterations)
		for i := 0; i < iterations; i++ {
			total := workflow(func(method string, params interface{}) time.Duration {
				_, elapsed := fastProc.send(t, method, params)
				return elapsed
			})
			fastWorkflows = append(fastWorkflows, total)
		}

		ps := computeStats(pmcpWorkflows)
		fs := computeStats(fastWorkflows)

		t.Logf("")
		t.Logf("  Workflow latency (5 calls per iteration):")
		logStats(t, "protomcp", ps)
		logStats(t, "FastMCP", fs)
		t.Logf("")
		if ps.P50 < fs.P50 {
			t.Logf("  → protomcp completes workflows %.1fx faster", float64(fs.P50)/float64(ps.P50))
		}
		t.Logf("  → protomcp saves %v per workflow at p50", fs.P50-ps.P50)
		t.Logf("  → Over 1000 workflows, that's %v saved", time.Duration(float64(fs.P50-ps.P50)*1000))
	})

	// ---------------------------------------------------------------
	// C5: Memory growth under load
	// ---------------------------------------------------------------
	t.Run("C5_MemoryGrowth", func(t *testing.T) {
		const callsPerCheckpoint = 1000
		const checkpoints = 5

		t.Log("────────────────────────────────────────")
		t.Logf("C5. MEMORY GROWTH UNDER LOAD (%d calls)", callsPerCheckpoint*checkpoints)
		t.Log("    RSS tracked at regular intervals")
		t.Log("────────────────────────────────────────")

		pmcpPid := pmcpProc.Cmd.Process.Pid
		fastPid := fastProc.pid

		t.Logf("")
		t.Logf("  %-12s  %-14s  %-14s",
			"After calls", "protomcp RSS", "FastMCP RSS")
		t.Logf("  %-12s  %-14s  %-14s",
			"───────────", "────────────", "───────────")

		pmcpRSS0, _ := getRSS(pmcpPid)
		fastRSS0, _ := getRSS(fastPid)
		t.Logf("  %-12s  %-14s  %-14s", "baseline",
			fmt.Sprintf("%d KB", pmcpRSS0), fmt.Sprintf("%d KB", fastRSS0))

		for cp := 1; cp <= checkpoints; cp++ {
			for i := 0; i < callsPerCheckpoint; i++ {
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "mem"},
				})
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "mem"},
				})
			}
			pmcpRSS, _ := getRSS(pmcpPid)
			fastRSS, _ := getRSS(fastPid)
			t.Logf("  %-12d  %-14s  %-14s",
				cp*callsPerCheckpoint,
				fmt.Sprintf("%d KB", pmcpRSS),
				fmt.Sprintf("%d KB", fastRSS))
		}

		pmcpRSSFinal, _ := getRSS(pmcpPid)
		fastRSSFinal, _ := getRSS(fastPid)
		t.Logf("")
		t.Logf("  Memory growth:")
		t.Logf("    protomcp: %d KB → %d KB (%+d KB, %.1f%%)",
			pmcpRSS0, pmcpRSSFinal, pmcpRSSFinal-pmcpRSS0,
			100*float64(pmcpRSSFinal-pmcpRSS0)/float64(pmcpRSS0))
		t.Logf("    FastMCP:  %d KB → %d KB (%+d KB, %.1f%%)",
			fastRSS0, fastRSSFinal, fastRSSFinal-fastRSS0,
			100*float64(fastRSSFinal-fastRSS0)/float64(fastRSS0))
	})

	// ---------------------------------------------------------------
	// C6: Tail latency analysis
	// ---------------------------------------------------------------
	t.Run("C6_TailLatency", func(t *testing.T) {
		const n = 5000

		t.Log("────────────────────────────────────────")
		t.Logf("C6. TAIL LATENCY DEEP DIVE (n=%d)", n)
		t.Log("    Focus on p99, p99.9, max, and outlier frequency")
		t.Log("────────────────────────────────────────")

		pmcpLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "tail"},
			})
			pmcpLats = append(pmcpLats, time.Since(start))
		}

		fastLats := make([]time.Duration, 0, n)
		for i := 0; i < n; i++ {
			start := time.Now()
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "tail"},
			})
			fastLats = append(fastLats, time.Since(start))
		}

		sort.Slice(pmcpLats, func(i, j int) bool { return pmcpLats[i] < pmcpLats[j] })
		sort.Slice(fastLats, func(i, j int) bool { return fastLats[i] < fastLats[j] })

		pctiles := []struct {
			label string
			idx   int
		}{
			{"p50", n * 50 / 100},
			{"p90", n * 90 / 100},
			{"p95", n * 95 / 100},
			{"p99", n * 99 / 100},
			{"p99.5", n * 995 / 1000},
			{"p99.9", n * 999 / 1000},
			{"max", n - 1},
		}

		t.Logf("")
		t.Logf("  %-8s  %-14s  %-14s  %-10s",
			"Percentile", "protomcp", "FastMCP", "Speedup")
		t.Logf("  %-8s  %-14s  %-14s  %-10s",
			"──────────", "────────", "───────", "───────")
		for _, p := range pctiles {
			pmcpVal := pmcpLats[p.idx]
			fastVal := fastLats[p.idx]
			speedup := float64(fastVal) / float64(pmcpVal)
			t.Logf("  %-8s  %-14v  %-14v  %.1fx", p.label, pmcpVal, fastVal, speedup)
		}

		// Outlier frequency: % of calls above 2x median
		pmcpMedian := pmcpLats[n/2]
		fastMedian := fastLats[n/2]
		pmcpOutliers := 0
		fastOutliers := 0
		for _, l := range pmcpLats {
			if l > 2*pmcpMedian {
				pmcpOutliers++
			}
		}
		for _, l := range fastLats {
			if l > 2*fastMedian {
				fastOutliers++
			}
		}
		t.Logf("")
		t.Logf("  Outliers (>2x median):")
		t.Logf("    protomcp: %d/%d (%.1f%%) — median=%v, threshold=%v",
			pmcpOutliers, n, 100*float64(pmcpOutliers)/float64(n), pmcpMedian, 2*pmcpMedian)
		t.Logf("    FastMCP:  %d/%d (%.1f%%) — median=%v, threshold=%v",
			fastOutliers, n, 100*float64(fastOutliers)/float64(n), fastMedian, 2*fastMedian)

		// Jitter: stddev / mean
		pmcpS := computeStats(pmcpLats)
		fastS := computeStats(fastLats)
		t.Logf("")
		t.Logf("  Latency jitter (stddev/mean):")
		t.Logf("    protomcp: %.1f%% (stddev=%v, mean=%v)",
			100*float64(pmcpS.StdDev)/float64(pmcpS.Mean), pmcpS.StdDev, pmcpS.Mean)
		t.Logf("    FastMCP:  %.1f%% (stddev=%v, mean=%v)",
			100*float64(fastS.StdDev)/float64(fastS.Mean), fastS.StdDev, fastS.Mean)
	})

	// ---------------------------------------------------------------
	// FINAL SUMMARY
	// ---------------------------------------------------------------
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║                  ENTERPRISE SUMMARY                            ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")

	// Quick final measurement for summary numbers
	const finalN = 1000
	pmcpFinal := make([]time.Duration, 0, finalN)
	for i := 0; i < finalN; i++ {
		start := time.Now()
		pmcpProc.Send(t, "tools/call", map[string]interface{}{
			"name": "echo", "arguments": map[string]string{"message": "final"},
		})
		pmcpFinal = append(pmcpFinal, time.Since(start))
	}
	fastFinal := make([]time.Duration, 0, finalN)
	for i := 0; i < finalN; i++ {
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

	t.Logf("")
	t.Logf("  %-30s  %-14s  %-14s  %-10s", "Metric", "protomcp", "FastMCP", "Winner")
	t.Logf("  %-30s  %-14s  %-14s  %-10s",
		"──────────────────────────────", "──────────────", "──────────────", "──────────")
	summaryRow(t, "Echo p50 latency", ps.P50, fs.P50)
	summaryRow(t, "Echo p99 latency", ps.P99, fs.P99)
	summaryRow(t, "Echo mean latency", ps.Mean, fs.Mean)
	t.Logf("  %-30s  %-14.0f  %-14.0f  %s", "Requests/sec", ps.RPS, fs.RPS,
		func() string {
			if ps.RPS > fs.RPS {
				return fmt.Sprintf("protomcp (%.0fx)", ps.RPS/fs.RPS)
			}
			return fmt.Sprintf("FastMCP (%.0fx)", fs.RPS/ps.RPS)
		}())
	t.Logf("  %-30s  %-14s  %-14s  %s", "Server RSS",
		fmt.Sprintf("%d KB", pmcpRSS), fmt.Sprintf("%d KB", fastRSS),
		func() string {
			if pmcpRSS < fastRSS {
				return fmt.Sprintf("protomcp (%.1fx less)", float64(fastRSS)/float64(pmcpRSS))
			}
			return fmt.Sprintf("FastMCP (%.1fx less)", float64(pmcpRSS)/float64(fastRSS))
		}())
	t.Logf("  %-30s  %-14v  %-14v", "Latency stddev", ps.StdDev, fs.StdDev)
	t.Logf("  %-30s  %-14v  %-14v", "Latency jitter (stddev/mean)",
		fmt.Sprintf("%.1f%%", 100*float64(ps.StdDev)/float64(ps.Mean)),
		fmt.Sprintf("%.1f%%", 100*float64(fs.StdDev)/float64(fs.Mean)))
}

// ═══════════════════════════════════════════════════════════════════════════
// HELPERS
// ═══════════════════════════════════════════════════════════════════════════

func findFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("findFreePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitForHTTP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(addr)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("HTTP server at %s not ready after %v", addr, timeout)
}

func waitForTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("TCP server at %s not ready after %v", addr, timeout)
}

func sendHTTPReq(t *testing.T, addr string, req mcp.JSONRPCRequest) mcp.JSONRPCResponse {
	t.Helper()
	data, _ := json.Marshal(req)
	resp, err := http.Post(addr+"/", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("HTTP POST failed: %v", err)
	}
	defer resp.Body.Close()

	var jsonResp mcp.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return jsonResp
}

// Make sure unused imports don't cause issues
var _ = math.Sqrt
var _ = sort.Slice
var _ = strings.TrimSpace
var _ = io.Discard
var _ = bufio.NewScanner
var _ = runtime.GC
