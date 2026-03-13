package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// ErrorFormatting returns middleware that catches panics and formats errors.
func ErrorFormatting() Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (resp *mcp.JSONRPCResponse, err error) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in handler", "method", req.Method, "panic", r)
					errData, _ := json.Marshal(map[string]interface{}{
						"error_code": "INTERNAL_ERROR",
						"message":    fmt.Sprintf("internal error: %v", r),
						"suggestion": "This is an unexpected error. Please try again or check the tool process logs.",
						"retryable":  true,
					})
					resp = &mcp.JSONRPCResponse{
						JSONRPC: "2.0",
						ID:      req.ID,
						Error: &mcp.JSONRPCError{
							Code:    -32603,
							Message: fmt.Sprintf("internal error: %v", r),
							Data:    errData,
						},
					}
					err = nil
				}
			}()

			resp, err = next(ctx, req)
			if err != nil {
				errData, _ := json.Marshal(map[string]interface{}{
					"error_code": "INTERNAL_ERROR",
					"message":    err.Error(),
					"suggestion": "Check the tool process logs for more details.",
					"retryable":  true,
				})
				return &mcp.JSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error: &mcp.JSONRPCError{
						Code:    -32603,
						Message: err.Error(),
						Data:    errData,
					},
				}, nil
			}
			return resp, nil
		}
	}
}
