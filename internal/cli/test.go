package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msilverblatt/protomcp/internal/testengine"
)

// RunTestList starts the engine for the given file, lists tools/resources/prompts,
// and prints output in the specified format ("text" or "json").
func RunTestList(ctx context.Context, file, format string) error {
	eng := testengine.New(file)
	if err := eng.Start(ctx); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}
	defer eng.Stop()

	tools, err := eng.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	resources, err := eng.ListResources(ctx)
	if err != nil {
		return fmt.Errorf("list resources: %w", err)
	}
	prompts, err := eng.ListPrompts(ctx)
	if err != nil {
		return fmt.Errorf("list prompts: %w", err)
	}

	if format == "json" {
		return printListJSON(tools, resources, prompts)
	}

	fmt.Printf("Tools (%d):\n", len(tools))
	fmt.Print(FormatToolTable(tools))
	fmt.Printf("\nResources (%d):\n", len(resources))
	fmt.Print(FormatResourceTable(resources))
	fmt.Printf("\nPrompts (%d):\n", len(prompts))
	fmt.Print(FormatPromptTable(prompts))

	return nil
}

// RunTestCall starts the engine, calls the specified tool, and prints the result.
func RunTestCall(ctx context.Context, file, toolName, argsJSON, format string, showTrace bool) error {
	eng := testengine.New(file)
	if err := eng.Start(ctx); err != nil {
		return fmt.Errorf("failed to start engine: %w", err)
	}
	defer eng.Stop()

	// Parse args JSON
	var args map[string]any
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return fmt.Errorf("invalid --args JSON: %w", err)
		}
	}

	// Call the tool
	result, err := eng.CallTool(ctx, toolName, args)
	if err != nil {
		return fmt.Errorf("call tool %q: %w", toolName, err)
	}

	if format == "json" {
		return printCallJSON(toolName, result, eng, ctx, showTrace)
	}

	// Text output
	fmt.Printf("Tool: %s\n", toolName)
	fmt.Printf("Duration: %s\n", result.Duration)
	if result.Result.IsError {
		fmt.Printf("Status: error\n")
	} else {
		fmt.Printf("Status: ok\n")
	}
	fmt.Printf("\nResult:\n")
	printContent(result.Result.Content)

	if len(result.ToolsEnabled) > 0 {
		fmt.Printf("\nTools enabled: %v\n", result.ToolsEnabled)
	}
	if len(result.ToolsDisabled) > 0 {
		fmt.Printf("\nTools disabled: %v\n", result.ToolsDisabled)
	}

	if showTrace {
		entries := eng.Trace().Entries()
		if len(entries) > 0 {
			fmt.Printf("\nTrace (%d messages):\n", len(entries))
			for _, e := range entries {
				fmt.Printf("  [%s] %s %s\n", e.Timestamp.Format("15:04:05.000"), e.Direction, e.Method)
			}
		}
	}

	return nil
}

func printContent(content []mcp.Content) {
	for _, c := range content {
		switch tc := c.(type) {
		case *mcp.TextContent:
			fmt.Printf("  %s\n", tc.Text)
		default:
			data, _ := json.Marshal(c)
			fmt.Printf("  %s\n", string(data))
		}
	}
}

func printListJSON(tools []*mcp.Tool, resources []*mcp.Resource, prompts []*mcp.Prompt) error {
	out := map[string]any{
		"tools":     tools,
		"resources": resources,
		"prompts":   prompts,
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printCallJSON(toolName string, result *testengine.CallResult, eng *testengine.Engine, ctx context.Context, showTrace bool) error {
	out := map[string]any{
		"tool":     toolName,
		"duration": result.Duration.String(),
		"isError":  result.Result.IsError,
		"content":  result.Result.Content,
	}
	if len(result.ToolsEnabled) > 0 {
		out["toolsEnabled"] = result.ToolsEnabled
	}
	if len(result.ToolsDisabled) > 0 {
		out["toolsDisabled"] = result.ToolsDisabled
	}
	if showTrace {
		out["trace"] = eng.Trace().Entries()
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
