# Tests

## Prerequisites

1. Build the pmcp binary:
   ```bash
   make build
   ```

2. Ensure Python 3 is available and the generated protobuf code exists:
   ```bash
   make proto
   ```

## Stress Tests

Stress tests are designed to push the system hard and find edge cases. They
spawn real Python tool processes and communicate over unix sockets.

```bash
# Run all stress tests
go test ./tests/stress/... -v -timeout 120s

# Run a specific test
go test ./tests/stress/... -v -run TestConcurrentCalls

# Skip long-running tests
go test ./tests/stress/... -v -short
```

### What's tested

- **concurrent_calls_test.go** — 100+ concurrent tool calls, response integrity, burst patterns
- **malformed_input_test.go** — Garbage JSON, truncated protobuf, oversized messages, mixed valid/invalid
- **rapid_reload_test.go** — Rapid consecutive reloads, reloads during calls, tool list consistency
- **connection_chaos_test.go** — Tool process crash mid-call, tool process hang (timeout), crash detection, start/stop cycling
- **memory_leak_test.go** — Goroutine count stability, memory growth over 1000+ calls, pending map cleanup

## Benchmarks

```bash
# Run all benchmarks
go test ./tests/bench/... -bench=. -benchmem -timeout 120s

# Run specific benchmark
go test ./tests/bench/... -bench=BenchmarkThroughput_ProcessManager -benchmem

# Run benchmarks with more iterations
go test ./tests/bench/... -bench=BenchmarkLatency -benchtime=10s

# Run latency distribution test (not a benchmark, but reports percentiles)
go test ./tests/bench/... -v -run TestLatencyDistribution

# Run startup time test
go test ./tests/bench/... -v -run TestStartupTime
```

### What's measured

- **throughput_bench_test.go** — Requests/sec through process manager, stdio transport, and HTTP transport; payload size scaling (100B to 100KB)
- **latency_bench_test.go** — Per-call latency with p50/p95/p99 percentiles; process manager vs stdio overhead
- **startup_bench_test.go** — Time from Start() to first successful tool call

## Comparison Benchmarks

```bash
# Run comparison (protomcp baseline)
go test ./tests/bench/comparison/... -v -run TestComparisonProtomcpVsDirect

# Run FastMCP comparison (requires FastMCP installed)
go test ./tests/bench/comparison/... -v -run TestComparisonFastMCP
```

To run a full FastMCP comparison manually:
1. Install FastMCP: `pip install fastmcp`
2. Start FastMCP echo server: `python3 tests/bench/comparison/fastmcp_echo.py`
3. Benchmark both with an MCP client

## Fixtures

Python tool scripts in `tests/stress/fixtures/` and `tests/bench/fixtures/`:
- `echo_tool.py` — Simple echo, handles list/call/reload
- `slow_tool.py` — Echo with configurable delay
- `hang_tool.py` — Hangs on call (never responds)
- `crash_on_call_tool.py` — Crashes on first call
