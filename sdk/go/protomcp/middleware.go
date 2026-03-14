package protomcp

import "sync"

// MiddlewareDef holds a registered middleware definition.
type MiddlewareDef struct {
	Name     string
	Priority int32
	Handler  func(phase, toolName, argsJSON, resultJSON string, isError bool) map[string]interface{}
}

var (
	middlewareRegistry []MiddlewareDef
	middlewareMu       sync.Mutex
)

// Middleware registers a custom middleware handler that intercepts tool calls.
// Priority determines execution order: lower numbers run first in the before phase.
func Middleware(name string, priority int32, handler func(phase, toolName, argsJSON, resultJSON string, isError bool) map[string]interface{}) {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()
	middlewareRegistry = append(middlewareRegistry, MiddlewareDef{
		Name:     name,
		Priority: priority,
		Handler:  handler,
	})
}

// GetRegisteredMiddleware returns a copy of the middleware registry.
func GetRegisteredMiddleware() []MiddlewareDef {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()
	result := make([]MiddlewareDef, len(middlewareRegistry))
	copy(result, middlewareRegistry)
	return result
}

// ClearMiddlewareRegistry removes all registered middleware.
func ClearMiddlewareRegistry() {
	middlewareMu.Lock()
	defer middlewareMu.Unlock()
	middlewareRegistry = nil
}
