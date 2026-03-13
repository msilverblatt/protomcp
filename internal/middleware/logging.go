package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// Logging returns middleware that logs each request and response.
func Logging(logger *slog.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			start := time.Now()
			logger.Debug("request", "method", req.Method, "id", string(req.ID))

			resp, err := next(ctx, req)
			duration := time.Since(start)

			if err != nil {
				logger.Error("request failed",
					"method", req.Method,
					"id", string(req.ID),
					"duration", duration,
					"error", err,
				)
			} else if resp != nil && resp.Error != nil {
				logger.Warn("request error response",
					"method", req.Method,
					"id", string(req.ID),
					"duration", duration,
					"error_code", resp.Error.Code,
					"error_message", resp.Error.Message,
				)
			} else {
				logger.Debug("response",
					"method", req.Method,
					"id", string(req.ID),
					"duration", duration,
				)
			}

			return resp, err
		}
	}
}
