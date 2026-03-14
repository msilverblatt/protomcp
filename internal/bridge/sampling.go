package bridge

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// SamplingBackend lets the bridge send sampling responses back to the SDK process.
type SamplingBackend interface {
	SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error
	OnSampling(fn func(*pb.SamplingRequest, string))
	SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error
	OnListRoots(fn func(string))
}

func (b *Bridge) handleSampling(req *pb.SamplingRequest, reqID string) {
	// Get a session to send the sampling request to the MCP client.
	var session *mcp.ServerSession
	for ss := range b.Server.Sessions() {
		session = ss
		break
	}
	if session == nil {
		b.backend.SendSamplingResponse(reqID, &pb.SamplingResponse{
			Error: "no active MCP session",
		})
		return
	}

	// Parse messages from JSON.
	var messages []*mcp.SamplingMessage
	if req.MessagesJson != "" {
		if err := json.Unmarshal([]byte(req.MessagesJson), &messages); err != nil {
			b.backend.SendSamplingResponse(reqID, &pb.SamplingResponse{
				Error: "failed to parse messages_json: " + err.Error(),
			})
			return
		}
	}

	params := &mcp.CreateMessageParams{
		Messages:     messages,
		MaxTokens:    int64(req.MaxTokens),
		SystemPrompt: req.SystemPrompt,
	}
	if req.ModelPreferencesJson != "" {
		var prefs mcp.ModelPreferences
		if err := json.Unmarshal([]byte(req.ModelPreferencesJson), &prefs); err != nil {
			b.backend.SendSamplingResponse(reqID, &pb.SamplingResponse{
				Error: "failed to parse model_preferences_json: " + err.Error(),
			})
			return
		}
		params.ModelPreferences = &prefs
	}

	result, err := session.CreateMessage(context.Background(), params)
	if err != nil {
		b.backend.SendSamplingResponse(reqID, &pb.SamplingResponse{
			Error: err.Error(),
		})
		return
	}

	// Serialize content back to JSON.
	contentJSON, _ := json.Marshal(result.Content)
	b.backend.SendSamplingResponse(reqID, &pb.SamplingResponse{
		Role:        string(result.Role),
		ContentJson: string(contentJSON),
		Model:       result.Model,
		StopReason:  result.StopReason,
	})
}

func (b *Bridge) handleListRoots(reqID string) {
	var session *mcp.ServerSession
	for ss := range b.Server.Sessions() {
		session = ss
		break
	}
	if session == nil {
		b.backend.SendListRootsResponse(reqID, &pb.ListRootsResponse{})
		return
	}

	result, err := session.ListRoots(context.Background(), nil)
	if err != nil {
		b.backend.SendListRootsResponse(reqID, &pb.ListRootsResponse{})
		return
	}

	var roots []*pb.RootDef
	for _, r := range result.Roots {
		roots = append(roots, &pb.RootDef{
			Uri:  r.URI,
			Name: r.Name,
		})
	}
	b.backend.SendListRootsResponse(reqID, &pb.ListRootsResponse{Roots: roots})
}
