package middleware_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/middleware"
)

func TestAuthTokenValid(t *testing.T) {
	os.Setenv("TEST_TOKEN", "secret123")
	defer os.Unsetenv("TEST_TOKEN")

	mw, err := middleware.NewAuth([]string{"token:TEST_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}

	next := func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{ID: req.ID, Result: []byte(`"ok"`)}, nil
	}

	handler := mw(next)
	ctx := middleware.WithAuthHeader(context.Background(), "Bearer secret123")
	resp, err := handler(ctx, mcp.JSONRPCRequest{ID: json.RawMessage(`1`), Method: "tools/call"})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}

func TestAuthTokenInvalid(t *testing.T) {
	os.Setenv("TEST_TOKEN", "secret123")
	defer os.Unsetenv("TEST_TOKEN")

	mw, err := middleware.NewAuth([]string{"token:TEST_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}

	next := func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{ID: req.ID, Result: []byte(`"ok"`)}, nil
	}

	handler := mw(next)
	ctx := middleware.WithAuthHeader(context.Background(), "Bearer wrong")
	_, err = handler(ctx, mcp.JSONRPCRequest{ID: json.RawMessage(`1`), Method: "tools/call"})
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestAuthTokenMissing(t *testing.T) {
	os.Setenv("TEST_TOKEN", "secret123")
	defer os.Unsetenv("TEST_TOKEN")

	mw, err := middleware.NewAuth([]string{"token:TEST_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}

	next := func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{}, nil
	}

	handler := mw(next)
	_, err = handler(context.Background(), mcp.JSONRPCRequest{ID: json.RawMessage(`1`), Method: "tools/call"})
	if err == nil {
		t.Fatal("expected auth error for missing header")
	}
}

func TestAuthApikeyValid(t *testing.T) {
	os.Setenv("TEST_KEY", "mykey")
	defer os.Unsetenv("TEST_KEY")

	mw, err := middleware.NewAuth([]string{"apikey:TEST_KEY"})
	if err != nil {
		t.Fatal(err)
	}

	next := func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{ID: req.ID, Result: []byte(`"ok"`)}, nil
	}

	handler := mw(next)
	ctx := middleware.WithAPIKeyHeader(context.Background(), "mykey")
	resp, err := handler(ctx, mcp.JSONRPCRequest{ID: json.RawMessage(`1`), Method: "tools/call"})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}

func TestNewAuthEnvNotSet(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR")
	_, err := middleware.NewAuth([]string{"token:NONEXISTENT_VAR"})
	if err == nil {
		t.Error("expected error for unset env var")
	}
}

func TestNewAuthMultiple(t *testing.T) {
	os.Setenv("TEST_TOKEN2", "tok")
	os.Setenv("TEST_KEY2", "key")
	defer os.Unsetenv("TEST_TOKEN2")
	defer os.Unsetenv("TEST_KEY2")

	mw, err := middleware.NewAuth([]string{"token:TEST_TOKEN2", "apikey:TEST_KEY2"})
	if err != nil {
		t.Fatal(err)
	}

	next := func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return &mcp.JSONRPCResponse{ID: req.ID, Result: []byte(`"ok"`)}, nil
	}

	handler := mw(next)
	ctx := middleware.WithAuthHeader(context.Background(), "Bearer tok")
	ctx = middleware.WithAPIKeyHeader(ctx, "key")
	resp, err := handler(ctx, mcp.JSONRPCRequest{ID: json.RawMessage(`1`), Method: "tools/call"})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
}
