package protomcp

import (
	"sort"
	"sync"
)

// LocalMiddlewareDef holds a registered local (in-process) middleware definition.
type LocalMiddlewareDef struct {
	Priority int
	Handler  func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult
}

var (
	localMwRegistry []LocalMiddlewareDef
	localMwMu       sync.Mutex
)

// LocalMiddleware registers a local middleware that wraps tool handlers.
// Priority determines execution order: lower = outermost.
func LocalMiddleware(priority int, handler func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult) {
	localMwMu.Lock()
	defer localMwMu.Unlock()
	localMwRegistry = append(localMwRegistry, LocalMiddlewareDef{
		Priority: priority,
		Handler:  handler,
	})
}

// GetLocalMiddleware returns middleware sorted by priority (lowest first = outermost).
func GetLocalMiddleware() []LocalMiddlewareDef {
	localMwMu.Lock()
	defer localMwMu.Unlock()
	result := make([]LocalMiddlewareDef, len(localMwRegistry))
	copy(result, localMwRegistry)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority < result[j].Priority
	})
	return result
}

// ClearLocalMiddleware removes all registered local middleware.
func ClearLocalMiddleware() {
	localMwMu.Lock()
	defer localMwMu.Unlock()
	localMwRegistry = nil
}

// BuildMiddlewareChain wraps the handler with all registered local middleware.
// Returns a function with the same signature as the original handler.
func BuildMiddlewareChain(toolName string, handler func(ToolContext, map[string]interface{}) ToolResult) func(ToolContext, map[string]interface{}) ToolResult {
	middlewares := GetLocalMiddleware()
	if len(middlewares) == 0 {
		return handler
	}

	chain := handler
	// Build from innermost (highest priority) to outermost (lowest priority).
	// GetLocalMiddleware returns sorted lowest-first, so we reverse iterate.
	for i := len(middlewares) - 1; i >= 0; i-- {
		mwHandler := middlewares[i].Handler
		nextFn := chain
		chain = func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return mwHandler(ctx, toolName, args, func(c ToolContext, a map[string]interface{}) ToolResult {
				return nextFn(c, a)
			})
		}
	}
	return chain
}
