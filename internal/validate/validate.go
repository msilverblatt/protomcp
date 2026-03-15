package validate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

var validName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

var genericNames = map[string]bool{
	"test": true, "tool1": true, "foo": true, "bar": true, "baz": true,
	"temp": true, "tmp": true, "example": true,
}

type ToolStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type ValidationError struct {
	Tool  string `json:"tool"`
	Issue string `json:"issue"`
}

type Result struct {
	Tools  []ToolStatus      `json:"tools"`
	Errors []ValidationError `json:"errors"`
	Pass   bool              `json:"pass"`
}

func Tools(tools []*pb.ToolDefinition, strict bool) Result {
	result := Result{Pass: true}
	seen := make(map[string]bool)

	for _, t := range tools {
		status := ToolStatus{Name: t.Name, Status: "ok"}

		if t.Name == "" {
			result.Errors = append(result.Errors, ValidationError{Tool: "", Issue: "empty tool name"})
			result.Pass = false
			status.Status = "error"
		} else if !validName.MatchString(t.Name) {
			result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: fmt.Sprintf("invalid name %q: must match [a-zA-Z_][a-zA-Z0-9_]*", t.Name)})
			result.Pass = false
			status.Status = "error"
		} else if seen[t.Name] {
			result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: "duplicate tool name"})
			result.Pass = false
			status.Status = "error"
		}
		seen[t.Name] = true

		if t.Description == "" {
			result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: "no description"})
			result.Pass = false
			status.Status = "error"
		}

		if t.InputSchemaJson != "" {
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(t.InputSchemaJson), &schema); err != nil {
				result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: fmt.Sprintf("invalid input schema JSON: %v", err)})
				result.Pass = false
				status.Status = "error"
			}
		}

		if strict {
			if len(t.Description) < 10 {
				result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: fmt.Sprintf("description too short (%d chars, minimum 10)", len(t.Description))})
				result.Pass = false
				status.Status = "error"
			}
			if genericNames[strings.ToLower(t.Name)] {
				result.Errors = append(result.Errors, ValidationError{Tool: t.Name, Issue: fmt.Sprintf("generic name %q", t.Name)})
				result.Pass = false
				status.Status = "error"
			}
		}

		result.Tools = append(result.Tools, status)
	}

	return result
}

func (r Result) FormatText() string {
	var sb strings.Builder
	for _, t := range r.Tools {
		if t.Status == "ok" {
			sb.WriteString(fmt.Sprintf("✓ %s — OK\n", t.Name))
		}
	}
	if len(r.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("✗ — %d error(s):\n", len(r.Errors)))
		for _, e := range r.Errors {
			if e.Tool != "" {
				sb.WriteString(fmt.Sprintf("  · %q: %s\n", e.Tool, e.Issue))
			} else {
				sb.WriteString(fmt.Sprintf("  · %s\n", e.Issue))
			}
		}
	}
	return sb.String()
}

func (r Result) FormatJSON() (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("failed to marshal validation result: %w", err)
	}
	return string(b), nil
}
