package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/transport"
)

func TestStdioTransport(t *testing.T) {
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"
	reader := strings.NewReader(req)
	var buf bytes.Buffer
	tp := transport.NewStdioWithIO(reader, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		result, _ := json.Marshal(map[string]string{"status": "ok"})
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify response was written
	var resp mcp.JSONRPCResponse
	if err := json.NewDecoder(&buf).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestStdioTransportNotification(t *testing.T) {
	// Notifications have no ID, so no response should be written
	req := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n"
	reader := strings.NewReader(req)
	var buf bytes.Buffer
	tp := transport.NewStdioWithIO(reader, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	handlerCalled := false
	err := tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		handlerCalled = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !handlerCalled {
		t.Error("handler was not called for notification")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for notification, got: %s", buf.String())
	}
}

func TestStdioTransportSendNotification(t *testing.T) {
	var buf bytes.Buffer
	tp := transport.NewStdioWithIO(strings.NewReader(""), &buf)

	notification := mcp.JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/tools/list_changed",
	}

	if err := tp.SendNotification(notification); err != nil {
		t.Fatalf("send notification: %v", err)
	}

	var got mcp.JSONRPCNotification
	if err := json.NewDecoder(&buf).Decode(&got); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if got.Method != "notifications/tools/list_changed" {
		t.Errorf("expected method %q, got %q", "notifications/tools/list_changed", got.Method)
	}
}

func TestStdioTransportMultipleRequests(t *testing.T) {
	reqs := `{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n"
	reader := strings.NewReader(reqs)
	var buf bytes.Buffer
	tp := transport.NewStdioWithIO(reader, &buf)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	callCount := 0
	err := tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		callCount++
		result, _ := json.Marshal(map[string]string{"method": req.Method})
		return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 handler calls, got %d", callCount)
	}

	// Decode both responses
	dec := json.NewDecoder(&buf)
	for i := 0; i < 2; i++ {
		var resp mcp.JSONRPCResponse
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("decode response %d: %v", i+1, err)
		}
		if resp.Error != nil {
			t.Errorf("response %d: unexpected error: %v", i+1, resp.Error)
		}
	}
}
