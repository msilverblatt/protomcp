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
