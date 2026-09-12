package bridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestStructuredToolResult(t *testing.T) {
	for _, value := range []string{`{}`, `{"rows":[[null,9007199254740993]],"truncated":false}`} {
		backend := &mockBackend{callResp: &pb.CallToolResponse{
			ResultJson:            `[{"type":"text","text":"preview"}]`,
			StructuredContentJson: value,
		}}
		result, err := makeToolHandler(backend, "query", nil)(context.Background(), &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: "query"},
		})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil || string(encoded) != value {
			t.Fatalf("lost structured content: %s, %v", encoded, err)
		}
		if len(result.Content) != 1 {
			t.Fatal("lost text fallback")
		}
	}
}

func TestRejectInvalidStructuredToolResult(t *testing.T) {
	for _, value := range []string{`null`, `[]`, `"hello"`, `{broken`} {
		backend := &mockBackend{callResp: &pb.CallToolResponse{StructuredContentJson: value}}
		_, err := makeToolHandler(backend, "query", nil)(context.Background(), &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{Name: "query"},
		})
		if err == nil {
			t.Fatalf("accepted invalid structured content: %s", value)
		}
	}
}
