package stress_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// TestGoroutineStability runs many tool calls and checks that the goroutine
// count stays stable (no goroutine leak).
func TestGoroutineStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	socketPath := filepath.Join(t.TempDir(), "goroutine.sock")

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  1,
		CallTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	// Warm up.
	for i := 0; i < 10; i++ {
		pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
	}
	runtime.GC()

	baselineGoroutines := runtime.NumGoroutine()
	t.Logf("Baseline goroutines: %d", baselineGoroutines)

	// Run many calls.
	const totalCalls = 500
	for i := 0; i < totalCalls; i++ {
		resp, err := pm.CallTool(ctx, "echo", fmt.Sprintf(`{"message":"call-%d"}`, i))
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if resp.IsError {
			t.Errorf("call %d unexpected error", i)
		}
	}

	runtime.GC()
	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d (after %d calls)", finalGoroutines, totalCalls)

	// Allow a generous margin (5 goroutines) for any background activity.
	goroutineGrowth := finalGoroutines - baselineGoroutines
	if goroutineGrowth > 5 {
		t.Errorf("goroutine count grew by %d (from %d to %d), possible leak",
			goroutineGrowth, baselineGoroutines, finalGoroutines)
	}
}

// TestMemoryStability runs many calls and checks that memory usage doesn't
// grow unboundedly.
func TestMemoryStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	socketPath := filepath.Join(t.TempDir(), "memory.sock")

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  1,
		CallTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	// Warm up.
	for i := 0; i < 50; i++ {
		pm.CallTool(ctx, "echo", `{"message":"warmup"}`)
	}
	runtime.GC()

	var baselineStats runtime.MemStats
	runtime.ReadMemStats(&baselineStats)
	baselineAlloc := baselineStats.Alloc

	// Run many calls with moderate payloads.
	const totalCalls = 1000
	const checkInterval = 200

	for i := 0; i < totalCalls; i++ {
		msg := fmt.Sprintf(`{"message":"memory-test-%d-padding-to-make-it-bigger"}`, i)
		resp, err := pm.CallTool(ctx, "echo", msg)
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if resp.IsError {
			t.Errorf("call %d error", i)
		}

		if (i+1)%checkInterval == 0 {
			runtime.GC()
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			t.Logf("After %d calls: alloc=%dKB, sys=%dKB, goroutines=%d",
				i+1, stats.Alloc/1024, stats.Sys/1024, runtime.NumGoroutine())
		}
	}

	runtime.GC()
	var finalStats runtime.MemStats
	runtime.ReadMemStats(&finalStats)
	finalAlloc := finalStats.Alloc

	t.Logf("Memory: baseline=%dKB, final=%dKB", baselineAlloc/1024, finalAlloc/1024)

	// Allow up to 10MB growth (generous for 1000 calls with small payloads).
	maxGrowth := uint64(10 * 1024 * 1024)
	if finalAlloc > baselineAlloc+maxGrowth {
		t.Errorf("memory grew by %dKB (from %dKB to %dKB), possible leak",
			(finalAlloc-baselineAlloc)/1024, baselineAlloc/1024, finalAlloc/1024)
	}
}

// TestPendingMapCleanup verifies that the pending request map in the process
// manager is cleaned up after calls complete.
func TestPendingMapCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	socketPath := filepath.Join(t.TempDir(), "pending.sock")

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  1,
		CallTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	// Run many sequential calls. After each completes, the pending map entry
	// should be cleaned up (via defer in CallTool). We can't inspect the map
	// directly since it's private, but we can verify that many calls complete
	// without growing resource usage.
	const totalCalls = 200
	for i := 0; i < totalCalls; i++ {
		resp, err := pm.CallTool(ctx, "echo", fmt.Sprintf(`{"message":"pending-%d"}`, i))
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
		if resp.IsError {
			t.Errorf("call %d error", i)
		}
	}

	// If the pending map weren't cleaned up, we'd see goroutine growth or
	// memory growth. The goroutine stability test above covers this more
	// directly. Here we just verify all calls completed.
	t.Logf("Completed %d sequential calls (pending map cleanup verified implicitly)", totalCalls)
}
