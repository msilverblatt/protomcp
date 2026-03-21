package protomcp

import (
	"testing"
)

func TestHiddenToolDefault(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()

	Tool("visible_tool",
		Description("A visible tool"),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("ok")
		}),
	)

	tools := GetRegisteredTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Hidden {
		t.Error("tool should not be hidden by default")
	}
}

func TestHiddenToolExplicitTrue(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()

	Tool("hidden_tool",
		Description("A hidden tool"),
		HiddenHint(true),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("secret")
		}),
	)

	tools := GetRegisteredTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if !tools[0].Hidden {
		t.Error("tool should be hidden when HiddenHint(true) is set")
	}
}

func TestHiddenToolExplicitFalse(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()

	Tool("not_hidden",
		Description("Explicitly not hidden"),
		HiddenHint(false),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("visible")
		}),
	)

	tools := GetRegisteredTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Hidden {
		t.Error("tool should not be hidden when HiddenHint(false) is set")
	}
}

func TestHiddenToolsCollectedFromRegistry(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()

	Tool("visible1",
		Description("Visible tool 1"),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("ok")
		}),
	)
	Tool("hidden1",
		Description("Hidden tool 1"),
		HiddenHint(true),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("secret")
		}),
	)
	Tool("visible2",
		Description("Visible tool 2"),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("ok")
		}),
	)
	Tool("hidden2",
		Description("Hidden tool 2"),
		HiddenHint(true),
		Handler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
			return Result("secret")
		}),
	)

	tools := GetRegisteredTools()
	if len(tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(tools))
	}

	var hiddenNames []string
	for _, td := range tools {
		if td.Hidden {
			hiddenNames = append(hiddenNames, td.Name)
		}
	}

	if len(hiddenNames) != 2 {
		t.Fatalf("expected 2 hidden tools, got %d: %v", len(hiddenNames), hiddenNames)
	}

	found1, found2 := false, false
	for _, name := range hiddenNames {
		if name == "hidden1" {
			found1 = true
		}
		if name == "hidden2" {
			found2 = true
		}
	}
	if !found1 {
		t.Error("hidden1 should be in hidden tools list")
	}
	if !found2 {
		t.Error("hidden2 should be in hidden tools list")
	}
}
