package process_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/tests/testutil"
)

func TestChunkedStreamIntegration(t *testing.T) {
	testutil.SetupPythonPath()

	// Set chunk threshold to 1KB to force chunking for moderate payloads.
	os.Setenv("PROTOMCP_CHUNK_THRESHOLD", "1024")
	defer os.Unsetenv("PROTOMCP_CHUNK_THRESHOLD")

	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("pmcp-stream-integ-%d.sock", os.Getpid()))
	defer os.Remove(socketPath)

	// Use the SDK-based simple_tool.py fixture which has an echo tool.
	cfg := process.ManagerConfig{
		File:        testutil.FixturePath("test/e2e/fixtures/simple_tool.py"),
		SocketPath:  socketPath,
		CallTimeout: 10 * time.Second,
	}

	mgr := process.NewManager(cfg)
	ctx := context.Background()
	tools, err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer mgr.Stop()

	if len(tools) == 0 {
		t.Fatal("no tools returned")
	}

	// Call echo with a 10KB message — well above 1KB threshold.
	// The SDK will chunk the result_json.
	largeMessage := strings.Repeat("X", 10*1024)
	args, _ := json.Marshal(map[string]string{"message": largeMessage})
	resp, err := mgr.CallTool(ctx, "echo", string(args))
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}

	if resp.IsError {
		t.Fatalf("tool returned error: %s", resp.ResultJson)
	}

	// Parse and verify the result contains the full message.
	var content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(resp.ResultJson), &content); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
	if content[0].Text != largeMessage {
		t.Errorf("expected %d char message, got %d chars", len(largeMessage), len(content[0].Text))
	}

	t.Logf("Successfully received %d byte result via chunked stream", len(resp.ResultJson))
}
