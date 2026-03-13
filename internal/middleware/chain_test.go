package middleware

import (
	"context"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

func TestChain_AppliesInOrder(t *testing.T) {
	var order []string

	mw1 := func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			order = append(order, "mw1-before")
			resp, err := next(ctx, req)
			order = append(order, "mw1-after")
			return resp, err
		}
	}

	mw2 := func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			order = append(order, "mw2-before")
			resp, err := next(ctx, req)
			order = append(order, "mw2-after")
			return resp, err
		}
	}

	handler := func(_ context.Context, _ mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		order = append(order, "handler")
		return &mcp.JSONRPCResponse{}, nil
	}

	chained := Chain(handler, mw1, mw2)
	chained(context.Background(), mcp.JSONRPCRequest{})

	expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("position %d: expected %s, got %s", i, v, order[i])
		}
	}
}

func TestChain_Empty(t *testing.T) {
	called := false
	handler := func(_ context.Context, _ mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		called = true
		return &mcp.JSONRPCResponse{}, nil
	}

	chained := Chain(handler)
	chained(context.Background(), mcp.JSONRPCRequest{})

	if !called {
		t.Fatal("handler should have been called")
	}
}
