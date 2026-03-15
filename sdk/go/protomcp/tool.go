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
	Hidden       bool
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
	Name       string
	Type       string
	ItemType   string   // for array
	UnionTypes []string // for union
	EnumValues []string // for literal
}

func IntArg(name string) ArgDef  { return ArgDef{Name: name, Type: "integer"} }
func StrArg(name string) ArgDef  { return ArgDef{Name: name, Type: "string"} }
func NumArg(name string) ArgDef  { return ArgDef{Name: name, Type: "number"} }
func BoolArg(name string) ArgDef { return ArgDef{Name: name, Type: "boolean"} }

func ArrayArg(name string, itemType string) ArgDef {
	return ArgDef{Name: name, Type: "array", ItemType: itemType}
}

func ObjectArg(name string) ArgDef {
	return ArgDef{Name: name, Type: "object"}
}

func UnionArg(name string, types ...string) ArgDef {
	return ArgDef{Name: name, Type: "union", UnionTypes: types}
}

func LiteralArg(name string, values ...string) ArgDef {
	return ArgDef{Name: name, Type: "literal", EnumValues: values}
}

func argToSchema(a ArgDef) map[string]interface{} {
	switch {
	case a.Type == "array":
		schema := map[string]interface{}{"type": "array"}
		if a.ItemType != "" {
			schema["items"] = map[string]interface{}{"type": a.ItemType}
		}
		return schema
	case a.Type == "union" && len(a.UnionTypes) > 0:
		anyOf := make([]interface{}, len(a.UnionTypes))
		for i, t := range a.UnionTypes {
			anyOf[i] = map[string]interface{}{"type": t}
		}
		return map[string]interface{}{"anyOf": anyOf}
	case a.Type == "literal" && len(a.EnumValues) > 0:
		vals := make([]interface{}, len(a.EnumValues))
		for i, v := range a.EnumValues {
			vals[i] = v
		}
		return map[string]interface{}{"type": "string", "enum": vals}
	default:
		return map[string]interface{}{"type": a.Type}
	}
}

func Args(args ...ArgDef) ToolOption {
	return func(td *ToolDef) {
		props := map[string]interface{}{}
		required := []string{}
		for _, a := range args {
			props[a.Name] = argToSchema(a)
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
func HiddenHint(h bool) ToolOption      { return func(d *ToolDef) { d.Hidden = h } }

func (td ToolDef) InputSchemaJSON() string {
	b, _ := json.Marshal(td.InputSchema)
	return string(b)
}

func GetRegisteredTools() []ToolDef {
	all := append([]ToolDef{}, registry...)
	all = append(all, GroupsToToolDefs()...)
	all = append(all, WorkflowsToToolDefs()...)
	return all
}
func ClearRegistry()                { registry = nil }
