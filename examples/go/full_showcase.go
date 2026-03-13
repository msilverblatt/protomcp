//go:build ignore

// Full showcase: structured output, dynamic tools, metadata, error handling.
// Run: pmcp dev examples/go/full_showcase.go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	protomcp.Tool("calculator",
		protomcp.Description("Perform arithmetic operations with structured output"),
		protomcp.Args(
			protomcp.NumArg("a"),
			protomcp.NumArg("b"),
			protomcp.StrArg("operation"),
		),
		protomcp.IdempotentHint(true),
		protomcp.ReadOnlyHint(true),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			a := args["a"].(float64)
			b := args["b"].(float64)
			op := args["operation"].(string)

			var result float64
			switch op {
			case "add":
				result = a + b
			case "subtract":
				result = a - b
			case "multiply":
				result = a * b
			case "divide":
				if b == 0 {
					return protomcp.ErrorResult("division by zero", "INVALID_INPUT", "provide a non-zero divisor", false)
				}
				result = a / b
			default:
				return protomcp.ErrorResult(
					fmt.Sprintf("unknown operation: %s", op),
					"INVALID_INPUT",
					"use add, subtract, multiply, or divide",
					false,
				)
			}

			output, _ := json.Marshal(map[string]interface{}{
				"result":    result,
				"operation": op,
				"operands":  []float64{a, b},
			})
			return protomcp.Result(string(output))
		}),
	)

	protomcp.Tool("enable_admin",
		protomcp.Description("Enable admin tools"),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			r := protomcp.Result("admin tools enabled")
			r.EnableTools = []string{"admin_panel"}
			return r
		}),
	)

	protomcp.Tool("admin_panel",
		protomcp.Description("Admin panel — only available after enable_admin"),
		protomcp.DestructiveHint(true),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			protomcp.Log.Warning("admin panel accessed")
			return protomcp.Result("admin action performed")
		}),
	)

	protomcp.Run()
}
