//go:build ignore

// File search tool with progress, cancellation, and logging.
// Run: pmcp dev examples/go/real_world.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	protomcp.Tool("search_files",
		protomcp.Description("Search for files matching a pattern in a directory"),
		protomcp.Args(protomcp.StrArg("directory"), protomcp.StrArg("pattern")),
		protomcp.ReadOnlyHint(true),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			dir := args["directory"].(string)
			pattern := args["pattern"].(string)

			protomcp.Log.Info(fmt.Sprintf("searching %s for %s", dir, pattern))

			var matches []string
			var total int64

			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				total++

				if ctx.IsCancelled() {
					return fmt.Errorf("cancelled")
				}

				if strings.Contains(info.Name(), pattern) {
					matches = append(matches, path)
				}
				return nil
			})

			protomcp.Log.Info(fmt.Sprintf("found %d matches in %d files", len(matches), total))
			return protomcp.Result(strings.Join(matches, "\n"))
		}),
	)

	protomcp.Run()
}
