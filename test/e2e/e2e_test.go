package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestE2E_WorkflowBasic(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("workflow_basic.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	// List tools — should see deploy.review (initial step) and status
	resp := SendRequestSkipNotifications(t, w, r, "tools/list", nil)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error)
	}
	var toolsList testutil.ToolsListResult
	json.Unmarshal(resp.Result, &toolsList)

	names := map[string]bool{}
	for _, tool := range toolsList.Tools {
		names[tool.Name] = true
	}
	if !names["status"] {
		t.Error("expected 'status' tool to be visible")
	}
	if !names["deploy.review"] {
		t.Error("expected 'deploy.review' (initial step) to be visible")
	}

	// Call the initial step
	callResp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "deploy.review",
		"arguments": map[string]string{"pr_number": "42"},
	})
	if callResp.Error != nil {
		t.Fatalf("deploy.review call error: %v", callResp.Error)
	}

	// After calling review, the next steps (approve, reject) should become available
	time.Sleep(200 * time.Millisecond)

	listResp2 := SendRequestSkipNotifications(t, w, r, "tools/list", nil)
	var toolsList2 testutil.ToolsListResult
	json.Unmarshal(listResp2.Result, &toolsList2)

	names2 := map[string]bool{}
	for _, tool := range toolsList2.Tools {
		names2[tool.Name] = true
	}
	if !names2["deploy.approve"] {
		t.Error("expected 'deploy.approve' to be visible after review step")
	}

	// Call approve, then execute
	approveResp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "deploy.approve",
		"arguments": map[string]interface{}{},
	})
	if approveResp.Error != nil {
		t.Fatalf("deploy.approve call error: %v", approveResp.Error)
	}

	time.Sleep(200 * time.Millisecond)

	executeResp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "deploy.execute",
		"arguments": map[string]interface{}{},
	})
	if executeResp.Error != nil {
		t.Fatalf("deploy.execute call error: %v", executeResp.Error)
	}
	var execResult testutil.ToolsCallResult
	json.Unmarshal(executeResp.Result, &execResult)
	if execResult.IsError {
		t.Error("deploy.execute should succeed after approval")
	}
}

func TestE2E_Sidecar(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("sidecar_basic.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	resp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "check_sidecar",
		"arguments": map[string]interface{}{},
	})
	if resp.Error != nil {
		t.Fatalf("check_sidecar error: %v", resp.Error)
	}

	var result testutil.ToolsCallResult
	json.Unmarshal(resp.Result, &result)

	text := extractText(result)
	if !strings.Contains(text, "200") {
		t.Errorf("expected sidecar to be reachable with status 200, got: %s", text)
	}
}

func TestE2E_HotReload(t *testing.T) {
	// Set up a temp dir with a server.py that uses a handlers/ directory
	tmpDir := t.TempDir()
	handlersDir := filepath.Join(tmpDir, "handlers")
	os.MkdirAll(handlersDir, 0755)

	// Create the main server.py that uses discovery with hot_reload=True
	serverContent := []byte("import os\nimport sys\nfrom protomcp.discovery import configure\nfrom protomcp.runner import run\n\nhandlers_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), \"handlers\")\nconfigure(handlers_dir=handlers_dir, hot_reload=True)\n\nif __name__ == \"__main__\":\n    run()\n")
	srcFile := filepath.Join(tmpDir, "server.py")
	os.WriteFile(srcFile, serverContent, 0644)

	// Create v1 handler with "original" tool
	v1Handler := []byte("from protomcp import tool\n\n@tool(description=\"Original tool\")\ndef original() -> str:\n    return \"v1\"\n")
	os.WriteFile(filepath.Join(handlersDir, "tools.py"), v1Handler, 0644)

	w, r, cleanup := StartProtomcp(t, "dev", srcFile)
	defer cleanup()

	InitializeSession(t, w, r)

	// Verify v1 tools
	resp := SendRequestSkipNotifications(t, w, r, "tools/list", nil)
	var list1 testutil.ToolsListResult
	json.Unmarshal(resp.Result, &list1)

	foundOriginal := false
	for _, tool := range list1.Tools {
		if tool.Name == "original" {
			foundOriginal = true
		}
	}
	if !foundOriginal {
		t.Fatal("expected 'original' tool in v1")
	}

	// Overwrite handler file with v2 content (different tool)
	v2Handler := []byte("from protomcp import tool\n\n@tool(description=\"New tool added in v2\")\ndef new_tool() -> str:\n    return \"v2\"\n")
	os.WriteFile(filepath.Join(handlersDir, "tools.py"), v2Handler, 0644)

	// Poll tools/list until we see the new tool (reload: debounce 100ms + process restart)
	deadline := time.Now().Add(10 * time.Second)
	var names map[string]bool
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		resp2 := SendRequestSkipNotifications(t, w, r, "tools/list", nil)
		var list2 testutil.ToolsListResult
		json.Unmarshal(resp2.Result, &list2)

		names = map[string]bool{}
		for _, tool := range list2.Tools {
			names[tool.Name] = true
		}
		if names["new_tool"] {
			break
		}
	}

	if !names["new_tool"] {
		t.Error("expected 'new_tool' after hot reload")
	}
	if names["original"] {
		t.Error("'original' should have been removed after hot reload")
	}
}

func TestE2E_Middleware(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("middleware_basic.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	resp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "echo_args",
		"arguments": map[string]string{"message": "hello"},
	})
	if resp.Error != nil {
		t.Fatalf("echo_args error: %v", resp.Error)
	}

	var result testutil.ToolsCallResult
	json.Unmarshal(resp.Result, &result)
	if result.IsError {
		t.Fatalf("echo_args returned error: %s", string(resp.Result))
	}

	resultText := extractText(result)
	if !strings.Contains(resultText, `"source": "middleware"`) {
		t.Errorf("expected middleware-injected 'source' field in result, got: %s", resultText)
	}

	logResp := SendRequestSkipNotifications(t, w, r, "tools/call", map[string]interface{}{
		"name":      "get_call_log",
		"arguments": map[string]interface{}{},
	})
	var logResult testutil.ToolsCallResult
	json.Unmarshal(logResp.Result, &logResult)

	logText := extractText(logResult)
	if !strings.Contains(logText, "echo_args") {
		t.Errorf("expected 'echo_args' in call log, got: %s", logText)
	}
}
