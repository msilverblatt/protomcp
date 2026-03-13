package middleware

import (
	"context"
	"encoding/json"
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/mcp"
)

type mockDispatcher struct {
	responses map[string]*pb.MiddlewareInterceptResponse
	calls     []dispatchCall
}

type dispatchCall struct {
	Name, Phase, ToolName, ArgsJSON, ResultJSON string
	IsError                                     bool
}

func (m *mockDispatcher) SendMiddlewareIntercept(_ context.Context, name, phase, toolName, argsJSON, resultJSON string, isError bool) (*pb.MiddlewareInterceptResponse, error) {
	m.calls = append(m.calls, dispatchCall{name, phase, toolName, argsJSON, resultJSON, isError})
	key := name + ":" + phase
	if resp, ok := m.responses[key]; ok {
		return resp, nil
	}
	return &pb.MiddlewareInterceptResponse{
		ArgumentsJson: argsJSON,
		ResultJson:    resultJSON,
	}, nil
}

func makeToolCallReq(toolName string, args map[string]interface{}) mcp.JSONRPCRequest {
	argsJSON, _ := json.Marshal(args)
	params, _ := json.Marshal(map[string]json.RawMessage{
		"name":      json.RawMessage(`"` + toolName + `"`),
		"arguments": argsJSON,
	})
	return mcp.JSONRPCRequest{
		Method: "tools/call",
		Params: params,
	}
}

func TestCustomMiddleware_BeforePhase(t *testing.T) {
	d := &mockDispatcher{responses: map[string]*pb.MiddlewareInterceptResponse{}}
	mw := CustomMiddleware(d, []RegisteredMW{{Name: "audit", Priority: 10}})

	called := false
	handler := mw(func(_ context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		called = true
		return &mcp.JSONRPCResponse{Result: json.RawMessage(`"ok"`)}, nil
	})

	handler(context.Background(), makeToolCallReq("test_tool", map[string]interface{}{"key": "val"}))

	if !called {
		t.Fatal("handler was not called")
	}
	if len(d.calls) < 1 {
		t.Fatal("expected at least 1 dispatch call")
	}
	if d.calls[0].Phase != "before" {
		t.Fatalf("expected before phase, got %s", d.calls[0].Phase)
	}
}

func TestCustomMiddleware_Rejection(t *testing.T) {
	d := &mockDispatcher{
		responses: map[string]*pb.MiddlewareInterceptResponse{
			"blocker:before": {Reject: true, RejectReason: "blocked"},
		},
	}
	mw := CustomMiddleware(d, []RegisteredMW{{Name: "blocker", Priority: 1}})

	called := false
	handler := mw(func(_ context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		called = true
		return &mcp.JSONRPCResponse{}, nil
	})

	_, err := handler(context.Background(), makeToolCallReq("test_tool", nil))

	if called {
		t.Fatal("handler should not have been called after rejection")
	}
	if err == nil {
		t.Fatal("expected error from rejection")
	}
}

func TestCustomMiddleware_PriorityOrdering(t *testing.T) {
	d := &mockDispatcher{responses: map[string]*pb.MiddlewareInterceptResponse{}}
	mw := CustomMiddleware(d, []RegisteredMW{
		{Name: "second", Priority: 20},
		{Name: "first", Priority: 10},
	})

	handler := mw(func(_ context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{Result: json.RawMessage(`"ok"`)}, nil
	})

	handler(context.Background(), makeToolCallReq("test_tool", nil))

	// Before phase: first (10) then second (20)
	if len(d.calls) < 2 {
		t.Fatalf("expected at least 2 before calls, got %d", len(d.calls))
	}
	if d.calls[0].Name != "first" {
		t.Fatalf("expected first middleware first, got %s", d.calls[0].Name)
	}
	if d.calls[1].Name != "second" {
		t.Fatalf("expected second middleware second, got %s", d.calls[1].Name)
	}
}

func TestCustomMiddleware_SkipsNonToolCall(t *testing.T) {
	d := &mockDispatcher{responses: map[string]*pb.MiddlewareInterceptResponse{}}
	mw := CustomMiddleware(d, []RegisteredMW{{Name: "audit", Priority: 10}})

	called := false
	handler := mw(func(_ context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		called = true
		return &mcp.JSONRPCResponse{}, nil
	})

	handler(context.Background(), mcp.JSONRPCRequest{Method: "initialize"})

	if !called {
		t.Fatal("handler should have been called for non-tools/call")
	}
	if len(d.calls) != 0 {
		t.Fatalf("expected no dispatch calls for non-tools/call, got %d", len(d.calls))
	}
}

func TestExtractToolCallParams(t *testing.T) {
	req := makeToolCallReq("my_tool", map[string]interface{}{"x": 1})
	name, argsJSON := extractToolCallParams(req)
	if name != "my_tool" {
		t.Fatalf("expected my_tool, got %s", name)
	}
	var args map[string]interface{}
	json.Unmarshal([]byte(argsJSON), &args)
	if args["x"] != float64(1) {
		t.Fatalf("expected x=1, got %v", args["x"])
	}
}
