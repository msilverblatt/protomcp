package protomcp

import "sync"

type ContextDef struct {
	ParamName string
	Resolver  func(args map[string]interface{}) interface{}
	Expose    bool
}

type ContextOption func(*ContextDef)

var (
	contextRegistry []ContextDef
	contextMu       sync.Mutex
)

// ServerContext registers a context resolver that injects a value into tool handlers.
func ServerContext(paramName string, resolver func(args map[string]interface{}) interface{}, opts ...ContextOption) {
	contextMu.Lock()
	defer contextMu.Unlock()
	cd := ContextDef{
		ParamName: paramName,
		Resolver:  resolver,
		Expose:    true,
	}
	for _, opt := range opts {
		opt(&cd)
	}
	contextRegistry = append(contextRegistry, cd)
}

// Expose sets whether the param appears in tool schemas.
func Expose(v bool) ContextOption {
	return func(cd *ContextDef) { cd.Expose = v }
}

// ResolveContexts runs all registered context resolvers against args. Returns resolved values.
func ResolveContexts(args map[string]interface{}) map[string]interface{} {
	contextMu.Lock()
	defs := make([]ContextDef, len(contextRegistry))
	copy(defs, contextRegistry)
	contextMu.Unlock()

	resolved := make(map[string]interface{})
	for _, cd := range defs {
		resolved[cd.ParamName] = cd.Resolver(args)
	}
	return resolved
}

// GetHiddenContextParams returns param names that should NOT appear in tool schemas (expose=false).
func GetHiddenContextParams() map[string]bool {
	contextMu.Lock()
	defer contextMu.Unlock()
	hidden := make(map[string]bool)
	for _, cd := range contextRegistry {
		if !cd.Expose {
			hidden[cd.ParamName] = true
		}
	}
	return hidden
}

// ClearContextRegistry removes all registered contexts.
func ClearContextRegistry() {
	contextMu.Lock()
	defer contextMu.Unlock()
	contextRegistry = nil
}
