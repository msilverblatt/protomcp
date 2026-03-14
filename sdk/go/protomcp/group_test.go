package protomcp

import (
	"context"
	"strings"
	"testing"
)

func dummyToolContext() ToolContext {
	return ToolContext{Ctx: context.Background()}
}

func TestGroupRegistration(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("math",
		GroupDescription("Math operations"),
		Action("add",
			ActionDescription("Add two numbers"),
			ActionArgs(IntArg("a"), IntArg("b")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("sum")
			}),
		),
		Action("multiply",
			ActionDescription("Multiply two numbers"),
			ActionArgs(IntArg("x"), IntArg("y")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("product")
			}),
		),
	)

	groups := GetRegisteredGroups()
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "math" {
		t.Errorf("expected name 'math', got '%s'", groups[0].Name)
	}
	if groups[0].Description != "Math operations" {
		t.Errorf("expected description 'Math operations', got '%s'", groups[0].Description)
	}
	if len(groups[0].Actions) != 2 {
		t.Errorf("expected 2 actions, got %d", len(groups[0].Actions))
	}
}

func TestUnionStrategySchema(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("db",
		GroupDescription("DB ops"),
		Action("query",
			ActionDescription("Run query"),
			ActionArgs(StrArg("sql")),
		),
		Action("insert",
			ActionDescription("Insert record"),
			ActionArgs(StrArg("table"), ObjectArg("data")),
		),
	)

	defs := GroupsToToolDefs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}
	td := defs[0]
	if td.Name != "db" {
		t.Errorf("expected name 'db', got '%s'", td.Name)
	}

	schema := td.InputSchema
	props, _ := schema["properties"].(map[string]interface{})
	actionProp, _ := props["action"].(map[string]interface{})
	enumVals, _ := actionProp["enum"].([]interface{})
	if len(enumVals) != 2 {
		t.Fatalf("expected 2 enum values, got %d", len(enumVals))
	}

	oneOf, _ := schema["oneOf"].([]interface{})
	if len(oneOf) != 2 {
		t.Fatalf("expected 2 oneOf entries, got %d", len(oneOf))
	}

	// Verify first entry has action const and sql
	entry0 := oneOf[0].(map[string]interface{})
	entryProps0 := entry0["properties"].(map[string]interface{})
	actionConst0 := entryProps0["action"].(map[string]interface{})
	if actionConst0["const"] != "query" {
		t.Errorf("expected first action const 'query', got '%v'", actionConst0["const"])
	}
	if _, ok := entryProps0["sql"]; !ok {
		t.Error("expected 'sql' in query properties")
	}
}

func TestSeparateStrategySchema(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("files",
		GroupDescription("File ops"),
		GroupStrategy("separate"),
		Action("read",
			ActionDescription("Read a file"),
			ActionArgs(StrArg("path")),
		),
		Action("write",
			ActionDescription("Write a file"),
			ActionArgs(StrArg("path"), StrArg("content")),
		),
	)

	defs := GroupsToToolDefs()
	if len(defs) != 2 {
		t.Fatalf("expected 2 tool defs, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["files.read"] {
		t.Error("expected 'files.read' tool def")
	}
	if !names["files.write"] {
		t.Error("expected 'files.write' tool def")
	}
}

func TestDispatchCorrectAction(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("calc",
		Action("add",
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				a := int(args["a"].(float64))
				b := int(args["b"].(float64))
				return Result(strings.Repeat("x", a+b))
			}),
			ActionArgs(IntArg("a"), IntArg("b")),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "add",
		"a":      float64(3),
		"b":      float64(4),
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
	if result.ResultText != "xxxxxxx" {
		t.Errorf("expected 7 x's, got '%s'", result.ResultText)
	}
}

func TestDispatchUnknownAction(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("calc2",
		Action("add",
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("ok")
			}),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "ad",
	})
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
	if !strings.Contains(result.ResultText, "Unknown action") {
		t.Errorf("expected 'Unknown action' in result, got '%s'", result.ResultText)
	}
	if !strings.Contains(result.ResultText, "add") {
		t.Errorf("expected 'add' suggestion in result, got '%s'", result.ResultText)
	}
}

func TestDispatchMissingAction(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("calc3",
		Action("add",
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("ok")
			}),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for missing action")
	}
	if !strings.Contains(result.ResultText, "Missing") {
		t.Errorf("expected 'Missing' in result, got '%s'", result.ResultText)
	}
}

