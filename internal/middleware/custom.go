package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/mcp"
)

// MiddlewareDispatcher sends intercept requests to the tool process.
type MiddlewareDispatcher interface {
	SendMiddlewareIntercept(ctx context.Context, mwName, phase, toolName, argsJSON, resultJSON string, isError bool) (*pb.MiddlewareInterceptResponse, error)
}

// RegisteredMW holds a registered custom middleware from the tool process.
type RegisteredMW struct {
	Name     string
	Priority int32
}

// CustomMiddleware creates a middleware that dispatches to tool-process-registered middleware.
func CustomMiddleware(dispatcher MiddlewareDispatcher, registered []RegisteredMW) Middleware {
	if len(registered) == 0 {
		return func(next Handler) Handler { return next }
	}

	// Sort by priority (lower first)
	sorted := make([]RegisteredMW, len(registered))
	copy(sorted, registered)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	return func(next Handler) Handler {
		return func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			// For non-tool-call requests, skip custom middleware
			if req.Method != "tools/call" {
				return next(ctx, req)
			}

			toolName, argsJSON := extractToolCallParams(req)

			// Before phase
			currentArgs := argsJSON
			for _, mw := range sorted {
				resp, err := dispatcher.SendMiddlewareIntercept(ctx, mw.Name, "before", toolName, currentArgs, "", false)
				if err != nil {
					return nil, fmt.Errorf("middleware %q before: %w", mw.Name, err)
				}
				if resp.GetReject() {
					return nil, fmt.Errorf("rejected by middleware %q: %s", mw.Name, resp.GetRejectReason())
				}
				if modified := resp.GetArgumentsJson(); modified != "" {
					currentArgs = modified
				}
			}

			// Inject modified args back into the request
			if currentArgs != argsJSON {
				modifiedParams, _ := json.Marshal(map[string]json.RawMessage{
					"name":      json.RawMessage(fmt.Sprintf("%q", toolName)),
					"arguments": json.RawMessage(currentArgs),
				})
				req.Params = modifiedParams
			}

			result, err := next(ctx, req)

			// After phase (reverse order)
			resultJSON := ""
			isError := false
			if result != nil {
				resultJSON = string(result.Result)
				isError = result.Error != nil
			}

			for i := len(sorted) - 1; i >= 0; i-- {
				mw := sorted[i]
				resp, afterErr := dispatcher.SendMiddlewareIntercept(ctx, mw.Name, "after", toolName, currentArgs, resultJSON, isError)
				if afterErr != nil {
					fmt.Fprintf(os.Stderr, "[protomcp] middleware %q after-phase error: %v\n", mw.Name, afterErr)
					return result, err
				}
				if modified := resp.GetResultJson(); modified != "" {
					resultJSON = modified
				}
			}

			return result, err
		}
	}
}

func extractToolCallParams(req mcp.JSONRPCRequest) (string, string) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return "", "{}"
	}
	argsJSON := "{}"
	if len(params.Arguments) > 0 {
		argsJSON = string(params.Arguments)
	}
	return params.Name, argsJSON
}
