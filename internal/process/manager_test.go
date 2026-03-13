package process_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
)

func TestStartAndHandshake(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	if len(tools) == 0 {
		t.Fatal("expected at least one tool from handshake")
	}
	if tools[0].Name != "echo" {
		t.Errorf("expected tool name 'echo', got %q", tools[0].Name)
	}
}

func TestCallTool(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	resp, err := pm.CallTool(ctx, "echo", `{"message": "hello"}`)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if resp.IsError {
		t.Errorf("unexpected error: %s", resp.ResultJson)
	}
}

func TestReload(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	pm := process.NewManager(process.ManagerConfig{
		File:        "testdata/echo_tool.py",
		RuntimeCmd:  "python3",
		RuntimeArgs: []string{"testdata/echo_tool.py"},
		SocketPath:  socketPath,
		MaxRetries:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := pm.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer pm.Stop()

	tools, err := pm.Reload(ctx)
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected tools after reload")
	}
}
