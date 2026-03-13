package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

func init() {
	// Ensure the Python SDK is importable by setting PYTHONPATH.
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

func TestE2E_Initialize(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", "fixtures/simple_tool.py")
	defer cleanup()

	resp := SendRequest(t, w, r, "initialize", nil)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}

	var result mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal InitializeResult: %v", err)
	}
	if result.Capabilities.Tools == nil || !result.Capabilities.Tools.ListChanged {
		t.Error("expected tools.listChanged = true")
	}
}

func TestE2E_ToolsList(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", "fixtures/simple_tool.py")
	defer cleanup()

	SendRequest(t, w, r, "initialize", nil)
	resp := SendRequest(t, w, r, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}

	var result mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal ToolsListResult: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
}

func TestE2E_ToolsCall(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", "fixtures/simple_tool.py")
	defer cleanup()

	SendRequest(t, w, r, "initialize", nil)
	resp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]string{"message": "hello"},
	})

	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}

	var result mcp.ToolsCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal ToolsCallResult: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool call returned error")
	}
}

func TestE2E_DynamicToolList(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", "fixtures/dynamic_tool.py")
	defer cleanup()

	SendRequest(t, w, r, "initialize", nil)

	// Both tools start visible; call auth with valid token to enable admin_action
	// and disable auth.
	resp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "auth",
		"arguments": map[string]string{"token": "valid"},
	})
	if resp.Error != nil {
		t.Fatalf("auth call error: %v", resp.Error)
	}

	listResp := SendRequest(t, w, r, "tools/list", nil)
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error)
	}
	var result mcp.ToolsListResult
	if err := json.Unmarshal(listResp.Result, &result); err != nil {
		t.Fatalf("unmarshal ToolsListResult: %v", err)
	}

	found := false
	for _, tool := range result.Tools {
		if tool.Name == "admin_action" {
			found = true
		}
	}
	if !found {
		t.Error("admin_action should be visible after auth")
	}
}
