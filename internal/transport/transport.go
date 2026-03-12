package transport

import (
	"context"

	"github.com/protomcp/protomcp/internal/mcp"
)

// RequestHandler processes a JSON-RPC request and returns a response.
type RequestHandler func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error)

// Transport defines the interface for MCP transports.
type Transport interface {
	Start(ctx context.Context, handler RequestHandler) error
	SendNotification(notification mcp.JSONRPCNotification) error
	Close() error
}
