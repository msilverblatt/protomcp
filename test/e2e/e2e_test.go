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

// ---------------------------------------------------------------------------
// Python E2E: resource read
// ---------------------------------------------------------------------------

func TestE2E_Python_ResourceRead(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("resource_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	// List resources
	listResp := SendRequest(t, w, r, "resources/list", nil)
	if listResp.Error != nil {
		t.Fatalf("resources/list error: %v", listResp.Error)
	}
	var listResult struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(listResp.Result, &listResult); err != nil {
		t.Fatalf("unmarshal resources/list: %v", err)
	}
	if len(listResult.Resources) < 1 {
		t.Fatalf("expected at least 1 resource, got %d", len(listResult.Resources))
	}

	// Read a specific resource
	readResp := SendRequest(t, w, r, "resources/read", map[string]interface{}{
		"uri": "config://app",
	})
	if readResp.Error != nil {
		t.Fatalf("resources/read error: %v", readResp.Error)
	}
	var readResult struct {
		Contents []struct {
			URI      string `json:"uri"`
			Text     string `json:"text"`
			MIMEType string `json:"mimeType"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(readResp.Result, &readResult); err != nil {
		t.Fatalf("unmarshal resources/read: %v", err)
	}
	if len(readResult.Contents) == 0 {
		t.Fatal("expected at least 1 content item")
	}
	if readResult.Contents[0].Text == "" {
		t.Error("expected non-empty text content")
	}
}

// ---------------------------------------------------------------------------
// Python E2E: prompt get
// ---------------------------------------------------------------------------

func TestE2E_Python_PromptGet(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("prompt_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)

	// List prompts
	listResp := SendRequest(t, w, r, "prompts/list", nil)
	if listResp.Error != nil {
		t.Fatalf("prompts/list error: %v", listResp.Error)
	}
	var listResult struct {
		Prompts []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"prompts"`
	}
	if err := json.Unmarshal(listResp.Result, &listResult); err != nil {
		t.Fatalf("unmarshal prompts/list: %v", err)
	}
	if len(listResult.Prompts) < 1 {
		t.Fatalf("expected at least 1 prompt, got %d", len(listResult.Prompts))
	}

	// Get a specific prompt
	getResp := SendRequest(t, w, r, "prompts/get", map[string]interface{}{
		"name":      "greet",
		"arguments": map[string]string{"name": "Alice"},
	})
	if getResp.Error != nil {
		t.Fatalf("prompts/get error: %v", getResp.Error)
	}
	var getResult struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(getResp.Result, &getResult); err != nil {
		t.Fatalf("unmarshal prompts/get: %v", err)
	}
	if len(getResult.Messages) == 0 {
		t.Fatal("expected at least 1 message")
	}
	if getResult.Messages[0].Content.Text == "" {
		t.Error("expected non-empty prompt message text")
	}
}

// ---------------------------------------------------------------------------
// Python E2E: tool call with echo verification
// ---------------------------------------------------------------------------

func TestE2E_Python_ToolCallEchoVerify(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("simple_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)
	resp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]string{"message": "test_echo_value"},
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
	if len(result.Content) == 0 {
		t.Fatal("expected at least 1 content item")
	}
	found := false
	for _, c := range result.Content {
		if c.Text == "test_echo_value" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected echo to return 'test_echo_value', got content: %+v", result.Content)
	}
}

// ---------------------------------------------------------------------------
// Python E2E: add tool call
// ---------------------------------------------------------------------------

func TestE2E_Python_ToolCallAdd(t *testing.T) {
	w, r, cleanup := StartProtomcp(t, "dev", fixture("simple_tool.py"))
	defer cleanup()

	InitializeSession(t, w, r)
	resp := SendRequest(t, w, r, "tools/call", map[string]interface{}{
		"name":      "add",
		"arguments": map[string]interface{}{"a": 7, "b": 3},
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
	if len(result.Content) == 0 {
		t.Fatal("expected at least 1 content item")
	}
	found := false
	for _, c := range result.Content {
		if c.Text == "10" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected add(7,3) to return '10', got content: %+v", result.Content)
	}
}
