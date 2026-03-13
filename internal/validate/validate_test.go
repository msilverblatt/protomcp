package validate_test

import (
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/validate"
)

func TestValidateValidTools(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "Add two numbers", InputSchemaJson: `{"type":"object","properties":{"a":{"type":"integer"}}}`},
		{Name: "multiply", Description: "Multiply two numbers", InputSchemaJson: `{"type":"object","properties":{"b":{"type":"integer"}}}`},
	}
	result := validate.Tools(tools, false)
	if !result.Pass {
		t.Errorf("expected pass, got errors: %v", result.Errors)
	}
	if len(result.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result.Tools))
	}
}

func TestValidateEmptyName(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "", Description: "Something", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	if result.Pass {
		t.Error("expected fail for empty name")
	}
}

func TestValidateInvalidNameChars(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "my-tool", Description: "Has hyphens", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	if result.Pass {
		t.Error("expected fail for invalid name characters")
	}
}

func TestValidateDuplicateNames(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "First", InputSchemaJson: `{"type":"object"}`},
		{Name: "add", Description: "Second", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	if result.Pass {
		t.Error("expected fail for duplicate names")
	}
}

func TestValidateEmptyDescription(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	if result.Pass {
		t.Error("expected fail for empty description")
	}
}

func TestValidateInvalidSchema(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "Add", InputSchemaJson: `not json`},
	}
	result := validate.Tools(tools, false)
	if result.Pass {
		t.Error("expected fail for invalid JSON schema")
	}
}

func TestValidateStrictShortDescription(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "Add", InputSchemaJson: `{"type":"object","properties":{"a":{"type":"integer"}}}`},
	}
	result := validate.Tools(tools, true)
	if result.Pass {
		t.Error("expected fail in strict mode for short description")
	}
}

func TestValidateStrictGenericName(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "test", Description: "A test tool for testing", InputSchemaJson: `{"type":"object","properties":{"a":{"type":"integer"}}}`},
	}
	result := validate.Tools(tools, true)
	if result.Pass {
		t.Error("expected fail in strict mode for generic name")
	}
}

func TestResultFormatText(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "Add two numbers", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	output := result.FormatText()
	if output == "" {
		t.Error("expected non-empty text output")
	}
}

func TestResultFormatJSON(t *testing.T) {
	tools := []*pb.ToolDefinition{
		{Name: "add", Description: "Add two numbers", InputSchemaJson: `{"type":"object"}`},
	}
	result := validate.Tools(tools, false)
	output := result.FormatJSON()
	if output == "" {
		t.Error("expected non-empty JSON output")
	}
}
