package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/middleware"
	"golang.org/x/net/websocket"
)

func TestSSETransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewSSETransport("localhost", 0)

	var capturedCtx context.Context
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		mu.Lock()
		capturedCtx = ctx
		mu.Unlock()
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	})

	// Wait for listener
	for i := 0; i < 100; i++ {
		if tp.listener != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tp.listener == nil {
		t.Fatal("SSE listener did not start")
	}
	addr := tp.listener.Addr().String()

	body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	req, _ := http.NewRequest("POST", "http://"+addr+"/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-API-Key", "my-api-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer test-token" {
		t.Fatalf("expected auth header 'Bearer test-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "my-api-key" {
		t.Fatalf("expected api key 'my-api-key', got %q", got)
	}

	cancel()
}

func TestHTTPTransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewHTTPTransport("localhost", 0)

	var capturedCtx context.Context
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		mu.Lock()
		capturedCtx = ctx
		mu.Unlock()
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}, nil
	})

	for i := 0; i < 100; i++ {
		if tp.listener != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tp.listener == nil {
		t.Fatal("HTTP listener did not start")
	}
	addr := tp.listener.Addr().String()

	body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	req, _ := http.NewRequest("POST", "http://"+addr+"/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer http-token")
	req.Header.Set("X-API-Key", "http-key")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer http-token" {
		t.Fatalf("expected 'Bearer http-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "http-key" {
		t.Fatalf("expected 'http-key', got %q", got)
	}

	cancel()
}

func TestWSTransport_InjectsAuthHeaders(t *testing.T) {
	tp := NewWSTransport("localhost", 0)

	var capturedCtx context.Context
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		mu.Lock()
		capturedCtx = ctx
		mu.Unlock()
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: json.RawMessage(`1`)}, nil
	})

	for i := 0; i < 100; i++ {
		if tp.listener != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if tp.listener == nil {
		t.Fatal("WS listener did not start")
	}
	addr := tp.listener.Addr().String()

	wsConfig, err := websocket.NewConfig("ws://"+addr+"/ws", "http://"+addr)
	if err != nil {
		t.Fatal(err)
	}
	wsConfig.Header.Set("Authorization", "Bearer ws-token")
	wsConfig.Header.Set("X-API-Key", "ws-key")

	conn, err := websocket.DialConfig(wsConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	msg := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	if err := websocket.Message.Send(conn, msg); err != nil {
		t.Fatal(err)
	}
	var reply string
	if err := websocket.Message.Receive(conn, &reply); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if capturedCtx == nil {
		t.Fatal("handler was not called")
	}
	if got := middleware.GetAuthHeader(capturedCtx); got != "Bearer ws-token" {
		t.Fatalf("expected 'Bearer ws-token', got %q", got)
	}
	if got := middleware.GetAPIKeyHeader(capturedCtx); got != "ws-key" {
		t.Fatalf("expected 'ws-key', got %q", got)
	}

	cancel()
}
