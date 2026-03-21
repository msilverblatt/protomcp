package bridge

import (
	"context"
	"encoding/base64"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// syncTools registers tools from the backend and removes any stale tools
// that are no longer present. The registered map tracks currently known tools.
func syncTools(server *mcp.Server, backend ProcessBackend, onMutation ToolListMutationHandler, registered map[string]bool) {
	tools := backend.ActiveTools()

	current := make(map[string]bool, len(tools))
	for _, t := range tools {
		current[t.Name] = true
		tool := convertToolDef(t)
		handler := makeToolHandler(backend, t.Name, onMutation)
		server.AddTool(tool, handler)
	}

	// Remove tools that were previously registered but are no longer present.
	var stale []string
	for name := range registered {
		if !current[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		server.RemoveTools(stale...)
	}

	// Update the registered set to match current.
	for name := range registered {
		delete(registered, name)
	}
	for name := range current {
		registered[name] = true
	}
}

func convertToolDef(t *pb.ToolDefinition) *mcp.Tool {
	tool := &mcp.Tool{
		Name:        t.Name,
		Description: t.Description,
	}

	// Parse input schema (field is `InputSchema any`, accepts json.RawMessage)
	if t.InputSchemaJson != "" {
		tool.InputSchema = json.RawMessage(t.InputSchemaJson)
	} else {
		// SDK panics if InputSchema is nil or not type "object"
		tool.InputSchema = json.RawMessage(`{"type":"object"}`)
	}

	// Parse output schema
	if t.OutputSchemaJson != "" {
		tool.OutputSchema = json.RawMessage(t.OutputSchemaJson)
	}

	// Set title (top-level field on Tool, not on Annotations)
	if t.Title != "" {
		tool.Title = t.Title
	}

	// Set annotations
	if t.ReadOnlyHint || t.DestructiveHint || t.IdempotentHint || t.OpenWorldHint {
		tool.Annotations = &mcp.ToolAnnotations{}
		if t.ReadOnlyHint {
			tool.Annotations.ReadOnlyHint = true
		}
		if t.DestructiveHint {
			v := true
			tool.Annotations.DestructiveHint = &v
		}
		if t.IdempotentHint {
			tool.Annotations.IdempotentHint = true
		}
		if t.OpenWorldHint {
			v := true
			tool.Annotations.OpenWorldHint = &v
		}
	}

	return tool
}

func makeToolHandler(backend ProcessBackend, name string, onMutation ToolListMutationHandler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// req.Params.Arguments is json.RawMessage — use directly, don't re-marshal
		argsJSON := "{}"
		if len(req.Params.Arguments) > 0 {
			argsJSON = string(req.Params.Arguments)
		}

		resp, err := backend.CallTool(ctx, name, argsJSON)
		if err != nil {
			return nil, err
		}

		// Process enable_tools/disable_tools from the response
		if onMutation != nil && (len(resp.EnableTools) > 0 || len(resp.DisableTools) > 0) {
			onMutation(resp.EnableTools, resp.DisableTools)
		}

		result := &mcp.CallToolResult{
			IsError: resp.IsError,
		}

		// Parse result_json into content items
		if resp.ResultJson != "" {
			var items []json.RawMessage
			if err := json.Unmarshal([]byte(resp.ResultJson), &items); err == nil {
				for _, item := range items {
					var typeCheck struct {
						Type string `json:"type"`
					}
					if json.Unmarshal(item, &typeCheck) == nil {
						switch typeCheck.Type {
						case "text":
							var tc struct {
								Text string `json:"text"`
							}
							if json.Unmarshal(item, &tc) == nil {
								result.Content = append(result.Content, &mcp.TextContent{Text: tc.Text})
							}
						case "image":
							var ic struct {
								Data     string `json:"data"`
								MIMEType string `json:"mimeType"`
							}
							if json.Unmarshal(item, &ic) == nil {
								decoded, decErr := base64.StdEncoding.DecodeString(ic.Data)
								if decErr != nil {
									// data may already be raw bytes encoded differently; store as-is
									decoded = []byte(ic.Data)
								}
								result.Content = append(result.Content, &mcp.ImageContent{Data: decoded, MIMEType: ic.MIMEType})
							}
						case "audio":
							var ac struct {
								Data     string `json:"data"`
								MIMEType string `json:"mimeType"`
							}
							if json.Unmarshal(item, &ac) == nil {
								decoded, decErr := base64.StdEncoding.DecodeString(ac.Data)
								if decErr != nil {
									decoded = []byte(ac.Data)
								}
								result.Content = append(result.Content, &mcp.AudioContent{Data: decoded, MIMEType: ac.MIMEType})
							}
						default:
							// Unknown content type, wrap as text
							result.Content = append(result.Content, &mcp.TextContent{Text: string(item)})
						}
					}
				}
			} else {
				// Not an array, treat the whole thing as text
				result.Content = append(result.Content, &mcp.TextContent{Text: resp.ResultJson})
			}
		}

		return result, nil
	}
}
