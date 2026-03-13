package transport

import (
	"context"
	"fmt"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// WSTransport is a stub implementation of the Transport interface for WebSocket.
type WSTransport struct {
	host string
	port int
}

// NewWSTransport creates a new WSTransport (stub).
func NewWSTransport(host string, port int) *WSTransport {
	return &WSTransport{host: host, port: port}
}

func (w *WSTransport) Start(ctx context.Context, handler RequestHandler) error {
	return fmt.Errorf("WebSocket transport not yet implemented")
}

func (w *WSTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	return fmt.Errorf("WebSocket transport not yet implemented")
}

func (w *WSTransport) Close() error {
	return nil
}
