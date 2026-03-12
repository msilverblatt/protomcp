package transport

import (
	"context"
	"fmt"

	"github.com/protomcp/protomcp/internal/mcp"
)

// HTTPTransport is a stub implementation of the Transport interface for HTTP.
type HTTPTransport struct {
	host string
	port int
}

// NewHTTPTransport creates a new HTTPTransport (stub).
func NewHTTPTransport(host string, port int) *HTTPTransport {
	return &HTTPTransport{host: host, port: port}
}

func (h *HTTPTransport) Start(ctx context.Context, handler RequestHandler) error {
	return fmt.Errorf("HTTP transport not yet implemented")
}

func (h *HTTPTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	return fmt.Errorf("HTTP transport not yet implemented")
}

func (h *HTTPTransport) Close() error {
	return nil
}
