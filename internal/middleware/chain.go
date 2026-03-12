package middleware

import (
	"context"

	"github.com/protomcp/protomcp/internal/mcp"
)

// Handler processes an MCP request and returns a response.
type Handler func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error)

// Middleware wraps a Handler with additional behavior.
type Middleware func(next Handler) Handler

// Chain applies middleware in order (first middleware is outermost).
func Chain(handler Handler, middlewares ...Middleware) Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
