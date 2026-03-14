package protomcp_test

import (
	"testing"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func TestToolRegistration(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("add",
		protomcp.Description("Add two numbers"),
		protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("3")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "add" {
		t.Errorf("name = %q, want %q", tools[0].Name, "add")
	}
	if tools[0].Desc != "Add two numbers" {
		t.Errorf("description = %q, want %q", tools[0].Desc, "Add two numbers")
	}
}

func TestArrayArg(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("list_items",
		protomcp.Description("List items"),
		protomcp.Args(protomcp.ArrayArg("tags", "string")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("ok")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]interface{})
	tagsProp := props["tags"].(map[string]interface{})
	if tagsProp["type"] != "array" {
		t.Errorf("expected type=array, got %v", tagsProp["type"])
	}
	items := tagsProp["items"].(map[string]interface{})
	if items["type"] != "string" {
		t.Errorf("expected items type=string, got %v", items["type"])
	}
}

func TestObjectArg(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("set_config",
		protomcp.Description("Set config"),
		protomcp.Args(protomcp.ObjectArg("config")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("ok")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]interface{})
	configProp := props["config"].(map[string]interface{})
	if configProp["type"] != "object" {
		t.Errorf("expected type=object, got %v", configProp["type"])
	}
}

func TestUnionArg(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("process",
		protomcp.Description("Process data"),
		protomcp.Args(protomcp.UnionArg("data", "string", "object")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("ok")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]interface{})
	dataProp := props["data"].(map[string]interface{})
	anyOf := dataProp["anyOf"].([]interface{})
	if len(anyOf) != 2 {
		t.Fatalf("expected 2 anyOf entries, got %d", len(anyOf))
	}
	first := anyOf[0].(map[string]interface{})
	if first["type"] != "string" {
		t.Errorf("expected first type=string, got %v", first["type"])
	}
	second := anyOf[1].(map[string]interface{})
	if second["type"] != "object" {
		t.Errorf("expected second type=object, got %v", second["type"])
	}
}

func TestLiteralArg(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("set_mode",
		protomcp.Description("Set mode"),
		protomcp.Args(protomcp.LiteralArg("mode", "fast", "slow", "balanced")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("ok")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	schema := tools[0].InputSchema
	props := schema["properties"].(map[string]interface{})
	modeProp := props["mode"].(map[string]interface{})
	if modeProp["type"] != "string" {
		t.Errorf("expected type=string, got %v", modeProp["type"])
	}
	enumVals := modeProp["enum"].([]interface{})
	if len(enumVals) != 3 {
		t.Fatalf("expected 3 enum values, got %d", len(enumVals))
	}
	expected := []string{"fast", "slow", "balanced"}
	for i, v := range enumVals {
		if v != expected[i] {
			t.Errorf("enum[%d] = %v, want %v", i, v, expected[i])
		}
	}
}

func TestToolMetadata(t *testing.T) {
	protomcp.ClearRegistry()
	protomcp.Tool("delete_user",
		protomcp.Description("Delete a user account"),
		protomcp.DestructiveHint(true),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			return protomcp.Result("deleted")
		}),
	)

	tools := protomcp.GetRegisteredTools()
	if !tools[0].Destructive {
		t.Error("expected destructive hint")
	}
}
