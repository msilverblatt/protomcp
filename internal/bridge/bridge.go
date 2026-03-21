package bridge

import (
	"context"
	"log/slog"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// ProcessBackend is the interface for communicating with an SDK tool process.
// Implemented by process.Manager.
type ProcessBackend interface {
	ActiveTools() []*pb.ToolDefinition
	CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error)
}

// CompletionBackend is the interface for completion operations.
type CompletionBackend interface {
	Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error)
}

// FullBackend combines all backend interfaces.
type FullBackend interface {
	ProcessBackend
	ResourceBackend
	PromptBackend
	CompletionBackend
	SamplingBackend
}

// ToolListMutationHandler is called when a tool call response contains enable_tools or disable_tools.
type ToolListMutationHandler func(enable, disable []string)

// Bridge connects an mcp.Server to a FullBackend.
// It registers proxy handlers that forward MCP requests to the SDK process.
type Bridge struct {
	Server             *mcp.Server
	backend            FullBackend
	logger             *slog.Logger
	onToolListMutation ToolListMutationHandler

	mu                  sync.Mutex
	registeredTools     map[string]bool
	registeredResources map[string]bool
	registeredTemplates map[string]bool
	registeredPrompts   map[string]bool
}

// New creates a Bridge with an mcp.Server that proxies to the given backend.
func New(backend FullBackend, logger *slog.Logger, version string) *Bridge {
	opts := &mcp.ServerOptions{
		Logger: logger,
		CompletionHandler: func(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			var refName string
			switch req.Params.Ref.Type {
			case "ref/resource":
				refName = req.Params.Ref.URI
			default:
				refName = req.Params.Ref.Name
			}
			resp, err := backend.Complete(ctx, req.Params.Ref.Type, refName, req.Params.Argument.Name, req.Params.Argument.Value)
			if err != nil {
				return nil, err
			}
			return &mcp.CompleteResult{
				Completion: mcp.CompletionResultDetails{
					Values:  resp.Values,
					Total:   int(resp.Total),
					HasMore: resp.HasMore,
				},
			}, nil
		},
	}

	opts.RootsListChangedHandler = func(ctx context.Context, req *mcp.RootsListChangedRequest) {
		if logger != nil {
			logger.Info("roots list changed")
		}
	}

	server := mcp.NewServer(
		&mcp.Implementation{Name: "protomcp", Version: version},
		opts,
	)

	b := &Bridge{
		Server:              server,
		backend:             backend,
		logger:              logger,
		registeredTools:     make(map[string]bool),
		registeredResources: make(map[string]bool),
		registeredTemplates: make(map[string]bool),
		registeredPrompts:   make(map[string]bool),
	}

	// Wire reverse-request callbacks from the SDK process.
	backend.OnSampling(func(req *pb.SamplingRequest, reqID string) {
		go b.handleSampling(req, reqID)
	})
	backend.OnListRoots(func(reqID string) {
		go b.handleListRoots(reqID)
	})

	return b
}

// SetToolListMutationHandler registers a callback for enable_tools/disable_tools in tool call responses.
func (b *Bridge) SetToolListMutationHandler(fn ToolListMutationHandler) {
	b.onToolListMutation = fn
}

// SyncTools reads tool definitions from the backend and registers them
// with the mcp.Server. Called on startup and after hot reload.
func (b *Bridge) SyncTools() {
	b.mu.Lock()
	defer b.mu.Unlock()
	syncTools(b.Server, b.backend, b.onToolListMutation, b.registeredTools)
}

// SyncResources reads resource and resource template definitions from the
// backend and registers them with the mcp.Server.
func (b *Bridge) SyncResources() {
	b.mu.Lock()
	defer b.mu.Unlock()
	syncResources(b.Server, b.backend, b.registeredResources, b.registeredTemplates)
}

// SyncPrompts reads prompt definitions from the backend and registers them
// with the mcp.Server.
func (b *Bridge) SyncPrompts() {
	b.mu.Lock()
	defer b.mu.Unlock()
	syncPrompts(b.Server, b.backend, b.registeredPrompts)
}
