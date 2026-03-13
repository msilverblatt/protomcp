package comparison_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// ═══════════════════════════════════════════════════════════════════════════
// SECTION D: DEEP FASTMCP vs PROTOMCP HEAD-TO-HEAD COMPARISON
// Tests every dimension: transports, concurrency, payloads, workloads,
// and resource efficiency — both frameworks side by side.
// ═══════════════════════════════════════════════════════════════════════════

// sendHTTPReqToPath posts a JSON-RPC request to addr+path and returns the response.
// Needed because FastMCP streamable-http uses /mcp, protomcp uses /.
func sendHTTPReqToPath(t *testing.T, addr, path string, req mcp.JSONRPCRequest) mcp.JSONRPCResponse {
	t.Helper()
	data, _ := json.Marshal(req)
	resp, err := http.Post(addr+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("HTTP POST to %s%s failed: %v", addr, path, err)
	}
	defer resp.Body.Close()
	var jsonResp mcp.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		t.Fatalf("decode response from %s%s: %v (status=%d)", addr, path, err, resp.StatusCode)
	}
	return jsonResp
}

// sendHTTPReqToPathNoFatal is like sendHTTPReqToPath but returns an error instead of
// calling t.Fatalf — safe for use in goroutines where Fatalf causes panics.
func sendHTTPReqToPathNoFatal(addr, path string, req mcp.JSONRPCRequest) (mcp.JSONRPCResponse, error) {
	data, _ := json.Marshal(req)
	resp, err := http.Post(addr+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return mcp.JSONRPCResponse{}, err
	}
	defer resp.Body.Close()
	var jsonResp mcp.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
		return mcp.JSONRPCResponse{}, err
	}
	return jsonResp, nil
}

// startFastMCPHTTP launches the fastmcp CLI with the given transport and port.
func startFastMCPHTTP(t *testing.T, fixture, transport string, port int) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("fastmcp", "run", fixture,
		"--transport", transport,
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", port),
	)
	cmd.Stderr = io.Discard
	cmd.Stdout = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fastmcp %s: %v", transport, err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
	})
	return cmd
}

// initializeHTTP sends the MCP initialize handshake over HTTP.
func initializeHTTP(t *testing.T, addr, path string) {
	t.Helper()
	sendHTTPReqToPath(t, addr, path, mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`0`),
		Method:  "initialize",
		Params: func() json.RawMessage {
			p, _ := json.Marshal(map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]interface{}{},
				"clientInfo":      map[string]string{"name": "bench", "version": "1.0"},
			})
			return p
		}(),
	})
}

