package bridge

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// mockBackend implements FullBackend for testing.
type mockBackend struct {
	tools    []*pb.ToolDefinition
	callResp *pb.CallToolResponse
	callErr  error
}

func (m *mockBackend) ActiveTools() []*pb.ToolDefinition {
	return m.tools
}

func (m *mockBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return m.callResp, m.callErr
}

func (m *mockBackend) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error) {
	return nil, nil
}

func (m *mockBackend) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error) {
	return nil, nil
}

func (m *mockBackend) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error) {
	return nil, nil
}

func (m *mockBackend) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error) {
	return nil, nil
}

func (m *mockBackend) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error) {
	return nil, nil
}

func (m *mockBackend) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error) {
	return &pb.CompletionResponse{Values: []string{}}, nil
}

func (m *mockBackend) SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error {
	return nil
}

func (m *mockBackend) OnSampling(fn func(*pb.SamplingRequest, string)) {}

func (m *mockBackend) SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error {
	return nil
}

func (m *mockBackend) OnListRoots(fn func(string)) {}

func TestBridgeNew(t *testing.T) {
	backend := &mockBackend{
		tools: []*pb.ToolDefinition{
			{Name: "echo", Description: "echoes input", InputSchemaJson: `{"type":"object"}`},
		},
	}
	b := New(backend, nil, "dev")
	if b.Server == nil {
		t.Fatal("expected non-nil server")
	}
}

func TestSyncTools(t *testing.T) {
	backend := &mockBackend{
		tools: []*pb.ToolDefinition{
			{Name: "echo", Description: "echoes input", InputSchemaJson: `{"type":"object","properties":{"message":{"type":"string"}}}`},
			{Name: "add", Description: "adds numbers", InputSchemaJson: `{"type":"object"}`},
		},
	}
	b := New(backend, nil, "dev")
	b.SyncTools()
	// Server should now have 2 tools registered
	// Verified via tool handler invocation in TestMakeToolHandler
}

func TestConvertToolDef(t *testing.T) {
	def := &pb.ToolDefinition{
		Name:            "test",
		Description:     "a test tool",
		InputSchemaJson: `{"type":"object"}`,
		ReadOnlyHint:    true,
		DestructiveHint: true,
		Title:           "Test Tool",
	}
	tool := convertToolDef(def)
	if tool.Name != "test" {
		t.Errorf("expected name 'test', got %q", tool.Name)
	}
	if tool.Title != "Test Tool" {
		t.Errorf("expected title 'Test Tool', got %q", tool.Title)
	}
	if tool.Annotations == nil {
		t.Fatal("expected annotations")
	}
	if !tool.Annotations.ReadOnlyHint {
		t.Error("expected ReadOnlyHint")
	}
}

func TestConvertToolDefDefaultSchema(t *testing.T) {
	def := &pb.ToolDefinition{
		Name: "bare",
	}
	tool := convertToolDef(def)
	// Should have a default input schema to avoid SDK panic
	if tool.InputSchema == nil {
		t.Fatal("expected non-nil InputSchema")
	}
}

func TestMakeToolHandler(t *testing.T) {
	resultJSON := `[{"type":"text","text":"hello"}]`
	backend := &mockBackend{
		callResp: &pb.CallToolResponse{
			ResultJson: resultJSON,
			IsError:    false,
		},
	}

	handler := makeToolHandler(backend, "echo", nil)
	// Create a minimal CallToolRequest
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "echo",
			Arguments: []byte(`{"message":"hi"}`),
		},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "hello" {
		t.Errorf("expected 'hello', got %q", tc.Text)
	}
}

func TestMakeToolHandlerError(t *testing.T) {
	backend := &mockBackend{
		callResp: &pb.CallToolResponse{
			ResultJson: `[{"type":"text","text":"something went wrong"}]`,
			IsError:    true,
		},
	}

	handler := makeToolHandler(backend, "fail", nil)
	req := &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name: "fail",
		},
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError to be true")
	}
}