func TestGroupsInGetRegisteredTools(t *testing.T) {
	ClearRegistry()
	ClearGroupRegistry()
	defer func() {
		ClearRegistry()
		ClearGroupRegistry()
	}()

	ToolGroup("tools_test",
		GroupDescription("Test group"),
		Action("ping",
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("pong")
			}),
		),
	)

	tools := GetRegisteredTools()
	found := false
	for _, td := range tools {
		if td.Name == "tools_test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'tools_test' in registered tools")
	}
}

func TestUnionHandlerDispatch(t *testing.T) {
	ClearGroupRegistry()
	defer ClearGroupRegistry()

	ToolGroup("handler_test",
		Action("greet",
			ActionArgs(StrArg("name")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("Hello, " + args["name"].(string) + "!")
			}),
		),
	)

	defs := GroupsToToolDefs()
	ctx := dummyToolContext()
	result := defs[0].HandlerFn(ctx, map[string]interface{}{
		"action": "greet",
		"name":   "World",
	})
	if result.ResultText != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got '%s'", result.ResultText)
	}
}

func TestValidationRequires(t *testing.T) {
	ClearGroupRegistry()
	ClearContextRegistry()
	defer ClearGroupRegistry()
	defer ClearContextRegistry()

	ToolGroup("val_req",
		Action("create",
			ActionRequires("name", "email"),
			ActionArgs(StrArg("name"), StrArg("email")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("created")
			}),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()

	// Missing required field
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
	})
	if !result.IsError {
		t.Error("expected error for missing email")
	}
	if !strings.Contains(result.ResultText, "email") {
		t.Errorf("expected 'email' in error, got '%s'", result.ResultText)
	}
	if result.ErrorCode != "MISSING_REQUIRED" {
		t.Errorf("expected error code MISSING_REQUIRED, got '%s'", result.ErrorCode)
	}

	// Empty string counts as missing
	result = DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"email":  "",
	})
	if !result.IsError {
		t.Error("expected error for empty email")
	}

	// All fields present
	result = DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "create",
		"name":   "Alice",
		"email":  "alice@example.com",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
	if result.ResultText != "created" {
		t.Errorf("expected 'created', got '%s'", result.ResultText)
	}
}

func TestValidationEnumField(t *testing.T) {
	ClearGroupRegistry()
	ClearContextRegistry()
	defer ClearGroupRegistry()
	defer ClearContextRegistry()

	ToolGroup("val_enum",
		Action("set",
			ActionEnumField("mode", []string{"fast", "slow", "balanced"}),
			ActionArgs(StrArg("mode")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("set:" + args["mode"].(string))
			}),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()

	// Invalid enum value
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "set",
		"mode":   "fats",
	})
	if !result.IsError {
		t.Error("expected error for invalid enum")
	}
	if result.ErrorCode != "INVALID_ENUM" {
		t.Errorf("expected INVALID_ENUM, got '%s'", result.ErrorCode)
	}
	if !strings.Contains(result.Suggestion, "fast") {
		t.Errorf("expected fuzzy suggestion 'fast', got '%s'", result.Suggestion)
	}

	// Valid enum value
	result = DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "set",
		"mode":   "fast",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
	if result.ResultText != "set:fast" {
		t.Errorf("expected 'set:fast', got '%s'", result.ResultText)
	}
}

func TestValidationCrossRule(t *testing.T) {
	ClearGroupRegistry()
	ClearContextRegistry()
	defer ClearGroupRegistry()
	defer ClearContextRegistry()

	ToolGroup("val_cross",
		Action("transfer",
			ActionCrossRule(func(args map[string]interface{}) bool {
				from, _ := args["from"].(string)
				to, _ := args["to"].(string)
				return from == to
			}, "Cannot transfer to the same account"),
			ActionArgs(StrArg("from"), StrArg("to")),
			ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
				return Result("transferred")
			}),
		),
	)

	groups := GetRegisteredGroups()
	ctx := dummyToolContext()

	// Cross rule violation
	result := DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "transfer",
		"from":   "acct1",
		"to":     "acct1",
	})
	if !result.IsError {
		t.Error("expected cross rule error")
	}
	if result.ErrorCode != "CROSS_PARAM_VIOLATION" {
		t.Errorf("expected CROSS_PARAM_VIOLATION, got '%s'", result.ErrorCode)
	}

	// No violation
	result = DispatchGroupAction(groups[0], ctx, map[string]interface{}{
		"action": "transfer",
		"from":   "acct1",
		"to":     "acct2",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
}
