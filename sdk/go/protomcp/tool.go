package protomcp

import (
	"encoding/json"
)

type ToolDef struct {
	Name         string
	Desc         string
	InputSchema  map[string]interface{}
	OutputSchema map[string]interface{}
	HandlerFn    func(ToolContext, map[string]interface{}) ToolResult
	Title        string
	Destructive  bool
	Idempotent   bool
	ReadOnly     bool
	OpenWorld    bool
	TaskSupport  bool
}

type ToolOption func(*ToolDef)

var registry []ToolDef

func Tool(name string, opts ...ToolOption) {
	td := ToolDef{
		Name:        name,
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}
	for _, opt := range opts {
		opt(&td)
	}
	registry = append(registry, td)
}

func Description(desc string) ToolOption {
	return func(td *ToolDef) { td.Desc = desc }
}

type ArgDef struct {
	Name string
	Type string
}

func IntArg(name string) ArgDef  { return ArgDef{Name: name, Type: "integer"} }
func StrArg(name string) ArgDef  { return ArgDef{Name: name, Type: "string"} }
func NumArg(name string) ArgDef  { return ArgDef{Name: name, Type: "number"} }
func BoolArg(name string) ArgDef { return ArgDef{Name: name, Type: "boolean"} }

func Args(args ...ArgDef) ToolOption {
	return func(td *ToolDef) {
		props := map[string]interface{}{}
		required := []string{}
		for _, a := range args {
			props[a.Name] = map[string]interface{}{"type": a.Type}
			required = append(required, a.Name)
		}
		td.InputSchema = map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}
}

func Handler(fn func(ToolContext, map[string]interface{}) ToolResult) ToolOption {
	return func(td *ToolDef) { td.HandlerFn = fn }
}

func Title(v string) ToolOption                { return func(td *ToolDef) { td.Title = v } }
func DestructiveHint(v bool) ToolOption { return func(td *ToolDef) { td.Destructive = v } }
func IdempotentHint(v bool) ToolOption  { return func(td *ToolDef) { td.Idempotent = v } }
func ReadOnlyHint(v bool) ToolOption    { return func(td *ToolDef) { td.ReadOnly = v } }
func OpenWorldHint(v bool) ToolOption   { return func(td *ToolDef) { td.OpenWorld = v } }
func TaskSupportHint(v bool) ToolOption { return func(td *ToolDef) { td.TaskSupport = v } }

func (td ToolDef) InputSchemaJSON() string {
	b, _ := json.Marshal(td.InputSchema)
	return string(b)
}

func GetRegisteredTools() []ToolDef { return append([]ToolDef{}, registry...) }
func ClearRegistry()                { registry = nil }
