package testengine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	pythonPath := filepath.Join(repoRoot, "sdk", "python", "src") +
		string(os.PathListSeparator) +
		filepath.Join(repoRoot, "sdk", "python", "gen")
	existing := os.Getenv("PYTHONPATH")
	if existing != "" {
		pythonPath = pythonPath + string(os.PathListSeparator) + existing
	}
	os.Setenv("PYTHONPATH", pythonPath)
}

func fixtureFile() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "test", "e2e", "fixtures", "simple_tool.py")
}

func TestEngineListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	eng := New(fixtureFile(), WithCallTimeout(15*time.Second))
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	defer eng.Stop()

	tools, err := eng.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) < 1 {
		t.Fatalf("expected at least 1 tool, got %d", len(tools))
	}

	// Verify trace has entries from the MCP initialization + list tools call
	entries := eng.Trace().Entries()
	if len(entries) == 0 {
		t.Error("expected trace entries, got none")
	}
}

func TestEngineCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	eng := New(fixtureFile(), WithCallTimeout(15*time.Second))
	if err := eng.Start(ctx); err != nil {
		t.Fatalf("engine start: %v", err)
	}
	defer eng.Stop()

	result, err := eng.CallTool(ctx, "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Duration <= 0 {
		t.Error("expected duration > 0")
	}

	text := toolResultText(result.Result)
	if text == "" {
		t.Error("expected non-empty text result")
	}
}
