package bench_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// BenchmarkStartup measures the time from process.Manager.Start() to the
// first successful tool call.
func BenchmarkStartup(b *testing.B) {
	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")

	for i := 0; i < b.N; i++ {
		socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("startup-b-%d-%d.sock", os.Getpid(), i))
		pm := process.NewManager(process.ManagerConfig{
			File:        fixture,
			RuntimeCmd:  "python3",
			RuntimeArgs: []string{fixture},
			SocketPath:  socketPath,
			MaxRetries:  1,
			CallTimeout: 30 * time.Second,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		start := time.Now()
		_, err := pm.Start(ctx)
		if err != nil {
			cancel()
			b.Fatalf("Start failed: %v", err)
		}

		// First tool call.
		resp, err := pm.CallTool(ctx, "echo", `{"message":"startup"}`)
		elapsed := time.Since(start)
		if err != nil {
			pm.Stop()
			cancel()
			b.Fatalf("CallTool failed: %v", err)
		}
		if resp.IsError {
			pm.Stop()
			cancel()
			b.Fatalf("unexpected error")
		}

		b.ReportMetric(float64(elapsed.Milliseconds()), "ms/startup")

		pm.Stop()
		cancel()
	}
}

// TestStartupTime is a test variant that reports startup time with more detail.
func TestStartupTime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping startup time test in short mode")
	}

	fixture := testutil.FixturePath("tests/bench/fixtures/echo_tool.py")

	const trials = 10
	durations := make([]time.Duration, 0, trials)

	for i := 0; i < trials; i++ {
		socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("startup-t-%d-%d.sock", os.Getpid(), i))
		pm := process.NewManager(process.ManagerConfig{
			File:        fixture,
			RuntimeCmd:  "python3",
			RuntimeArgs: []string{fixture},
			SocketPath:  socketPath,
			MaxRetries:  1,
			CallTimeout: 30 * time.Second,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		start := time.Now()
		_, err := pm.Start(ctx)
		if err != nil {
			cancel()
			t.Fatalf("trial %d Start failed: %v", i, err)
		}

		resp, err := pm.CallTool(ctx, "echo", `{"message":"startup"}`)
		elapsed := time.Since(start)
		if err != nil {
			pm.Stop()
			cancel()
			t.Fatalf("trial %d CallTool failed: %v", i, err)
		}
		if resp.IsError {
			pm.Stop()
			cancel()
			t.Errorf("trial %d unexpected error", i)
		}

		durations = append(durations, elapsed)
		pm.Stop()
		cancel()
	}

	var total time.Duration
	var min, max time.Duration
	min = durations[0]
	for _, d := range durations {
		total += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}
	avg := total / time.Duration(trials)

	t.Logf("Startup time over %d trials:", trials)
	t.Logf("  min:  %v", min)
	t.Logf("  avg:  %v", avg)
	t.Logf("  max:  %v", max)
}
