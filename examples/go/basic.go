//go:build ignore

// A minimal protomcp tool — adds and multiplies numbers.
// Run: pmcp dev examples/go/basic.go
package main

import (
	"fmt"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	protomcp.Tool("add",
		protomcp.Description("Add two numbers"),
		protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			a := int(args["a"].(float64))
			b := int(args["b"].(float64))
			return protomcp.Result(fmt.Sprintf("%d", a+b))
		}),
	)

	protomcp.Tool("multiply",
		protomcp.Description("Multiply two numbers"),
		protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			a := int(args["a"].(float64))
			b := int(args["b"].(float64))
			return protomcp.Result(fmt.Sprintf("%d", a*b))
		}),
	)

	protomcp.Run()
}
