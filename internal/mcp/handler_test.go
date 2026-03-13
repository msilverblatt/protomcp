package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

type mockToolBackend struct {
	tools      []*pb.ToolDefinition
	callResult *pb.CallToolResponse
}

func (m *mockToolBackend) ActiveTools() []*pb.ToolDefinition {
	return m.tools
}

func (m *mockToolBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	if m.callResult != nil {
		return m.callResult, nil
	}
	return &pb.CallToolResponse{
		ResultJson: `[{"type":"text","text":"result"}]`,
	}, nil
}

func TestHandleInitialize(t *testing.T) {
	h := mcp.NewHandler(&mockToolBackend{})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle initialize failed: %v", err)
	}

	var result mcp.InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !result.Capabilities.Tools.ListChanged {
		t.Error("expected tools.listChanged = true")
	}
}

func TestHandleToolsList(t *testing.T) {
	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{
			{Name: "search", Description: "Search docs", InputSchemaJson: `{"type":"object","properties":{"query":{"type":"string"}}}`},
		},
	}
	h := mcp.NewHandler(backend)
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle tools/list failed: %v", err)
	}

	var result mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "search" {
		t.Errorf("tool name = %q, want %q", result.Tools[0].Name, "search")
	}
}

func TestHandleToolsCall(t *testing.T) {
	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{
			{Name: "search", Description: "Search docs", InputSchemaJson: `{}`},
		},
	}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(map[string]interface{}{
		"name":      "search",
		"arguments": map[string]string{"query": "hello"},
	})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`3`),
		Method:  "tools/call",
		Params:  params,
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle tools/call failed: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestHandleUnknownMethod(t *testing.T) {
	h := mcp.NewHandler(&mockToolBackend{})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`4`),
		Method:  "unknown/method",
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestHandleToolsList_OutputSchemaAndAnnotations(t *testing.T) {
	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{
			{
				Name:             "write_file",
				Description:      "Write a file",
				InputSchemaJson:  `{"type":"object"}`,
				OutputSchemaJson: `{"type":"object","properties":{"path":{"type":"string"}}}`,
				Title:            "Write File",
				ReadOnlyHint:     false,
				DestructiveHint:  true,
			},
		},
	}
	h := mcp.NewHandler(backend)
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`10`),
		Method:  "tools/list",
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle tools/list failed: %v", err)
	}

	var result mcp.ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	tool := result.Tools[0]
	if string(tool.OutputSchema) == "" {
		t.Fatal("expected outputSchema to be set")
	}
	if tool.Annotations == nil {
		t.Fatal("expected annotations to be set")
	}
	if tool.Annotations.Title != "Write File" {
		t.Errorf("expected title 'Write File', got %q", tool.Annotations.Title)
	}
	if !tool.Annotations.DestructiveHint {
		t.Error("expected destructiveHint = true")
	}
}

func TestHandleToolsCall_StructuredContent(t *testing.T) {
	backend := &mockBackendWithStructured{}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(map[string]interface{}{
		"name":      "structured_tool",
		"arguments": map[string]string{},
	})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`11`),
		Method:  "tools/call",
		Params:  params,
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle tools/call failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var result mcp.ToolsCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if string(result.StructuredContent) == "" {
		t.Fatal("expected structuredContent to be set")
	}
}

type mockBackendWithStructured struct{}

func (m *mockBackendWithStructured) ActiveTools() []*pb.ToolDefinition {
	return []*pb.ToolDefinition{{Name: "structured_tool", InputSchemaJson: `{}`}}
}

func (m *mockBackendWithStructured) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return &pb.CallToolResponse{
		ResultJson:            `[{"type":"text","text":"ok"}]`,
		StructuredContentJson: `{"count":42}`,
	}, nil
}

func TestHandleCancelledNotification(t *testing.T) {
	h := mcp.NewHandler(&mockToolBackend{})

	// Track a call first
	h.CancelTracker().TrackCallWithContext(context.Background(), "req-99")

	params, _ := json.Marshal(map[string]interface{}{
		"requestId": "req-99",
		"reason":    "user cancelled",
	})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/cancelled",
		Params:  params,
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Fatal("expected nil response for notification")
	}
	if !h.CancelTracker().IsCancelled("req-99") {
		t.Fatal("expected req-99 to be marked cancelled")
	}
}

func TestHandleTasksGetAndCancel(t *testing.T) {
	h := mcp.NewHandler(&mockToolBackend{})
	h.TaskManager().Register("task-abc", "req-1")

	getParams, _ := json.Marshal(map[string]interface{}{"taskId": "task-abc"})
	getReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`20`),
		Method:  "tasks/get",
		Params:  getParams,
	}

	resp, err := h.Handle(context.Background(), getReq)
	if err != nil {
		t.Fatalf("tasks/get failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	var getResult mcp.TasksGetResult
	if err := json.Unmarshal(resp.Result, &getResult); err != nil {
		t.Fatalf("unmarshal tasks/get result: %v", err)
	}
	if getResult.State != "running" {
		t.Fatalf("expected running, got %s", getResult.State)
	}

	cancelParams, _ := json.Marshal(map[string]interface{}{"taskId": "task-abc"})
	cancelReq := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`21`),
		Method:  "tasks/cancel",
		Params:  cancelParams,
	}

	resp, err = h.Handle(context.Background(), cancelReq)
	if err != nil {
		t.Fatalf("tasks/cancel failed: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Verify state is now cancelled
	state, err := h.TaskManager().GetStatus("task-abc")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "cancelled" {
		t.Fatalf("expected cancelled, got %s", state.State)
	}
}

func TestHandleTasksGet_Unknown(t *testing.T) {
	h := mcp.NewHandler(&mockToolBackend{})

	params, _ := json.Marshal(map[string]interface{}{"taskId": "nonexistent"})
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`22`),
		Method:  "tasks/get",
		Params:  params,
	}

	resp, err := h.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unknown task")
	}
}

func TestHandleToolsCall_RawPassthrough(t *testing.T) {
	largeText := strings.Repeat("X", 100000)
	contentJSON := fmt.Sprintf(`[{"type":"text","text":"%s"}]`, largeText)

	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{{Name: "generate", InputSchemaJson: `{"type":"object"}`}},
		callResult: &pb.CallToolResponse{
			ResultJson: contentJSON,
		},
	}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "generate"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected RPC error: %s", resp.Error.Message)
	}

	var result struct {
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)

	if string(result.Content) != contentJSON {
		t.Errorf("content was re-serialized instead of passed through.\nGot length: %d\nWant length: %d", len(result.Content), len(contentJSON))
	}
}

func TestHandleToolsCall_RawPassthrough_Fallback(t *testing.T) {
	backend := &mockToolBackend{
		tools: []*pb.ToolDefinition{{Name: "echo", InputSchemaJson: `{"type":"object"}`}},
		callResult: &pb.CallToolResponse{
			ResultJson: `"just a string"`,
		},
	}
	h := mcp.NewHandler(backend)

	params, _ := json.Marshal(mcp.ToolsCallParams{Name: "echo"})
	resp, err := h.Handle(context.Background(), mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  params,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result struct {
		Content []mcp.ContentItem `json:"content"`
	}
	json.Unmarshal(resp.Result, &result)

	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Errorf("expected fallback to text content, got: %+v", result.Content)
	}
}
