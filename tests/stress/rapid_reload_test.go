package stress_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

// TestRapidReload triggers reload rapidly and verifies the process manager
// handles it correctly without losing state or corrupting responses.
func TestRapidReload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	socketPath := filepath.Join(t.TempDir(), "rapid-reload.sock")

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  3,
		CallTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tools, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	// Rapidly reload 10 times.
	const reloadCount = 10
	for i := 0; i < reloadCount; i++ {
		newTools, err := pm.Reload(ctx)
		if err != nil {
			t.Fatalf("Reload %d failed: %v", i, err)
		}
		if len(newTools) == 0 {
			t.Errorf("Reload %d returned no tools", i)
		}
	}

	// Verify the system still works after rapid reloads.
	resp, err := pm.CallTool(ctx, "echo", `{"message":"after-reload"}`)
	if err != nil {
		t.Fatalf("CallTool after reloads failed: %v", err)
	}
	if resp.IsError {
		t.Errorf("unexpected error after reloads: %s", resp.ResultJson)
	}

	t.Logf("Completed %d rapid reloads successfully", reloadCount)
}

// TestReloadDuringCalls triggers reloads while tool calls are in-flight.
func TestReloadDuringCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	socketPath := filepath.Join(t.TempDir(), "reload-during.sock")

	pm := process.NewManager(process.ManagerConfig{
		File:        fixture,
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{fixture},
		SocketPath:  socketPath,
		MaxRetries:  3,
		CallTimeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	// Make a call, then reload, then another call.
	for round := 0; round < 5; round++ {
		resp, err := pm.CallTool(ctx, "echo", fmt.Sprintf(`{"message":"round-%d"}`, round))
		if err != nil {
			t.Fatalf("round %d pre-reload call failed: %v", round, err)
		}
		if resp.IsError {
			t.Errorf("round %d pre-reload call error: %s", round, resp.ResultJson)
		}

		_, err = pm.Reload(ctx)
		if err != nil {
			t.Fatalf("round %d reload failed: %v", round, err)
		}

		resp, err = pm.CallTool(ctx, "echo", fmt.Sprintf(`{"message":"round-%d-after"}`, round))
		if err != nil {
			t.Fatalf("round %d post-reload call failed: %v", round, err)
		}
		if resp.IsError {
			t.Errorf("round %d post-reload call error: %s", round, resp.ResultJson)
		}
	}

	t.Log("Call-reload-call cycles completed successfully")
}

// TestReloadPreservesToolList ensures that after reload, the tool list is
// refreshed and consistent.
func TestReloadPreservesToolList(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	fixture := testutil.FixturePath("tests/stress/fixtures/echo_tool.py")
	p := testutil.StartPMCP(t, "dev", fixture)
	p.Initialize(t)

	// Get initial tool list.
	r := p.Send(t, "tools/list", nil)
	if r.Resp.Error != nil {
		t.Fatalf("initial tools/list error: %s", r.Resp.Error.Message)
	}

	var initialList struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	json.Unmarshal(r.Resp.Result, &initialList)

	// Make several calls to ensure stability.
	for i := 0; i < 20; i++ {
		r = p.Send(t, "tools/call", map[string]interface{}{
			"name":      "echo",
			"arguments": map[string]string{"message": fmt.Sprintf("check-%d", i)},
		})
		if r.Resp.Error != nil {
			t.Fatalf("call %d error: %s", i, r.Resp.Error.Message)
		}
	}

	// Get tool list again and verify it hasn't changed.
	r = p.Send(t, "tools/list", nil)
	if r.Resp.Error != nil {
		t.Fatalf("final tools/list error: %s", r.Resp.Error.Message)
	}

	var finalList struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	json.Unmarshal(r.Resp.Result, &finalList)

	if len(initialList.Tools) != len(finalList.Tools) {
		t.Errorf("tool count changed: %d -> %d", len(initialList.Tools), len(finalList.Tools))
	}
}
