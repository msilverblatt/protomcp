// Benchmark tool fixture for protomcp — Go implementation.
// Provides echo, add, compute, generate, parse_json tools.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	protomcp.Tool("echo",
		protomcp.Description("Echo the input back"),
		protomcp.Args(protomcp.StrArg("message")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			msg, _ := args["message"].(string)
			return protomcp.Result(msg)
		}),
	)

	protomcp.Tool("add",
		protomcp.Description("Add two numbers"),
		protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			a := int(args["a"].(float64))
			b := int(args["b"].(float64))
			return protomcp.Result(fmt.Sprintf("%d", a+b))
		}),
	)

	protomcp.Tool("compute",
		protomcp.Description("CPU-bound work: hash a string N times"),
		protomcp.Args(protomcp.IntArg("iterations")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			iters := int(args["iterations"].(float64))
			result := "seed"
			for i := 0; i < iters; i++ {
				h := sha256.Sum256([]byte(result))
				result = fmt.Sprintf("%x", h)
			}
			return protomcp.Result(result)
		}),
	)

	protomcp.Tool("generate",
		protomcp.Description("Return a string of the requested size in bytes"),
		protomcp.Args(protomcp.IntArg("size")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			size := int(args["size"].(float64))
			return protomcp.Result(strings.Repeat("X", size))
		}),
	)

	protomcp.Tool("parse_json",
		protomcp.Description("Parse JSON and return it serialized back"),
		protomcp.Args(protomcp.StrArg("data")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			data, _ := args["data"].(string)
			var parsed interface{}
			json.Unmarshal([]byte(data), &parsed)
			out, _ := json.Marshal(parsed)
			return protomcp.Result(string(out))
		}),
	)

	protomcp.Run()
}
