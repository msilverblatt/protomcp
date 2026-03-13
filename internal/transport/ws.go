package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/msilverblatt/protomcp/internal/mcp"
	"golang.org/x/net/websocket"
)

// WSTransport implements the Transport interface using WebSocket.
// Clients connect to /ws and exchange JSON-RPC messages full-duplex.
type WSTransport struct {
	host string
	port int

	mu      sync.RWMutex
	conns   map[*websocket.Conn]struct{}
	server  *http.Server
	listener net.Listener
}

// NewWSTransport creates a new WSTransport.
func NewWSTransport(host string, port int) *WSTransport {
	return &WSTransport{
		host:  host,
		port:  port,
		conns: make(map[*websocket.Conn]struct{}),
	}
}

func (w *WSTransport) addConn(c *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conns[c] = struct{}{}
}

func (w *WSTransport) removeConn(c *websocket.Conn) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.conns, c)
}

// Start starts the HTTP server, upgrades /ws connections to WebSocket,
// and blocks until ctx is cancelled.
func (w *WSTransport) Start(ctx context.Context, handler RequestHandler) error {
	wsHandler := websocket.Handler(func(conn *websocket.Conn) {
		w.addConn(conn)
		defer func() {
			w.removeConn(conn)
			conn.Close()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			var raw []byte
			if err := websocket.Message.Receive(conn, &raw); err != nil {
				return
			}

			var req mcp.JSONRPCRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				continue
			}

			resp, err := handler(ctx, req)
			if err != nil {
				continue
			}

			// Notifications (no ID) get no response.
			if req.ID == nil {
				continue
			}

			if resp == nil {
				continue
			}

			data, err := json.Marshal(resp)
			if err != nil {
				continue
			}

			if err := websocket.Message.Send(conn, string(data)); err != nil {
				return
			}
		}
	})

	mux := http.NewServeMux()
	mux.Handle("/ws", wsHandler)

	var err error
	addr := fmt.Sprintf("%s:%d", w.host, w.port)
	w.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ws listen %s: %w", addr, err)
	}

	w.server = &http.Server{Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		if err := w.server.Serve(w.listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		_ = w.server.Close()
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// SendNotification sends a JSON-RPC notification to all connected WebSocket clients.
func (w *WSTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	for conn := range w.conns {
		_ = websocket.Message.Send(conn, string(data))
	}
	return nil
}

// Close shuts down the HTTP server.
func (w *WSTransport) Close() error {
	if w.server != nil {
		return w.server.Close()
	}
	return nil
}
