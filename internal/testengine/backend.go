package testengine

import (
	"context"
	"sync"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/internal/toollist"
)

// backend implements bridge.FullBackend by combining process.Manager
// with toollist.Manager, following the same pattern as toolBackend in
// cmd/protomcp/main.go.
type backend struct {
	pm       *process.Manager
	tlm      *toollist.Manager
	mu       sync.RWMutex
	allTools []*pb.ToolDefinition
}

func newBackend(pm *process.Manager, tlm *toollist.Manager, tools []*pb.ToolDefinition) *backend {
	return &backend{
		pm:       pm,
		tlm:      tlm,
		allTools: tools,
	}
}

func (b *backend) ActiveTools() []*pb.ToolDefinition {
	b.mu.RLock()
	defer b.mu.RUnlock()
	activeNames := b.tlm.GetActive()
	nameSet := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		nameSet[n] = true
	}
	var result []*pb.ToolDefinition
	for _, t := range b.allTools {
		if nameSet[t.Name] {
			result = append(result, t)
		}
	}
	return result
}

func (b *backend) UpdateTools(tools []*pb.ToolDefinition) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allTools = tools
}

func (b *backend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return b.pm.CallTool(ctx, name, argsJSON)
}

func (b *backend) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error) {
	return b.pm.ListResources(ctx)
}

func (b *backend) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error) {
	return b.pm.ListResourceTemplates(ctx)
}

func (b *backend) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error) {
	return b.pm.ReadResource(ctx, uri)
}

func (b *backend) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error) {
	return b.pm.ListPrompts(ctx)
}

func (b *backend) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error) {
	return b.pm.GetPrompt(ctx, name, argsJSON)
}

func (b *backend) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error) {
	return b.pm.Complete(ctx, refType, refName, argName, argValue)
}

func (b *backend) SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error {
	return b.pm.SendSamplingResponse(reqID, resp)
}

func (b *backend) OnSampling(fn func(*pb.SamplingRequest, string)) {
	b.pm.OnSampling(fn)
}

func (b *backend) SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error {
	return b.pm.SendListRootsResponse(reqID, resp)
}

func (b *backend) OnListRoots(fn func(string)) {
	b.pm.OnListRoots(fn)
}
