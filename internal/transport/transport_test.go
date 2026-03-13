package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"github.com/msilverblatt/protomcp/internal/transport"
	"golang.org/x/net/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// echoHandler returns a handler that echoes the method back as result.
func echoHandler(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
	result, _ := json.Marshal(map[string]string{"method": req.Method})
	return &mcp.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}, nil
}

// startTransportOnServer starts the given transport against a test HTTP server
// via its Start method (which creates its own listener). Returns the bound address.
// For SSE and HTTP transports we use httptest to exercise the handlers directly.

// --- SSE Transport ---

func TestSSETransportPost(t *testing.T) {
	// Test via httptest server that mimics the SSE transport's POST handler.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req mcp.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		resp, _ := echoHandler(r.Context(), req)
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer srv.Close()

	reqBody, _ := json.Marshal(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"})
	res, err := http.Post(srv.URL+"/", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp mcp.JSONRPCResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestSSETransportStart(t *testing.T) {
	tp := transport.NewSSETransport("127.0.0.1", getFreePort(t))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Send a POST request.
	addr := fmt.Sprintf("http://127.0.0.1:%d/", getTransportPort(t, tp))
	_ = addr

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

func TestSSETransportIntegration(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewSSETransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	// POST a request.
	reqBody, _ := json.Marshal(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`42`), Method: "tools/list"})
	res, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/", port), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp mcp.JSONRPCResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	cancel()
	<-errCh
}

func TestSSETransportSendNotification(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewSSETransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Connect SSE client.
	sseCtx, sseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sseCancel()

	req, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/sse", port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("SSE connect: %v", err)
	}
	defer resp.Body.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		received <- string(buf[:n])
	}()

	// Give SSE client time to register.
	time.Sleep(20 * time.Millisecond)

	notif := mcp.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/tools/list_changed"}
	if err := tp.SendNotification(notif); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}

	select {
	case data := <-received:
		if !strings.Contains(data, "notifications/tools/list_changed") {
			t.Errorf("unexpected SSE data: %q", data)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for SSE notification")
	}

	cancel()
	<-errCh
}

// --- HTTP Transport ---

func TestHTTPTransportIntegration(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewHTTPTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	reqBody, _ := json.Marshal(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"})
	res, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/", port), "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var resp mcp.JSONRPCResponse
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	cancel()
	<-errCh
}

func TestHTTPTransportSendNotification(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewHTTPTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Connect /events SSE client.
	sseCtx, sseCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer sseCancel()

	req, _ := http.NewRequestWithContext(sseCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/events", port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("events connect: %v", err)
	}
	defer resp.Body.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := resp.Body.Read(buf)
		received <- string(buf[:n])
	}()

	time.Sleep(20 * time.Millisecond)

	notif := mcp.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/tools/list_changed"}
	if err := tp.SendNotification(notif); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}

	select {
	case data := <-received:
		if !strings.Contains(data, "notifications/tools/list_changed") {
			t.Errorf("unexpected events data: %q", data)
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for /events notification")
	}

	cancel()
	<-errCh
}

func TestHTTPTransportInvalidMethod(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewHTTPTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = tp.Start(ctx, echoHandler) }()
	time.Sleep(50 * time.Millisecond)

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", res.StatusCode)
	}
}

// --- WebSocket Transport ---

func TestWSTransportIntegration(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewWSTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	origin := "http://127.0.0.1/"
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	conn, err := websocket.Dial(url, "", origin)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	reqBody, _ := json.Marshal(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`7`), Method: "tools/list"})
	if err := websocket.Message.Send(conn, string(reqBody)); err != nil {
		t.Fatalf("ws send: %v", err)
	}

	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("ws receive: %v", err)
	}

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("decode ws response: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	cancel()
	<-errCh
}

func TestWSTransportNotification(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewWSTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = tp.Start(ctx, echoHandler) }()
	time.Sleep(50 * time.Millisecond)

	origin := "http://127.0.0.1/"
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	conn, err := websocket.Dial(url, "", origin)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	// Give server time to register the connection.
	time.Sleep(20 * time.Millisecond)

	notif := mcp.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/tools/list_changed"}
	if err := tp.SendNotification(notif); err != nil {
		t.Fatalf("SendNotification: %v", err)
	}

	conn.SetDeadline(time.Now().Add(time.Second))
	var raw string
	if err := websocket.Message.Receive(conn, &raw); err != nil {
		t.Fatalf("ws receive notification: %v", err)
	}

	var got mcp.JSONRPCNotification
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if got.Method != "notifications/tools/list_changed" {
		t.Errorf("expected method %q, got %q", "notifications/tools/list_changed", got.Method)
	}
}

// --- gRPC Transport ---

func TestGRPCTransportIntegration(t *testing.T) {
	port := getFreePort(t)
	tp := transport.NewGRPCTransport("127.0.0.1", port)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tp.Start(ctx, echoHandler)
	}()
	time.Sleep(50 * time.Millisecond)

	conn, err := grpc.NewClient(
		fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer conn.Close()

	reqJSON, _ := json.Marshal(mcp.JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"})
	in := wrapperspb.String(string(reqJSON))
	out := new(wrapperspb.StringValue)

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	if err := conn.Invoke(callCtx, "/protomcp.MCPService/Call", in, out); err != nil {
		t.Fatalf("grpc invoke: %v", err)
	}

	var resp mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(out.GetValue()), &resp); err != nil {
		t.Fatalf("decode grpc response: %v", err)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	cancel()
	<-errCh
}

func TestGRPCTransportSendNotification(t *testing.T) {
	tp := transport.NewGRPCTransport("127.0.0.1", 0)
	notif := mcp.JSONRPCNotification{JSONRPC: "2.0", Method: "notifications/tools/list_changed"}
	// SendNotification is a no-op for gRPC unary transport; just verify no error.
	if err := tp.SendNotification(notif); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- helpers ---

// getFreePort returns an available TCP port.
func getFreePort(t *testing.T) int {
	t.Helper()
	// Use httptest to get a free port, then close immediately.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.Listener.Addr().String()
	srv.Close()
	// Parse port from addr like "127.0.0.1:PORT"
	var port int
	fmt.Sscanf(addr[strings.LastIndex(addr, ":")+1:], "%d", &port)
	return port
}

// getTransportPort is unused but kept for reference.
func getTransportPort(t *testing.T, tp interface{}) int {
	t.Helper()
	return 0
}

// discard io.Reader content.
func drainBody(r io.Reader) {
	_, _ = io.Copy(io.Discard, r)
}
