package protomcp

// CompletionResult holds the result of a completion handler.
type CompletionResult struct {
	Values  []string
	Total   int32
	HasMore bool
}

// completionKey uniquely identifies a completion handler.
type completionKey struct {
	RefType string
	RefName string
	ArgName string
}

// CompletionHandler takes the current argument value and returns completions.
type CompletionHandler func(argumentValue string) CompletionResult

var completionRegistry = map[completionKey]CompletionHandler{}

// RegisterCompletion registers a completion handler keyed by (refType, refName, argName).
func RegisterCompletion(refType, refName, argName string, handler CompletionHandler) {
	completionRegistry[completionKey{RefType: refType, RefName: refName, ArgName: argName}] = handler
}

// GetCompletionHandler looks up a completion handler by key.
func GetCompletionHandler(refType, refName, argName string) (CompletionHandler, bool) {
	h, ok := completionRegistry[completionKey{RefType: refType, RefName: refName, ArgName: argName}]
	return h, ok
}

// ClearCompletionRegistry resets the completion registry.
func ClearCompletionRegistry() {
	completionRegistry = map[completionKey]CompletionHandler{}
}
