package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/process"
)

type pythonBackend struct{ *process.Manager }

func (p pythonBackend) ActiveTools() []*pb.ToolDefinition { return p.Tools() }

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file not created: %s", path)
}

func TestPythonAsyncStructuredCancellationMCP(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PYTHONPATH", filepath.Join(root, "sdk/python/src"))
	// Unix socket paths on macOS have a short maximum length.
	socketDir, err := os.MkdirTemp("", "pmcp-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(socketDir)
	manager := process.NewManager(process.ManagerConfig{
		File: "testdata/async_tools.py", RuntimeCmd: "python3",
		RuntimeArgs: []string{"testdata/async_tools.py"},
		SocketPath:  filepath.Join(socketDir, "s"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := manager.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()
	bridge := New(pythonBackend{manager}, nil, "test")
	bridge.SyncTools()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := bridge.Server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "regression", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "rows"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(string(encoded), strings.Repeat("x", 70000)) {
		t.Fatalf("structured rows missing or truncated: %d bytes, isError=%v", len(encoded), result.IsError)
	}
	if len(result.Content) != 1 {
		t.Fatal("lost text preview")
	}

	marker := filepath.Join(t.TempDir(), "cleanup")
	callCtx, cancelCall := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, err := session.CallTool(callCtx, &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{"marker": marker}})
		done <- err
	}()
	waitForFile(t, marker+".started")
	cancelCall()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled call succeeded")
		}
	case <-ctx.Done():
		t.Fatal("cancellation did not return")
	}
	waitForFile(t, marker)
	result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "ping"})
	if err != nil || result.IsError {
		t.Fatalf("ping after cancellation: %v", err)
	}
}
