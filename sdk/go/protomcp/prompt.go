package protomcp

import "encoding/json"

// PromptArg describes a single argument for a prompt.
type PromptArg struct {
	Name        string
	Description string
	Required    bool
}

// PromptMessage represents a single message returned by a prompt handler.
type PromptMessage struct {
	Role        string // "user" or "assistant"
	ContentJSON string
}

// UserMessage creates a PromptMessage with role "user" and text content.
func UserMessage(text string) PromptMessage {
	return PromptMessage{
		Role:        "user",
		ContentJSON: `{"type":"text","text":` + jsonEscapeString(text) + `}`,
	}
}

// AssistantMessage creates a PromptMessage with role "assistant" and text content.
func AssistantMessage(text string) PromptMessage {
	return PromptMessage{
		Role:        "assistant",
		ContentJSON: `{"type":"text","text":` + jsonEscapeString(text) + `}`,
	}
}

func jsonEscapeString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// PromptDef represents a registered prompt.
type PromptDef struct {
	Name        string
	Description string
	Arguments   []PromptArg
	HandlerFn   func(args map[string]string) (string, []PromptMessage)
}

var promptRegistry []PromptDef

// RegisterPrompt adds a prompt to the global registry.
func RegisterPrompt(def PromptDef) {
	promptRegistry = append(promptRegistry, def)
}

// GetRegisteredPrompts returns a copy of all registered prompts.
func GetRegisteredPrompts() []PromptDef {
	return append([]PromptDef{}, promptRegistry...)
}

// ClearPromptRegistry resets the prompt registry.
func ClearPromptRegistry() {
	promptRegistry = nil
}

// Prompt is a convenience function to register a prompt with a handler.
func Prompt(name, description string, args []PromptArg, handler func(args map[string]string) (string, []PromptMessage)) {
	RegisterPrompt(PromptDef{
		Name:        name,
		Description: description,
		Arguments:   args,
		HandlerFn:   handler,
	})
}
