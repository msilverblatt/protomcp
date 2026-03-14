//go:build ignore

// Demonstrates tool groups with per-action schemas, validation, and strategies.
// Run: pmcp dev examples/go/tool_groups.go
package main

import (
	"fmt"
	"strings"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	// Union strategy (default): single tool "db" with action discriminator
	protomcp.ToolGroup("db",
		protomcp.GroupDescription("Database operations"),
		protomcp.Action("query",
			protomcp.ActionDescription("Run a read-only SQL query"),
			protomcp.ActionArgs(protomcp.StringArg("sql"), protomcp.IntArg("limit")),
			protomcp.ActionRequires("sql"),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				sql := args["sql"].(string)
				limit := 100
				if v, ok := args["limit"].(float64); ok {
					limit = int(v)
				}
				return protomcp.Result(fmt.Sprintf("Executed: %s (limit %d)", sql, limit))
			}),
		),
		protomcp.Action("insert",
			protomcp.ActionDescription("Insert a record into a table"),
			protomcp.ActionArgs(protomcp.StringArg("table"), protomcp.StringArg("data")),
			protomcp.ActionRequires("table", "data"),
			protomcp.ActionEnumField("table", []string{"users", "events", "logs"}),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				table := args["table"].(string)
				data := args["data"].(string)
				return protomcp.Result(fmt.Sprintf("Inserted into %s: %s", table, data))
			}),
		),
		protomcp.Action("migrate",
			protomcp.ActionDescription("Run a schema migration"),
			protomcp.ActionArgs(protomcp.StringArg("version"), protomcp.BoolArg("dry_run")),
			protomcp.ActionRequires("version"),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				version := args["version"].(string)
				dryRun := false
				if v, ok := args["dry_run"].(bool); ok {
					dryRun = v
				}
				mode := "applied"
				if dryRun {
					mode = "dry run"
				}
				return protomcp.Result(fmt.Sprintf("Migration %s %s", version, mode))
			}),
		),
	)

	// Separate strategy: each action becomes its own tool (files.read, files.write)
	protomcp.ToolGroup("files",
		protomcp.GroupDescription("File operations"),
		protomcp.GroupStrategy("separate"),
		protomcp.Action("read",
			protomcp.ActionDescription("Read a file by path"),
			protomcp.ActionArgs(protomcp.StringArg("path")),
			protomcp.ActionRequires("path"),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				return protomcp.Result(fmt.Sprintf("Contents of %s", args["path"]))
			}),
		),
		protomcp.Action("write",
			protomcp.ActionDescription("Write content to a file"),
			protomcp.ActionArgs(protomcp.StringArg("path"), protomcp.StringArg("content")),
			protomcp.ActionRequires("path", "content"),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				content := args["content"].(string)
				return protomcp.Result(fmt.Sprintf("Wrote %d bytes to %s", len(content), args["path"]))
			}),
		),
		protomcp.Action("search",
			protomcp.ActionDescription("Search files by pattern"),
			protomcp.ActionArgs(protomcp.StringArg("pattern"), protomcp.StringArg("scope")),
			protomcp.ActionRequires("pattern"),
			protomcp.ActionEnumField("scope", []string{"workspace", "project", "global"}),
			protomcp.ActionHandler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
				scope := "workspace"
				if v, ok := args["scope"].(string); ok && v != "" {
					scope = v
				}
				return protomcp.Result(fmt.Sprintf("Searching '%s' in %s", args["pattern"], scope))
			}),
		),
	)

	_ = strings.Join // suppress unused import
	protomcp.Run()
}
