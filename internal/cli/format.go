package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// FormatToolTable renders a table of tools with name, description, and parameters.
func FormatToolTable(tools []*mcp.Tool) string {
	if len(tools) == 0 {
		return "  (no tools)\n"
	}

	var sb strings.Builder
	for i, t := range tools {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %s", t.Name))
		if t.Description != "" {
			sb.WriteString(fmt.Sprintf(" — %s", t.Description))
		}
		sb.WriteString("\n")

		params := parseInputSchema(t.InputSchema)
		if len(params) > 0 {
			for _, p := range params {
				sb.WriteString(fmt.Sprintf("    %s\n", p))
			}
		}
	}
	return sb.String()
}

// FormatResourceTable renders a table of resources with URI, description, and MIME type.
func FormatResourceTable(resources []*mcp.Resource) string {
	if len(resources) == 0 {
		return "  (no resources)\n"
	}

	var sb strings.Builder
	for i, r := range resources {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %s", r.URI))
		if r.Name != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", r.Name))
		}
		if r.Description != "" {
			sb.WriteString(fmt.Sprintf(" — %s", r.Description))
		}
		if r.MIMEType != "" {
			sb.WriteString(fmt.Sprintf("  [%s]", r.MIMEType))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// FormatPromptTable renders a table of prompts with name, description, and arguments.
func FormatPromptTable(prompts []*mcp.Prompt) string {
	if len(prompts) == 0 {
		return "  (no prompts)\n"
	}

	var sb strings.Builder
	for i, p := range prompts {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("  %s", p.Name))
		if p.Description != "" {
			sb.WriteString(fmt.Sprintf(" — %s", p.Description))
		}
		sb.WriteString("\n")

		for _, arg := range p.Arguments {
			reqStr := "optional"
			if arg.Required {
				reqStr = "required"
			}
			line := fmt.Sprintf("    %s (%s)", arg.Name, reqStr)
			if arg.Description != "" {
				line += fmt.Sprintf(" — %s", arg.Description)
			}
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// paramLine represents a parsed parameter from a JSON schema.
type paramLine struct {
	name     string
	typ      string
	required bool
}

func (p paramLine) String() string {
	reqStr := "optional"
	if p.required {
		reqStr = "required"
	}
	return fmt.Sprintf("%s (%s, %s)", p.name, p.typ, reqStr)
}

// parseInputSchema extracts parameter info from a tool's InputSchema.
func parseInputSchema(schema any) []paramLine {
	if schema == nil {
		return nil
	}

	// Marshal and unmarshal to get a map
	data, err := json.Marshal(schema)
	if err != nil {
		return nil
	}

	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}

	if len(s.Properties) == 0 {
		return nil
	}

	requiredSet := make(map[string]bool, len(s.Required))
	for _, r := range s.Required {
		requiredSet[r] = true
	}

	var params []paramLine
	for name, raw := range s.Properties {
		var prop struct {
			Type string `json:"type"`
		}
		typ := "any"
		if json.Unmarshal(raw, &prop) == nil && prop.Type != "" {
			typ = prop.Type
		}
		params = append(params, paramLine{
			name:     name,
			typ:      typ,
			required: requiredSet[name],
		})
	}

	sort.Slice(params, func(i, j int) bool {
		// Required params first, then alphabetical
		if params[i].required != params[j].required {
			return params[i].required
		}
		return params[i].name < params[j].name
	})

	return params
}
