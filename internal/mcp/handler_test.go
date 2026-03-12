package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/protomcp/protomcp/internal/mcp"

	pb "github.com/protomcp/protomcp/gen/proto/protomcp"
)

type mockToolBackend struct {
	tools []*pb.ToolDefinition
}

func (m *mockToolBackend) ActiveTools() []*pb.ToolDefinition {
	return m.tools
}

func (m *mockToolBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
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
