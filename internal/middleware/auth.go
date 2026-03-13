package middleware

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

type authContextKey string

const (
	authHeaderKey   authContextKey = "auth-header"
	apiKeyHeaderKey authContextKey = "apikey-header"
)

// WithAuthHeader adds an Authorization header value to the context.
func WithAuthHeader(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, authHeaderKey, value)
}

// WithAPIKeyHeader adds an X-API-Key header value to the context.
func WithAPIKeyHeader(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, apiKeyHeaderKey, value)
}

// GetAuthHeader retrieves the Authorization header from context.
func GetAuthHeader(ctx context.Context) string {
	v, _ := ctx.Value(authHeaderKey).(string)
	return v
}

// GetAPIKeyHeader retrieves the X-API-Key header from context.
func GetAPIKeyHeader(ctx context.Context) string {
	v, _ := ctx.Value(apiKeyHeaderKey).(string)
	return v
}

type authChecker struct {
	scheme string
	value  string
}

// NewAuth creates an auth middleware from --auth flag values.
// Each value must be "token:ENV_VAR" or "apikey:ENV_VAR".
// Returns error if env var is not set.
func NewAuth(authSpecs []string) (Middleware, error) {
	var checkers []authChecker

	for _, spec := range authSpecs {
		parts := strings.SplitN(spec, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid auth spec %q", spec)
		}
		scheme, envVar := parts[0], parts[1]
		value := os.Getenv(envVar)
		if value == "" {
			return nil, fmt.Errorf("environment variable %q is not set (required by --auth %s)", envVar, spec)
		}
		checkers = append(checkers, authChecker{scheme: scheme, value: value})
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			for _, c := range checkers {
				switch c.scheme {
				case "token":
					header := GetAuthHeader(ctx)
					expected := "Bearer " + c.value
					if header != expected {
						return nil, fmt.Errorf("unauthorized: invalid or missing Bearer token")
					}
				case "apikey":
					header := GetAPIKeyHeader(ctx)
					if header != c.value {
						return nil, fmt.Errorf("unauthorized: invalid or missing API key")
					}
				}
			}
			return next(ctx, req)
		}
	}, nil
}
