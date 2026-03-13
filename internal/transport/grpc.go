package transport

import (
	"context"
	"fmt"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// GRPCTransport is a stub implementation of the Transport interface for gRPC.
type GRPCTransport struct {
	host string
	port int
}

// NewGRPCTransport creates a new GRPCTransport (stub).
func NewGRPCTransport(host string, port int) *GRPCTransport {
	return &GRPCTransport{host: host, port: port}
}

func (g *GRPCTransport) Start(ctx context.Context, handler RequestHandler) error {
	return fmt.Errorf("gRPC transport not yet implemented")
}

func (g *GRPCTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	return fmt.Errorf("gRPC transport not yet implemented")
}

func (g *GRPCTransport) Close() error {
	return nil
}
