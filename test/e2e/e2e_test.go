package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/msilverblatt/protomcp/tests/testutil"
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

func fixture(name string) string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "fixtures", name)
}

func TestE2E_Initialize(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("simple_tool.py"))
	defer cleanup()

	resp := InitializeSession(t, w, r)

	var result testutil.InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal InitializeResult: %v", err)
	}
	if result.Capabilities.Tools == nil || !result.Capabilities.Tools.ListChanged {
		t.Error("expected tools.listChanged = true")
	}
}

func TestE2E_ToolsList(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("simple_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)
	resp := SendRequest(t, w, r, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}

	var result testutil.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal ToolsListResult: %v", err)
	}
	if len(result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(result.Tools))
	}
}

func TestE2E_ToolsCall(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("simple_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)
	resp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]string{"message": "hello"},
	})

	if resp.Error != nil {
		t.Fatalf("tools/call error: %v", resp.Error)
	}

	var result testutil.ToolsCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal ToolsCallResult: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool call returned error")
	}
}

func TestE2E_ToolGroupSeparate(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("tool_group_separate.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	resp := SendRequest(t, w, r, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}

	var result testutil.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	if !names["db.query"] {
		t.Error("expected tool 'db.query'")
	}
	if !names["db.insert"] {
		t.Error("expected tool 'db.insert'")
	}
	if names["db"] {
		t.Error("should NOT have a single 'db' tool in separate strategy")
	}

	callResp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "db.query",
		"arguments": map[string]string{"sql": "SELECT * FROM users"},
	})
	if callResp.Error != nil {
		t.Fatalf("tools/call error: %v", callResp.Error)
	}
	var callResult testutil.ToolsCallResult
	if err := json.Unmarshal(callResp.Result, &callResult); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if callResult.IsError {
		t.Error("tool call should not be an error")
	}
}

func TestE2E_DynamicToolList(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("dynamic_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)

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
	var result testutil.ToolsListResult
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
