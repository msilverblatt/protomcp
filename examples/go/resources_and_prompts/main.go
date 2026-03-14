// examples/go/resources_and_prompts.go
// Demonstrates resources, prompts, and completions — the full MCP feature set.
// Run: pmcp dev examples/go/resources_and_prompts.go
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

var notes = map[string]string{
	"meeting-2024-01-15": "Discussed Q1 roadmap. Action items: hire 2 engineers.",
	"meeting-2024-01-22": "Sprint review. Shipped auth system. Demo went well.",
	"meeting-2024-02-01": "Retrospective. Need better test coverage.",
}

func main() {
	// ─── Resources ───────────────────────────────────────────────────────

	protomcp.RegisterResource(protomcp.ResourceDef{
		URI:         "notes://index",
		Name:        "notes_index",
		Description: "List of all available meeting notes",
		MimeType:    "application/json",
		HandlerFn: func() []protomcp.ResourceContent {
			ids := make([]string, 0, len(notes))
			for id := range notes {
				ids = append(ids, id)
			}
			data, _ := json.Marshal(ids)
			return []protomcp.ResourceContent{{URI: "notes://index", Text: string(data), MimeType: "application/json"}}
		},
	})

	protomcp.RegisterResourceTemplate(protomcp.ResourceTemplateDef{
		URITemplate: "notes://{note_id}",
		Name:        "read_note",
		Description: "Read a specific meeting note by ID",
		MimeType:    "text/plain",
		HandlerFn: func(uri string) []protomcp.ResourceContent {
			noteID := strings.TrimPrefix(uri, "notes://")
			text, ok := notes[noteID]
			if !ok {
				text = fmt.Sprintf("Note not found: %s", noteID)
			}
			return []protomcp.ResourceContent{{URI: uri, Text: text}}
		},
	})

	// ─── Prompts ─────────────────────────────────────────────────────────

	protomcp.RegisterPrompt(protomcp.PromptDef{
		Name:        "summarize_note",
		Description: "Summarize a meeting note",
		Arguments: []protomcp.PromptArg{
			{Name: "note_id", Description: "ID of the note to summarize", Required: true},
			{Name: "style", Description: "Summary style: brief, detailed, or bullet"},
		},
		HandlerFn: func(args map[string]string) (string, []protomcp.PromptMessage) {
			noteID := args["note_id"]
			style := args["style"]
			if style == "" {
				style = "brief"
			}
			text, ok := notes[noteID]
			if !ok {
				text = "Note not found"
			}
			return "", []protomcp.PromptMessage{
				protomcp.UserMessage(fmt.Sprintf("Summarize this meeting note in a %s style:\n\n%s", style, text)),
			}
		},
	})

	// ─── Completions ─────────────────────────────────────────────────────

	protomcp.RegisterCompletion("ref/prompt", "summarize_note", "note_id", func(value string) protomcp.CompletionResult {
		var matches []string
		for id := range notes {
			if strings.HasPrefix(id, value) {
				matches = append(matches, id)
			}
		}
		return protomcp.CompletionResult{Values: matches, Total: int32(len(matches))}
	})

	// ─── Tools ───────────────────────────────────────────────────────────

	protomcp.Tool("search_notes",
		protomcp.Description("Search meeting notes by keyword"),
		protomcp.Args(protomcp.StrArg("query")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			query := args["query"].(string)
			matches := map[string]string{}
			for id, text := range notes {
				if strings.Contains(strings.ToLower(text), strings.ToLower(query)) {
					matches[id] = text
				}
			}
			data, _ := json.Marshal(matches)
			return protomcp.Result(string(data))
		}),
	)

	protomcp.Run()
}