// waitForHTTPPost polls until the server accepts a POST request on the given path.
// Unlike waitForHTTP (which does GET), this works for FastMCP endpoints that only accept POST.
func waitForHTTPPost(t *testing.T, addr, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	body := []byte(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}`)
	for time.Now().Before(deadline) {
		resp, err := http.Post(addr+path, "application/json", bytes.NewReader(body))
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("HTTP POST to %s%s not ready after %v", addr, path, timeout)
}

// sseClient implements the MCP SSE protocol: connects to GET /sse, reads the
// endpoint event, then POSTs JSON-RPC requests to that endpoint and reads
// responses from the SSE stream.
type sseClient struct {
	baseAddr    string
	postURL     string
	respCh      chan mcp.JSONRPCResponse
	sseResp     *http.Response
	nextID      int
	mu          sync.Mutex
}

// connectSSE connects to a FastMCP SSE server. It opens the GET /sse stream,
// reads the endpoint event, and starts a goroutine to read response messages.
func connectSSE(t *testing.T, baseAddr string, timeout time.Duration) *sseClient {
	t.Helper()

	c := &sseClient{
		baseAddr: baseAddr,
		respCh:   make(chan mcp.JSONRPCResponse, 64),
	}

	// Connect to SSE stream
	deadline := time.Now().Add(timeout)
	var sseResp *http.Response
	for time.Now().Before(deadline) {
		var err error
		sseResp, err = http.Get(baseAddr + "/sse")
		if err == nil && sseResp.StatusCode == 200 {
			break
		}
		if sseResp != nil {
			sseResp.Body.Close()
			sseResp = nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if sseResp == nil {
		t.Fatalf("SSE connect to %s/sse failed after %v", baseAddr, timeout)
	}
	c.sseResp = sseResp

	// Read the endpoint event from the SSE stream
	scanner := bufio.NewScanner(sseResp.Body)
	endpointCh := make(chan string, 1)

	// Start SSE reader goroutine
	go func() {
		var eventType string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if eventType == "endpoint" {
					endpointCh <- data
				} else if eventType == "message" {
					var resp mcp.JSONRPCResponse
					if json.Unmarshal([]byte(data), &resp) == nil {
						c.respCh <- resp
					}
				}
				eventType = ""
			}
		}
	}()

	// Wait for the endpoint event
	select {
	case endpoint := <-endpointCh:
		// endpoint is like "/messages/?session_id=XXX"
		c.postURL = baseAddr + endpoint
	case <-time.After(timeout):
		sseResp.Body.Close()
		t.Fatalf("SSE endpoint event not received from %s after %v", baseAddr, timeout)
	}

	t.Cleanup(func() {
		sseResp.Body.Close()
	})

	return c
}

// send posts a JSON-RPC request and waits for the response on the SSE stream.
func (c *sseClient) send(t *testing.T, method string, params interface{}) (mcp.JSONRPCResponse, time.Duration) {
	t.Helper()
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

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

	resp, err := http.Post(c.postURL, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("SSE POST to %s failed: %v", c.postURL, err)
	}
	resp.Body.Close()

	// Wait for response on SSE stream
	select {
	case r := <-c.respCh:
		return r, time.Since(start)
	case <-time.After(30 * time.Second):
		t.Fatalf("SSE response timeout for method=%s id=%d", method, id)
		return mcp.JSONRPCResponse{}, 0
	}
}

// initialize sends the MCP initialize handshake over SSE.
func (c *sseClient) initialize(t *testing.T) {
	t.Helper()
	c.send(t, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "bench", "version": "1.0"},
	})
}

func TestDeepFastMCPComparison(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deep comparison in short mode")
	}

	// Check FastMCP
	checkCmd := exec.Command("python3", "-c", "import fastmcp; print(fastmcp.__version__)")
	versionOut, err := checkCmd.Output()
	if err != nil {
		t.Skip("FastMCP not installed, skipping deep comparison")
	}
	fastVersion := strings.TrimSpace(string(versionOut))

	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║     SECTION D: DEEP FASTMCP vs PROTOMCP COMPARISON            ║")
	t.Log("║     Head-to-head across transports, payloads, workloads        ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Logf("  FastMCP version: %s", fastVersion)
	t.Logf("  OS: %s/%s", runtime.GOOS, runtime.GOARCH)

	pyFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")
	fastFixture := testutil.FixturePath("tests/bench/comparison/fastmcp_echo.py")

	// Collect summary data across sections
	type summaryEntry struct {
		metric  string
		pmcpVal string
		fastVal string
		winner  string
	}
	var summary []summaryEntry
	addSummaryDuration := func(metric string, pmcp, fast time.Duration) {
		winner := ""
		if pmcp < fast {
			winner = fmt.Sprintf("protomcp (%.0fx)", float64(fast)/float64(pmcp))
		} else if fast < pmcp {
			winner = fmt.Sprintf("FastMCP (%.0fx)", float64(pmcp)/float64(fast))
		} else {
			winner = "tie"
		}
		summary = append(summary, summaryEntry{metric, pmcp.String(), fast.String(), winner})
	}
	addSummaryFloat := func(metric string, pmcpVal, fastVal float64, unit string, lowerBetter bool) {
		winner := ""
		if lowerBetter {
			if pmcpVal < fastVal {
				winner = fmt.Sprintf("protomcp (%.1fx)", fastVal/pmcpVal)
			} else {
				winner = fmt.Sprintf("FastMCP (%.1fx)", pmcpVal/fastVal)
			}
		} else {
			if pmcpVal > fastVal {
				winner = fmt.Sprintf("protomcp (%.1fx)", pmcpVal/fastVal)
			} else {
				winner = fmt.Sprintf("FastMCP (%.1fx)", fastVal/pmcpVal)
			}
		}
		summary = append(summary, summaryEntry{
			metric,
			fmt.Sprintf("%.0f %s", pmcpVal, unit),
			fmt.Sprintf("%.0f %s", fastVal, unit),
			winner,
		})
	}

	// ═══════════════════════════════════════════════════════════════════
	// D1: COLD START ACROSS TRANSPORTS
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D1_ColdStartTransports", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("D1. COLD START ACROSS TRANSPORTS")
		t.Log("    Process spawn → handshake → first tool call")
		t.Log("────────────────────────────────────────")

		type transportConfig struct {
			name           string
			pmcpTransport  string
			fastTransport  string
			fastEndpoint   string
			useSSEClient   bool // true = FastMCP SSE protocol (GET stream + POST to session)
		}
		transports := []transportConfig{
			{"stdio", "stdio", "stdio", "", false},
			{"HTTP", "http", "streamable-http", "/mcp", false},
			{"SSE", "sse", "sse", "", true},
		}

		const trials = 5

		t.Logf("")
		t.Logf("  %-10s  %-18s  %-18s  %-10s", "Transport", "protomcp p50", "FastMCP p50", "Speedup")
		t.Logf("  %-10s  %-18s  %-18s  %-10s", "─────────", "────────────", "───────────", "───────")

		for _, tc := range transports {
			pmcpLats := make([]time.Duration, 0, trials)
			fastLats := make([]time.Duration, 0, trials)

			if tc.pmcpTransport == "stdio" {
				// Stdio cold start
				for trial := 0; trial < trials; trial++ {
					start := time.Now()
					p := testutil.StartPMCP(t, "dev", pyFixture)
					p.Initialize(t)
					p.Send(t, "tools/call", map[string]interface{}{
						"name": "echo", "arguments": map[string]string{"message": "cold"},
					})
					pmcpLats = append(pmcpLats, time.Since(start))
				}

				for trial := 0; trial < trials; trial++ {
					start := time.Now()
					fp := startStdioProcess(t, "python3", fastFixture)
					fp.initialize(t)
					fp.send(t, "tools/call", map[string]interface{}{
						"name": "echo", "arguments": map[string]string{"message": "cold"},
					})
					fastLats = append(fastLats, time.Since(start))
				}
			} else {
				// Network transport cold start
				for trial := 0; trial < trials; trial++ {
					port := findFreePort(t)
					start := time.Now()
					testutil.StartPMCP(t, "dev", pyFixture,
						"--transport", tc.pmcpTransport,
						"--host", "127.0.0.1",
						"--port", fmt.Sprintf("%d", port),
					)
					addr := fmt.Sprintf("http://127.0.0.1:%d", port)
					waitForTCP(t, fmt.Sprintf("127.0.0.1:%d", port), 15*time.Second)
					initializeHTTP(t, addr, "/")
					sendHTTPReqToPath(t, addr, "/", mcp.JSONRPCRequest{
						JSONRPC: "2.0",
						ID:      json.RawMessage(fmt.Sprintf("%d", trial+1)),
						Method:  "tools/call",
						Params: func() json.RawMessage {
							p, _ := json.Marshal(map[string]interface{}{
								"name": "echo", "arguments": map[string]string{"message": "cold"},
							})
							return p
						}(),
					})
					pmcpLats = append(pmcpLats, time.Since(start))
				}

				if tc.useSSEClient {
					// FastMCP SSE: connect via SSE client protocol
					for trial := 0; trial < trials; trial++ {
						port := findFreePort(t)
						start := time.Now()
						startFastMCPHTTP(t, fastFixture, tc.fastTransport, port)
						addr := fmt.Sprintf("http://127.0.0.1:%d", port)
						sc := connectSSE(t, addr, 20*time.Second)
						sc.initialize(t)
						sc.send(t, "tools/call", map[string]interface{}{
							"name": "echo", "arguments": map[string]string{"message": "cold"},
						})
						fastLats = append(fastLats, time.Since(start))
					}
				} else {
					// FastMCP HTTP: simple POST
					for trial := 0; trial < trials; trial++ {
						port := findFreePort(t)
						start := time.Now()
						startFastMCPHTTP(t, fastFixture, tc.fastTransport, port)
						addr := fmt.Sprintf("http://127.0.0.1:%d", port)
						waitForHTTPPost(t, addr, tc.fastEndpoint, 20*time.Second)
						sendHTTPReqToPath(t, addr, tc.fastEndpoint, mcp.JSONRPCRequest{
							JSONRPC: "2.0",
							ID:      json.RawMessage(fmt.Sprintf("%d", trial+1)),
							Method:  "tools/call",
							Params: func() json.RawMessage {
								p, _ := json.Marshal(map[string]interface{}{
									"name": "echo", "arguments": map[string]string{"message": "cold"},
								})
								return p
							}(),
						})
						fastLats = append(fastLats, time.Since(start))
					}
				}
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			speedup := float64(fs.P50) / float64(ps.P50)
			t.Logf("  %-10s  %-18v  %-18v  %.1fx", tc.name, ps.P50, fs.P50, speedup)

			if tc.name == "stdio" {
				addSummaryDuration("Cold start (stdio)", ps.P50, fs.P50)
			}
		}
	})

	// ═══════════════════════════════════════════════════════════════════
	// D2: TRANSPORT LATENCY HEAD-TO-HEAD
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D2_TransportLatency", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("D2. TRANSPORT LATENCY HEAD-TO-HEAD (n=1000)")
		t.Log("    Same echo call, different transports, both frameworks")
		t.Log("────────────────────────────────────────")

		const n = 1000
		const warmup = 100

		// --- D2a: Stdio ---
		t.Run("D2a_Stdio", func(t *testing.T) {
			t.Log("")
			t.Log("  D2a. STDIO")

			pmcpProc := testutil.StartPMCP(t, "dev", pyFixture)
			pmcpProc.Initialize(t)
			fastProc := startStdioProcess(t, "python3", fastFixture)
			fastProc.initialize(t)

			for i := 0; i < warmup; i++ {
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
			}

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp stdio", ps)
			logStats(t, "FastMCP  stdio", fs)
			logComparison(t, "stdio p50", ps.P50, fs.P50)

			addSummaryDuration("Stdio echo p50", ps.P50, fs.P50)
			addSummaryFloat("Stdio rps", ps.RPS, fs.RPS, "rps", false)
		})

		// --- D2b: HTTP ---
		t.Run("D2b_HTTP", func(t *testing.T) {
			t.Log("")
			t.Log("  D2b. HTTP (protomcp http vs FastMCP streamable-http)")

			pmcpPort := findFreePort(t)
			testutil.StartPMCP(t, "dev", pyFixture,
				"--transport", "http",
				"--host", "127.0.0.1",
				"--port", fmt.Sprintf("%d", pmcpPort),
			)
			pmcpAddr := fmt.Sprintf("http://127.0.0.1:%d", pmcpPort)
			waitForHTTP(t, pmcpAddr, 10*time.Second)
			initializeHTTP(t, pmcpAddr, "/")

			fastPort := findFreePort(t)
			startFastMCPHTTP(t, fastFixture, "streamable-http", fastPort)
			fastAddr := fmt.Sprintf("http://127.0.0.1:%d", fastPort)
			waitForHTTPPost(t, fastAddr, "/mcp", 20*time.Second)
			initializeHTTP(t, fastAddr, "/mcp")

			// Warmup
			for i := 0; i < warmup; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
				sendHTTPReqToPath(t, pmcpAddr, "/", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+1)),
					Method: "tools/call", Params: params,
				})
				sendHTTPReqToPath(t, fastAddr, "/mcp", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+1)),
					Method: "tools/call", Params: params,
				})
			}

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				start := time.Now()
				sendHTTPReqToPath(t, pmcpAddr, "/", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+warmup+1)),
					Method: "tools/call", Params: params,
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				start := time.Now()
				sendHTTPReqToPath(t, fastAddr, "/mcp", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+warmup+1)),
					Method: "tools/call", Params: params,
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp HTTP", ps)
			logStats(t, "FastMCP  HTTP", fs)
			logComparison(t, "HTTP p50", ps.P50, fs.P50)

			addSummaryDuration("HTTP echo p50", ps.P50, fs.P50)
			addSummaryFloat("HTTP rps", ps.RPS, fs.RPS, "rps", false)
		})

		// --- D2c: SSE ---
		t.Run("D2c_SSE", func(t *testing.T) {
			t.Log("")
			t.Log("  D2c. SSE (protomcp sse vs FastMCP sse)")

			// protomcp SSE: POST to / returns JSON directly
			pmcpPort := findFreePort(t)
			testutil.StartPMCP(t, "dev", pyFixture,
				"--transport", "sse",
				"--host", "127.0.0.1",
				"--port", fmt.Sprintf("%d", pmcpPort),
			)
			pmcpAddr := fmt.Sprintf("http://127.0.0.1:%d", pmcpPort)
			waitForTCP(t, fmt.Sprintf("127.0.0.1:%d", pmcpPort), 10*time.Second)
			initializeHTTP(t, pmcpAddr, "/")

			// FastMCP SSE: full MCP SSE protocol (GET /sse → endpoint → POST → SSE response)
			fastPort := findFreePort(t)
			startFastMCPHTTP(t, fastFixture, "sse", fastPort)
			fastAddr := fmt.Sprintf("http://127.0.0.1:%d", fastPort)
			fastSSE := connectSSE(t, fastAddr, 20*time.Second)
			fastSSE.initialize(t)

			// Warmup
			for i := 0; i < warmup; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
				sendHTTPReqToPath(t, pmcpAddr, "/", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+1)),
					Method: "tools/call", Params: params,
				})
				fastSSE.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
			}

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				start := time.Now()
				sendHTTPReqToPath(t, pmcpAddr, "/", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i+warmup+1)),
					Method: "tools/call", Params: params,
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				_, elapsed := fastSSE.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "bench"},
				})
				fastLats = append(fastLats, elapsed)
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp SSE", ps)
			logStats(t, "FastMCP  SSE", fs)
			logComparison(t, "SSE p50", ps.P50, fs.P50)

			addSummaryDuration("SSE echo p50", ps.P50, fs.P50)
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// D3: CONCURRENT CLIENT COMPARISON (HTTP)
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D3_ConcurrentHTTP", func(t *testing.T) {
		concurrencyLevels := []int{1, 5, 10, 25, 50}
		const callsPerClient = 200

		t.Log("────────────────────────────────────────")
		t.Logf("D3. CONCURRENT HTTP CLIENTS (%d calls/client)", callsPerClient)
		t.Log("    protomcp http vs FastMCP streamable-http")
		t.Log("    Using HTTP keep-alive connection pooling (fair to both)")
		t.Log("────────────────────────────────────────")

		// Helper: POST with pooled client + 10s per-request timeout.
		// Returns error instead of fatal (goroutine-safe).
		pooledPost := func(client *http.Client, addr, path string, req mcp.JSONRPCRequest) (mcp.JSONRPCResponse, error) {
			data, _ := json.Marshal(req)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			httpReq, _ := http.NewRequestWithContext(ctx, "POST", addr+path, bytes.NewReader(data))
			httpReq.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(httpReq)
			if err != nil {
				return mcp.JSONRPCResponse{}, err
			}
			defer resp.Body.Close()
			var jsonResp mcp.JSONRPCResponse
			if err := json.NewDecoder(resp.Body).Decode(&jsonResp); err != nil {
				return mcp.JSONRPCResponse{}, err
			}
			return jsonResp, nil
		}

		// Create a fresh pooled HTTP client per concurrency level.
		// Prevents stale connection state from prior levels from causing timeouts.
		newPooledClient := func(maxConns int) *http.Client {
			return &http.Client{
				Transport: &http.Transport{
					MaxIdleConns:        maxConns,
					MaxIdleConnsPerHost: maxConns,
					IdleConnTimeout:     30 * time.Second,
				},
			}
		}

		pmcpPort := findFreePort(t)
		testutil.StartPMCP(t, "dev", pyFixture,
			"--transport", "http",
			"--host", "127.0.0.1",
			"--port", fmt.Sprintf("%d", pmcpPort),
		)
		pmcpAddr := fmt.Sprintf("http://127.0.0.1:%d", pmcpPort)
		waitForHTTP(t, pmcpAddr, 10*time.Second)
		initializeHTTP(t, pmcpAddr, "/")

		// Start FastMCP streamable-http
		fastPort := findFreePort(t)
		startFastMCPHTTP(t, fastFixture, "streamable-http", fastPort)
		fastAddr := fmt.Sprintf("http://127.0.0.1:%d", fastPort)
		waitForHTTPPost(t, fastAddr, "/mcp", 20*time.Second)
		initializeHTTP(t, fastAddr, "/mcp")

		t.Logf("")
		t.Logf("  %-8s  %-12s  %-12s  %-10s  %-8s  %-12s  %-12s  %-10s  %-8s",
			"Clients", "pmcp p50", "pmcp p99", "pmcp rps", "pmcp err",
			"fast p50", "fast p99", "fast rps", "fast err")
		t.Logf("  %-8s  %-12s  %-12s  %-10s  %-8s  %-12s  %-12s  %-10s  %-8s",
			"───────", "────────", "────────", "────────", "────────",
			"────────", "────────", "────────", "────────")

		var bestPmcpRPS, bestFastRPS float64
		pmcpReqID := 1000
		fastReqID := 1000

		for _, concurrency := range concurrencyLevels {
			// Connection pools per framework.
			pmcpClient := newPooledClient(concurrency * 4)
			fastClient := newPooledClient(concurrency * 4)

			// Warmup
			for i := 0; i < 50; i++ {
				params, _ := json.Marshal(map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
				pooledPost(pmcpClient, pmcpAddr, "/", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i)),
					Method: "tools/call", Params: params,
				})
				pooledPost(fastClient, fastAddr, "/mcp", mcp.JSONRPCRequest{
					JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", i)),
					Method: "tools/call", Params: params,
				})
			}

			// --- protomcp concurrent ---
			var pmcpMu sync.Mutex
			pmcpAllLats := make([]time.Duration, 0, concurrency*callsPerClient)
			pmcpErrors := 0
			var pmcpWg sync.WaitGroup

			pmcpStart := time.Now()
			for c := 0; c < concurrency; c++ {
				pmcpWg.Add(1)
				go func() {
					defer pmcpWg.Done()
					localLats := make([]time.Duration, 0, callsPerClient)
					localErrors := 0
					for i := 0; i < callsPerClient; i++ {
						pmcpMu.Lock()
						pmcpReqID++
						id := pmcpReqID
						pmcpMu.Unlock()

						params, _ := json.Marshal(map[string]interface{}{
							"name": "echo", "arguments": map[string]string{"message": "bench"},
						})
						start := time.Now()
						_, err := pooledPost(pmcpClient, pmcpAddr, "/", mcp.JSONRPCRequest{
							JSONRPC: "2.0",
							ID:      json.RawMessage(fmt.Sprintf("%d", id)),
							Method:  "tools/call",
							Params:  params,
						})
						if err != nil {
							localErrors++
							continue
						}
						localLats = append(localLats, time.Since(start))
					}
					pmcpMu.Lock()
					pmcpAllLats = append(pmcpAllLats, localLats...)
					pmcpErrors += localErrors
					pmcpMu.Unlock()
				}()
			}
			pmcpWg.Wait()
			pmcpElapsed := time.Since(pmcpStart)
			pmcpTotalRPS := float64(len(pmcpAllLats)) / pmcpElapsed.Seconds()
			if pmcpTotalRPS > bestPmcpRPS {
				bestPmcpRPS = pmcpTotalRPS
			}

			// --- FastMCP concurrent ---
			var fastMu sync.Mutex
			fastAllLats := make([]time.Duration, 0, concurrency*callsPerClient)
			fastErrors := 0
			var fastWg sync.WaitGroup

			fastStart := time.Now()
			for c := 0; c < concurrency; c++ {
				fastWg.Add(1)
				go func() {
					defer fastWg.Done()
					localLats := make([]time.Duration, 0, callsPerClient)
					localErrors := 0
					for i := 0; i < callsPerClient; i++ {
						fastMu.Lock()
						fastReqID++
						id := fastReqID
						fastMu.Unlock()

						params, _ := json.Marshal(map[string]interface{}{
							"name": "echo", "arguments": map[string]string{"message": "bench"},
						})
						start := time.Now()
						_, err := pooledPost(fastClient, fastAddr, "/mcp", mcp.JSONRPCRequest{
							JSONRPC: "2.0",
							ID:      json.RawMessage(fmt.Sprintf("%d", id)),
							Method:  "tools/call",
							Params:  params,
						})
						if err != nil {
							localErrors++
							continue
						}
						localLats = append(localLats, time.Since(start))
					}
					fastMu.Lock()
					fastAllLats = append(fastAllLats, localLats...)
					fastErrors += localErrors
					fastMu.Unlock()
				}()
			}
			fastWg.Wait()
			fastElapsed := time.Since(fastStart)
			fastTotalRPS := float64(len(fastAllLats)) / fastElapsed.Seconds()
			if fastTotalRPS > bestFastRPS {
				bestFastRPS = fastTotalRPS
			}

			pmcpErrStr := fmt.Sprintf("%d", pmcpErrors)
			fastErrStr := fmt.Sprintf("%d", fastErrors)

			if len(pmcpAllLats) > 0 && len(fastAllLats) > 0 {
				ps := computeStats(pmcpAllLats)
				fs := computeStats(fastAllLats)
				t.Logf("  %-8d  %-12v  %-12v  %-10.0f  %-8s  %-12v  %-12v  %-10.0f  %-8s",
					concurrency, ps.P50, ps.P99, pmcpTotalRPS, pmcpErrStr,
					fs.P50, fs.P99, fastTotalRPS, fastErrStr)
			} else if len(pmcpAllLats) > 0 {
				ps := computeStats(pmcpAllLats)
				t.Logf("  %-8d  %-12v  %-12v  %-10.0f  %-8s  %-12s  %-12s  %-10s  %-8s",
					concurrency, ps.P50, ps.P99, pmcpTotalRPS, pmcpErrStr,
					"FAILED", "FAILED", "0", fastErrStr)
			} else {
				t.Logf("  %-8d  %-12s  %-12s  %-10s  %-8s  %-12s  %-12s  %-10s  %-8s",
					concurrency, "FAILED", "FAILED", "0", pmcpErrStr,
					"FAILED", "FAILED", "0", fastErrStr)
			}
		}

		addSummaryFloat("Peak concurrent rps", bestPmcpRPS, bestFastRPS, "rps", false)
	})

	// ═══════════════════════════════════════════════════════════════════
	// D4: PAYLOAD SCALING HEAD-TO-HEAD
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D4_PayloadScaling", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("D4. PAYLOAD SCALING HEAD-TO-HEAD (stdio)")
		t.Log("    Echo with increasing payload sizes")
		t.Log("────────────────────────────────────────")

		pmcpProc := testutil.StartPMCP(t, "dev", pyFixture)
		pmcpProc.Initialize(t)
		fastProc := startStdioProcess(t, "python3", fastFixture)
		fastProc.initialize(t)

		// Warmup
		for i := 0; i < 50; i++ {
			pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
		}

		payloads := []struct {
			label string
			size  int
		}{
			{"100B", 100},
			{"1KB", 1024},
			{"10KB", 10240},
			{"100KB", 102400},
			{"500KB", 512000},
		}

		const callsPerSize = 200

		t.Logf("")
		t.Logf("  %-8s  %-14s  %-14s  %-14s  %-14s  %-10s",
			"Size", "pmcp p50", "pmcp p99", "fast p50", "fast p99", "Speedup")
		t.Logf("  %-8s  %-14s  %-14s  %-14s  %-14s  %-10s",
			"────────", "────────", "────────", "────────", "────────", "───────")

		for _, pl := range payloads {
			msg := strings.Repeat("X", pl.size)

			pmcpLats := make([]time.Duration, 0, callsPerSize)
			for i := 0; i < callsPerSize; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": msg},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, callsPerSize)
			for i := 0; i < callsPerSize; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": msg},
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			speedup := float64(fs.P50) / float64(ps.P50)
			t.Logf("  %-8s  %-14v  %-14v  %-14v  %-14v  %.1fx",
				pl.label, ps.P50, ps.P99, fs.P50, fs.P99, speedup)

			if pl.label == "1KB" {
				addSummaryDuration("1KB payload p50", ps.P50, fs.P50)
			}
			if pl.label == "100KB" {
				addSummaryDuration("100KB payload p50", ps.P50, fs.P50)
			}
		}
	})

	// ═══════════════════════════════════════════════════════════════════
	// D5: COMPLEX TOOL WORKLOADS HEAD-TO-HEAD
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D5_ComplexWorkloads", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("D5. COMPLEX TOOL WORKLOADS HEAD-TO-HEAD (stdio)")
		t.Log("    CPU-bound, JSON, large response, mixed workflow")
		t.Log("────────────────────────────────────────")

		pmcpProc := testutil.StartPMCP(t, "dev", pyFixture)
		pmcpProc.Initialize(t)
		fastProc := startStdioProcess(t, "python3", fastFixture)
		fastProc.initialize(t)

		// Warmup
		for i := 0; i < 50; i++ {
			pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
		}

		const n = 200

		// --- D5a: CPU-bound ---
		t.Run("D5a_CPUBound", func(t *testing.T) {
			t.Log("")
			t.Log("  D5a. CPU-BOUND: compute(iterations=1000)")

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "compute", "arguments": map[string]interface{}{"iterations": 1000},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "compute", "arguments": map[string]interface{}{"iterations": 1000},
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp compute", ps)
			logStats(t, "FastMCP  compute", fs)
			logComparison(t, "CPU-bound p50", ps.P50, fs.P50)

			addSummaryDuration("CPU-bound p50", ps.P50, fs.P50)
		})

		// --- D5b: JSON serialization ---
		t.Run("D5b_JSONSerialization", func(t *testing.T) {
			t.Log("")
			t.Log("  D5b. JSON SERIALIZATION: parse_json with nested object")

			nestedJSON := `{"user":{"name":"Alice","age":30,"address":{"city":"NYC","zip":"10001","coords":{"lat":40.7128,"lng":-74.006}},"tags":["admin","user","beta"],"settings":{"theme":"dark","notifications":true,"limits":{"maxItems":100,"timeout":30}}}}`

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "parse_json", "arguments": map[string]string{"data": nestedJSON},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "parse_json", "arguments": map[string]string{"data": nestedJSON},
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp parse_json", ps)
			logStats(t, "FastMCP  parse_json", fs)
			logComparison(t, "JSON p50", ps.P50, fs.P50)

			addSummaryDuration("JSON parse p50", ps.P50, fs.P50)
		})

		// --- D5c: Large response ---
		t.Run("D5c_LargeResponse", func(t *testing.T) {
			t.Log("")
			t.Log("  D5c. LARGE RESPONSE: generate(size=10000)")

			pmcpLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "generate", "arguments": map[string]interface{}{"size": 10000},
				})
				pmcpLats = append(pmcpLats, time.Since(start))
			}

			fastLats := make([]time.Duration, 0, n)
			for i := 0; i < n; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "generate", "arguments": map[string]interface{}{"size": 10000},
				})
				fastLats = append(fastLats, time.Since(start))
			}

			ps := computeStats(pmcpLats)
			fs := computeStats(fastLats)
			logStats(t, "protomcp generate", ps)
			logStats(t, "FastMCP  generate", fs)
			logComparison(t, "large response p50", ps.P50, fs.P50)
		})

		// --- D5d: Mixed workflow ---
		t.Run("D5d_MixedWorkflow", func(t *testing.T) {
			t.Log("")
			t.Log("  D5d. MIXED WORKFLOW: rotate 5 tools, 1000 calls each")

			tools := []struct {
				name string
				args map[string]interface{}
			}{
				{"echo", map[string]interface{}{"message": "hello"}},
				{"add", map[string]interface{}{"a": 42, "b": 17}},
				{"compute", map[string]interface{}{"iterations": 10}},
				{"generate", map[string]interface{}{"size": 100}},
				{"parse_json", map[string]interface{}{"data": `{"key":"value","n":42}`}},
			}

			const totalCalls = 1000

			pmcpByTool := make(map[string][]time.Duration)
			fastByTool := make(map[string][]time.Duration)
			for _, tool := range tools {
				pmcpByTool[tool.name] = make([]time.Duration, 0, totalCalls/len(tools))
				fastByTool[tool.name] = make([]time.Duration, 0, totalCalls/len(tools))
			}

			// protomcp mixed calls
			for i := 0; i < totalCalls; i++ {
				tool := tools[i%len(tools)]
				start := time.Now()
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": tool.name, "arguments": tool.args,
				})
				pmcpByTool[tool.name] = append(pmcpByTool[tool.name], time.Since(start))
			}

			// FastMCP mixed calls
			for i := 0; i < totalCalls; i++ {
				tool := tools[i%len(tools)]
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": tool.name, "arguments": tool.args,
				})
				fastByTool[tool.name] = append(fastByTool[tool.name], time.Since(start))
			}

			t.Logf("")
			t.Logf("  %-14s  %-14s  %-14s  %-14s  %-14s  %-10s",
				"Tool", "pmcp p50", "pmcp p99", "fast p50", "fast p99", "Speedup")
			t.Logf("  %-14s  %-14s  %-14s  %-14s  %-14s  %-10s",
				"──────────────", "──────────────", "──────────────", "──────────────", "──────────────", "──────────")

			for _, tool := range tools {
				ps := computeStats(pmcpByTool[tool.name])
				fs := computeStats(fastByTool[tool.name])
				speedup := float64(fs.P50) / float64(ps.P50)
				t.Logf("  %-14s  %-14v  %-14v  %-14v  %-14v  %.1fx",
					tool.name, ps.P50, ps.P99, fs.P50, fs.P99, speedup)
			}

			// Overall mixed workflow stats
			pmcpAll := make([]time.Duration, 0, totalCalls)
			fastAll := make([]time.Duration, 0, totalCalls)
			for _, tool := range tools {
				pmcpAll = append(pmcpAll, pmcpByTool[tool.name]...)
				fastAll = append(fastAll, fastByTool[tool.name]...)
			}
			ps := computeStats(pmcpAll)
			fs := computeStats(fastAll)
			t.Logf("")
			logComparison(t, "mixed workflow p50", ps.P50, fs.P50)

			addSummaryDuration("Mixed workflow p50", ps.P50, fs.P50)
		})
	})

	// ═══════════════════════════════════════════════════════════════════
	// D6: RESOURCE EFFICIENCY UNDER SUSTAINED LOAD
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D6_ResourceEfficiency", func(t *testing.T) {
		t.Log("────────────────────────────────────────")
		t.Log("D6. RESOURCE EFFICIENCY UNDER SUSTAINED LOAD (stdio)")
		t.Log("    10K calls, RSS tracked at checkpoints")
		t.Log("────────────────────────────────────────")

		pmcpProc := testutil.StartPMCP(t, "dev", pyFixture)
		pmcpProc.Initialize(t)
		fastProc := startStdioProcess(t, "python3", fastFixture)
		fastProc.initialize(t)

		pmcpPid := pmcpProc.Cmd.Process.Pid
		fastPid := fastProc.pid

		// Warmup
		for i := 0; i < 200; i++ {
			pmcpProc.Send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
		}

		const totalCalls = 10000
		const checkpoints = 5
		const callsPerCheckpoint = totalCalls / checkpoints

		pmcpRSS0, _ := getRSS(pmcpPid)
		fastRSS0, _ := getRSS(fastPid)

		t.Logf("")
		t.Logf("  %-14s  %-14s  %-14s", "After calls", "protomcp RSS", "FastMCP RSS")
		t.Logf("  %-14s  %-14s  %-14s", "───────────", "────────────", "───────────")
		t.Logf("  %-14s  %-14s  %-14s", "baseline",
			fmt.Sprintf("%d KB", pmcpRSS0), fmt.Sprintf("%d KB", fastRSS0))

		var pmcpFinalRSS, fastFinalRSS int64

		// protomcp sustained load
		pmcpStart := time.Now()
		for cp := 1; cp <= checkpoints; cp++ {
			for i := 0; i < callsPerCheckpoint; i++ {
				pmcpProc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "x"},
				})
			}
			pmcpRSS, _ := getRSS(pmcpPid)
			pmcpFinalRSS = pmcpRSS
			fastRSS, _ := getRSS(fastPid)
			_ = fastRSS // just protomcp running, don't log yet
		}
		pmcpElapsed := time.Since(pmcpStart)

		// FastMCP sustained load
		fastStart := time.Now()
		for cp := 1; cp <= checkpoints; cp++ {
			for i := 0; i < callsPerCheckpoint; i++ {
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "x"},
				})
			}
			pmcpRSS, _ := getRSS(pmcpPid)
			fastRSS, _ := getRSS(fastPid)
			fastFinalRSS = fastRSS
			t.Logf("  %-14d  %-14s  %-14s",
				cp*callsPerCheckpoint,
				fmt.Sprintf("%d KB", pmcpRSS),
				fmt.Sprintf("%d KB", fastRSS))
		}
		fastElapsed := time.Since(fastStart)

		pmcpRPS := float64(totalCalls) / pmcpElapsed.Seconds()
		fastRPS := float64(totalCalls) / fastElapsed.Seconds()

		pmcpEfficiency := float64(totalCalls) / (float64(pmcpFinalRSS) / 1024.0) // calls per MB
		fastEfficiency := float64(totalCalls) / (float64(fastFinalRSS) / 1024.0)

		t.Logf("")
		t.Logf("  Memory growth:")
		t.Logf("    protomcp: %d KB → %d KB (+%d KB, %.1f%%)",
			pmcpRSS0, pmcpFinalRSS, pmcpFinalRSS-pmcpRSS0,
			100.0*float64(pmcpFinalRSS-pmcpRSS0)/float64(pmcpRSS0))
		t.Logf("    FastMCP:  %d KB → %d KB (+%d KB, %.1f%%)",
			fastRSS0, fastFinalRSS, fastFinalRSS-fastRSS0,
			100.0*float64(fastFinalRSS-fastRSS0)/float64(fastRSS0))
		t.Logf("")
		t.Logf("  Throughput:")
		t.Logf("    protomcp: %.0f rps (%v for %d calls)", pmcpRPS, pmcpElapsed.Round(time.Millisecond), totalCalls)
		t.Logf("    FastMCP:  %.0f rps (%v for %d calls)", fastRPS, fastElapsed.Round(time.Millisecond), totalCalls)
		t.Logf("")
		t.Logf("  Efficiency (calls per MB of RSS):")
		t.Logf("    protomcp: %.0f calls/MB", pmcpEfficiency)
		t.Logf("    FastMCP:  %.0f calls/MB", fastEfficiency)

		addSummaryFloat("Sustained rps", pmcpRPS, fastRPS, "rps", false)
		addSummaryFloat("Server RSS", float64(pmcpFinalRSS), float64(fastFinalRSS), "KB", true)
		addSummaryFloat("Efficiency", pmcpEfficiency, fastEfficiency, "calls/MB", false)
	})

	// ═══════════════════════════════════════════════════════════════════
	// D7: RAW SIDEBAND PAYLOAD BENCHMARK
	// Full matrix: every SDK (Python, TypeScript, Go) × payload sizes (1KB–10MB)
	// All results compared head-to-head against FastMCP as the primary metric.
	// ═══════════════════════════════════════════════════════════════════
	t.Run("D7_RawSidebandPayload", func(t *testing.T) {
		t.Log("────────────────────────────────────────────────────────────────────")
		t.Log("D7. RAW SIDEBAND PAYLOAD BENCHMARK")
		t.Log("    Every SDK × every payload size × vs FastMCP")
		t.Log("    Exercises: SDK raw sideband transfer → Go readLoop → JSON passthrough")
		t.Log("────────────────────────────────────────────────────────────────────")

		// Set chunk threshold low so raw sideband kicks in for 10KB+.
		t.Setenv("PROTOMCP_CHUNK_THRESHOLD", "8192")

		pySDKFixture := testutil.FixturePath("tests/bench/fixtures/sdk_echo_tool.py")
		tsFixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.ts")
		goBinary := testutil.FixturePath("tests/bench/fixtures/go_bench_tool")
		goSrcDir := testutil.FixturePath("tests/bench/fixtures/go")

		// Pre-build Go fixture if needed
		if _, err := os.Stat(goBinary); os.IsNotExist(err) {
			t.Log("  Building Go fixture binary...")
			buildCmd := exec.Command("go", "build", "-o", goBinary, ".")
			buildCmd.Dir = goSrcDir
			if out, err := buildCmd.CombinedOutput(); err != nil {
				t.Fatalf("Go fixture build failed: %v\n%s", err, out)
			}
		}

		type sdkEntry struct {
			name    string
			proc    *testutil.StdioPMCP
			skip    bool
		}

		// Start all SDK processes
		pyProc := testutil.StartPMCP(t, "dev", pySDKFixture)
		pyProc.Initialize(t)

		var tsProc *testutil.StdioPMCP
		tsSkip := false
		if _, err := exec.LookPath("npx"); err != nil {
			t.Log("  Skipping TypeScript: npx not installed")
			tsSkip = true
		} else {
			tsProc = testutil.StartPMCP(t, "dev", tsFixture)
			tsProc.Initialize(t)
		}

		goProc := testutil.StartPMCP(t, "dev", goBinary)
		goProc.Initialize(t)

		// Start FastMCP
		fastProc := startStdioProcess(t, "python3", fastFixture)
		fastProc.initialize(t)

		sdks := []sdkEntry{
			{"Python", pyProc, false},
			{"TypeScript", tsProc, tsSkip},
			{"Go", goProc, false},
		}

		// Warmup all processes
		for i := 0; i < 50; i++ {
			for _, sdk := range sdks {
				if sdk.skip {
					continue
				}
				sdk.proc.Send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": "w"},
				})
			}
			fastProc.send(t, "tools/call", map[string]interface{}{
				"name": "echo", "arguments": map[string]string{"message": "w"},
			})
		}

		payloads := []struct {
			label string
			size  int
		}{
			{"1KB", 1024},
			{"10KB", 10240},
			{"50KB", 51200},
			{"100KB", 102400},
			{"500KB", 512000},
			{"1MB", 1048576},
			{"5MB", 5242880},
			{"10MB", 10485760},
		}

		const callsPerSize = 50

		// ── Per-SDK results table ──
		for _, sdk := range sdks {
			if sdk.skip {
				continue
			}

			t.Logf("")
			t.Logf("  ── %s SDK vs FastMCP ──", sdk.name)
			t.Logf("  %-8s  %-16s  %-16s  %-20s",
				"Size", sdk.name+" p50", "FastMCP p50", "vs FastMCP")
			t.Logf("  %-8s  %-16s  %-16s  %-20s",
				"────────", "────────────────", "────────────────", "────────────────────")

			for _, pl := range payloads {
				msg := strings.Repeat("X", pl.size)

				// SDK with raw sideband — use generate tool
				sdkLats := make([]time.Duration, 0, callsPerSize)
				for i := 0; i < callsPerSize; i++ {
					start := time.Now()
					sdk.proc.Send(t, "tools/call", map[string]interface{}{
						"name": "generate", "arguments": map[string]int{"size": pl.size},
					})
					sdkLats = append(sdkLats, time.Since(start))
				}

				// FastMCP
				fastLats := make([]time.Duration, 0, callsPerSize)
				for i := 0; i < callsPerSize; i++ {
					start := time.Now()
					fastProc.send(t, "tools/call", map[string]interface{}{
						"name": "echo", "arguments": map[string]string{"message": msg},
					})
					fastLats = append(fastLats, time.Since(start))
				}

				ss := computeStats(sdkLats)
				fs := computeStats(fastLats)

				ratio := float64(fs.P50) / float64(ss.P50)
				var vsStr string
				if ratio > 1.0 {
					vsStr = fmt.Sprintf("protomcp %.1fx faster", ratio)
				} else if ratio < 1.0 {
					vsStr = fmt.Sprintf("FastMCP %.1fx faster", 1.0/ratio)
				} else {
					vsStr = "tie"
				}

				t.Logf("  %-8s  %-16v  %-16v  %s",
					pl.label, ss.P50, fs.P50, vsStr)

				// Add key sizes to grand summary (Python only to avoid duplication)
				if sdk.name == "Python" {
					switch pl.label {
					case "100KB":
						addSummaryDuration("100KB sideband p50", ss.P50, fs.P50)
					case "1MB":
						addSummaryDuration("1MB sideband p50", ss.P50, fs.P50)
					case "5MB":
						addSummaryDuration("5MB sideband p50", ss.P50, fs.P50)
					case "10MB":
						addSummaryDuration("10MB sideband p50", ss.P50, fs.P50)
					}
				}
			}
		}

		// ── Cross-SDK comparison at key sizes ──
		t.Logf("")
		t.Logf("  ── Cross-SDK Comparison (p50 latency) ──")

		keySizes := []struct {
			label string
			size  int
		}{
			{"100KB", 102400},
			{"500KB", 512000},
			{"1MB", 1048576},
			{"5MB", 5242880},
			{"10MB", 10485760},
		}

		// Build header
		hdrCols := []string{"Size"}
		for _, sdk := range sdks {
			if sdk.skip {
				continue
			}
			hdrCols = append(hdrCols, sdk.name)
		}
		hdrCols = append(hdrCols, "FastMCP")
		hdrFmt := "  "
		divFmt := "  "
		for range hdrCols {
			hdrFmt += "%-16s"
			divFmt += "%-16s"
		}
		hdrVals := make([]interface{}, len(hdrCols))
		divVals := make([]interface{}, len(hdrCols))
		for i, h := range hdrCols {
			hdrVals[i] = h
			divVals[i] = "────────────────"
		}
		t.Logf(hdrFmt, hdrVals...)
		t.Logf(divFmt, divVals...)

		for _, ks := range keySizes {
			msg := strings.Repeat("X", ks.size)

			rowVals := make([]interface{}, 0, len(hdrCols))
			rowVals = append(rowVals, ks.label)

			for _, sdk := range sdks {
				if sdk.skip {
					continue
				}
				lats := make([]time.Duration, 0, callsPerSize)
				for i := 0; i < callsPerSize; i++ {
					start := time.Now()
					sdk.proc.Send(t, "tools/call", map[string]interface{}{
						"name": "generate", "arguments": map[string]int{"size": ks.size},
					})
					lats = append(lats, time.Since(start))
				}
				s := computeStats(lats)
				rowVals = append(rowVals, s.P50.String())
			}

			// FastMCP
			fastLats := make([]time.Duration, 0, callsPerSize)
			for i := 0; i < callsPerSize; i++ {
				start := time.Now()
				fastProc.send(t, "tools/call", map[string]interface{}{
					"name": "echo", "arguments": map[string]string{"message": msg},
				})
				fastLats = append(fastLats, time.Since(start))
			}
			fs := computeStats(fastLats)
			rowVals = append(rowVals, fs.P50.String())

			t.Logf(hdrFmt, rowVals...)
		}

		// ── Memory comparison ──
		t.Logf("")
		t.Logf("  ── Memory (RSS in KB) after payload tests ──")
		for _, sdk := range sdks {
			if sdk.skip {
				continue
			}
			rss, _ := getRSS(sdk.proc.Cmd.Process.Pid)
			t.Logf("    %-12s %d KB", sdk.name+":", rss)
		}
		fastRSS, _ := getRSS(fastProc.pid)
		t.Logf("    %-12s %d KB", "FastMCP:", fastRSS)
	})

	// ═══════════════════════════════════════════════════════════════════
	// GRAND SUMMARY TABLE
	// ═══════════════════════════════════════════════════════════════════
	t.Log("")
	t.Log("╔══════════════════════════════════════════════════════════════════╗")
	t.Log("║           DEEP COMPARISON GRAND SUMMARY                        ║")
	t.Log("╚══════════════════════════════════════════════════════════════════╝")
	t.Log("")

	t.Logf("  %-28s  %-18s  %-18s  %-16s", "Metric", "protomcp", "FastMCP", "Winner")
	t.Logf("  %-28s  %-18s  %-18s  %-16s",
		"────────────────────────────", "──────────────────", "──────────────────", "────────────────")

	for _, s := range summary {
		t.Logf("  %-28s  %-18s  %-18s  %s", s.metric, s.pmcpVal, s.fastVal, s.winner)
	}
}
