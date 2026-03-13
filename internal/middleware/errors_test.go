package middleware

import (
	"context"
	"fmt"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

func TestErrorFormatting_PanicRecovery(t *testing.T) {
	mw := ErrorFormatting()
	handler := mw(func(_ context.Context, _ mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		panic("test panic")
	})

	resp, err := handler(context.Background(), mcp.JSONRPCRequest{ID: []byte(`1`)})

	if err != nil {
		t.Fatalf("expected nil error after panic recovery, got %v", err)
	}
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response after panic")
	}
	if resp.Error.Code != -32603 {
		t.Fatalf("expected code -32603, got %d", resp.Error.Code)
	}
}

func TestErrorFormatting_ErrorConversion(t *testing.T) {
	mw := ErrorFormatting()
	handler := mw(func(_ context.Context, _ mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return nil, fmt.Errorf("something failed")
	})

	resp, err := handler(context.Background(), mcp.JSONRPCRequest{ID: []byte(`1`)})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil || resp.Error == nil {
		t.Fatal("expected error response")
	}
	if resp.Error.Message != "something failed" {
		t.Fatalf("expected 'something failed', got %s", resp.Error.Message)
	}
}

func TestErrorFormatting_PassThrough(t *testing.T) {
	mw := ErrorFormatting()
	handler := mw(func(_ context.Context, _ mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{JSONRPC: "2.0"}, nil
	})

	resp, err := handler(context.Background(), mcp.JSONRPCRequest{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatal("expected pass-through response")
	}
}
