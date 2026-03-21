package bridge

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// PromptBackend is the interface for prompt operations.
type PromptBackend interface {
	ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error)
	GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error)
}

func syncPrompts(server *mcp.Server, backend PromptBackend, registered map[string]bool) {
	ctx := context.Background()
	prompts, err := backend.ListPrompts(ctx)
	if err != nil {
		return
	}
	current := make(map[string]bool, len(prompts))
	for _, p := range prompts {
		current[p.Name] = true
		prompt := &mcp.Prompt{
			Name:        p.Name,
			Description: p.Description,
		}
		for _, a := range p.Arguments {
			prompt.Arguments = append(prompt.Arguments, &mcp.PromptArgument{
				Name:        a.Name,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		handler := makePromptHandler(backend, p.Name)
		server.AddPrompt(prompt, handler)
	}

	// Remove prompts that were previously registered but are no longer present.
	var stale []string
	for name := range registered {
		if !current[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		server.RemovePrompts(stale...)
	}

	// Update the registered set to match current.
	for name := range registered {
		delete(registered, name)
	}
	for name := range current {
		registered[name] = true
	}
}

func makePromptHandler(backend PromptBackend, name string) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		argsJSON := "{}"
		if req.Params.Arguments != nil {
			b, err := json.Marshal(req.Params.Arguments)
			if err != nil {
				return nil, err
			}
			argsJSON = string(b)
		}

		resp, err := backend.GetPrompt(ctx, name, argsJSON)
		if err != nil {
			return nil, err
		}

		result := &mcp.GetPromptResult{
			Description: resp.Description,
		}

		for _, m := range resp.Messages {
			pm := &mcp.PromptMessage{
				Role: mcp.Role(m.Role),
			}
			if m.ContentJson != "" {
				var typeCheck struct {
					Type string `json:"type"`
				}
				if json.Unmarshal([]byte(m.ContentJson), &typeCheck) == nil {
					switch typeCheck.Type {
					case "text":
						var tc struct {
							Text string `json:"text"`
						}
						if json.Unmarshal([]byte(m.ContentJson), &tc) == nil {
							pm.Content = &mcp.TextContent{Text: tc.Text}
						}
					default:
						pm.Content = &mcp.TextContent{Text: m.ContentJson}
					}
				} else {
					pm.Content = &mcp.TextContent{Text: m.ContentJson}
				}
			}
			result.Messages = append(result.Messages, pm)
		}

		return result, nil
	}
}
