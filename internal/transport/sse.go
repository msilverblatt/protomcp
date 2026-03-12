package transport

import (
	"context"
	"fmt"

	"github.com/protomcp/protomcp/internal/mcp"
)

// SSETransport is a stub implementation of the Transport interface for Server-Sent Events.
type SSETransport struct {
	host string
	port int
}

// NewSSETransport creates a new SSETransport (stub).
func NewSSETransport(host string, port int) *SSETransport {
	return &SSETransport{host: host, port: port}
}

func (s *SSETransport) Start(ctx context.Context, handler RequestHandler) error {
	return fmt.Errorf("SSE transport not yet implemented")
}

func (s *SSETransport) SendNotification(notification mcp.JSONRPCNotification) error {
	return fmt.Errorf("SSE transport not yet implemented")
}

func (s *SSETransport) Close() error {
	return nil
}
