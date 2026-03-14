package testengine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/msilverblatt/protomcp/internal/bridge"
	"github.com/msilverblatt/protomcp/internal/config"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/internal/toollist"
)

// CallResult wraps the result of a tool call with additional metadata.
type CallResult struct {
	Result        *mcp.CallToolResult
	Duration      time.Duration
	ToolsEnabled  []string
	ToolsDisabled []string
}

// Option configures the Engine.
type Option func(*engineConfig)

type engineConfig struct {
	runtime     string
	callTimeout time.Duration
	logger      *slog.Logger
}

// WithRuntime overrides the auto-detected runtime command.
func WithRuntime(runtime string) Option {
	return func(c *engineConfig) {
		c.runtime = runtime
	}
}

// WithCallTimeout sets the timeout for tool calls.
func WithCallTimeout(d time.Duration) Option {
	return func(c *engineConfig) {
		c.callTimeout = d
	}
}

// WithLogger sets the logger for the engine.
func WithLogger(l *slog.Logger) Option {
	return func(c *engineConfig) {
		c.logger = l
	}
}

// Engine wires together a process manager, bridge, and MCP client
// for testing tool processes.
type Engine struct {
	file string
	cfg  engineConfig

	pm      *process.Manager
	tlm     *toollist.Manager
	be      *backend
	br      *bridge.Bridge
	client  *mcp.Client
	session *mcp.ClientSession
	trace   *TraceLog

	mu              sync.Mutex
	toolsChangedFn  func([]*mcp.Tool)
	cancel          context.CancelFunc
}

// New creates a new Engine for the given tool file.
func New(file string, opts ...Option) *Engine {
	cfg := engineConfig{
		callTimeout: 30 * time.Second,
		logger:      slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	for _, o := range opts {
		o(&cfg)
	}

	return &Engine{
		file:  file,
		cfg:   cfg,
		trace: NewTraceLog(),
	}
}

// Start initializes the process manager, bridge, and MCP client connection.
func (e *Engine) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.cancel = cancel

	// Determine runtime
	var runtimeCmd string
	var runtimeArgs []string
	if e.cfg.runtime != "" {
		runtimeCmd = e.cfg.runtime
		runtimeArgs = []string{e.file}
	} else {
		runtimeCmd, runtimeArgs = config.RuntimeCommand(e.file)
	}

	// Determine socket path
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	socketPath := filepath.Join(dir, "pmcp", fmt.Sprintf("testengine-%d.sock", os.Getpid()))

	// Create tool list manager early — callbacks fire during pm.Start() handshake
	e.tlm = toollist.New()

	// Start process manager
	e.pm = process.NewManager(process.ManagerConfig{
		File:        e.file,
		RuntimeCmd:  runtimeCmd,
		RuntimeArgs: runtimeArgs,
		SocketPath:  socketPath,
		MaxRetries:  3,
		CallTimeout: e.cfg.callTimeout,
	})

	// Wire callbacks BEFORE Start() — the SDK process may send DisableToolsRequest
	// during the handshake (e.g. for hidden tools)
	e.pm.OnEnableTools(func(names []string) {
		e.tlm.Enable(names)
		if e.br != nil {
			e.br.SyncTools()
		}
	})
	e.pm.OnDisableTools(func(names []string) {
		e.tlm.Disable(names)
		if e.br != nil {
			e.br.SyncTools()
		}
	})

	tools, err := e.pm.Start(ctx)
	if err != nil {
		cancel()
		return fmt.Errorf("start process: %w", err)
	}

	// Register all tools, then apply any disables that happened during handshake
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}
	e.tlm.SetRegistered(toolNames)

	// Create backend adapter
	e.be = newBackend(e.pm, e.tlm, tools)

	// Create bridge
	e.br = bridge.New(e.be, e.cfg.logger)
	e.br.SetToolListMutationHandler(func(enable, disable []string) {
		if len(enable) > 0 {
			e.tlm.Enable(enable)
		}
		if len(disable) > 0 {
			e.tlm.Disable(disable)
		}
		e.br.SyncTools()
	})
	e.br.SyncTools()
	e.br.SyncResources()
	e.br.SyncPrompts()

	// Create in-memory transport pair
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Wrap client transport with LoggingTransport for tracing
	tracingTransport := &mcp.LoggingTransport{
		Transport: clientTransport,
		Writer:    e.trace.Writer(),
	}

	// Connect server first in goroutine
	serverErrCh := make(chan error, 1)
	go func() {
		_, err := e.br.Server.Connect(ctx, serverTransport, nil)
		serverErrCh <- err
	}()

	// Create and connect client
	e.client = mcp.NewClient(
		&mcp.Implementation{Name: "protomcp-test-engine", Version: "1.0.0"},
		&mcp.ClientOptions{
			ToolListChangedHandler: func(_ context.Context, _ *mcp.ToolListChangedRequest) {
				// Refresh tools and notify callback
				e.mu.Lock()
				fn := e.toolsChangedFn
				e.mu.Unlock()
				if fn != nil && e.session != nil {
					result, err := e.session.ListTools(ctx, nil)
					if err == nil {
						fn(result.Tools)
					}
				}
			},
		},
	)

	session, err := e.client.Connect(ctx, tracingTransport, nil)
	if err != nil {
		cancel()
		e.pm.Stop()
		return fmt.Errorf("client connect: %w", err)
	}
	e.session = session

	// Check server connection error
	select {
	case serverErr := <-serverErrCh:
		if serverErr != nil {
			cancel()
			e.pm.Stop()
			return fmt.Errorf("server connect: %w", serverErr)
		}
	default:
	}

	return nil
}

// Stop shuts down the engine.
func (e *Engine) Stop() {
	// Cancel context first to unblock any in-flight operations
	if e.cancel != nil {
		e.cancel()
	}

	// Close session and stop PM concurrently with a hard timeout
	done := make(chan struct{})
	go func() {
		if e.session != nil {
			e.session.Close()
		}
		if e.pm != nil {
			e.pm.Stop()
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		// Force kill if cleanup hangs
		if e.pm != nil {
			e.pm.Stop()
		}
	}
}

// ListTools returns the tools available from the server.
func (e *Engine) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	result, err := e.session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool calls a tool by name with the given arguments.
func (e *Engine) CallTool(ctx context.Context, name string, args map[string]any) (*CallResult, error) {
	start := time.Now()
	result, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	duration := time.Since(start)
	if err != nil {
		return nil, err
	}
	return &CallResult{
		Result:   result,
		Duration: duration,
	}, nil
}

// ListResources returns the resources available from the server.
func (e *Engine) ListResources(ctx context.Context) ([]*mcp.Resource, error) {
	result, err := e.session.ListResources(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ReadResource reads a resource by URI.
func (e *Engine) ReadResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	return e.session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
}

// ListPrompts returns the prompts available from the server.
func (e *Engine) ListPrompts(ctx context.Context) ([]*mcp.Prompt, error) {
	result, err := e.session.ListPrompts(ctx, nil)
	if err != nil {
		return nil, err
	}
	return result.Prompts, nil
}

// GetPrompt gets a prompt by name with the given arguments.
func (e *Engine) GetPrompt(ctx context.Context, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	return e.session.GetPrompt(ctx, &mcp.GetPromptParams{
		Name:      name,
		Arguments: args,
	})
}

// Trace returns the protocol trace log.
func (e *Engine) Trace() *TraceLog {
	return e.trace
}

// OnToolsChanged registers a callback for tool list changes.
func (e *Engine) OnToolsChanged(fn func([]*mcp.Tool)) {
	e.mu.Lock()
	e.toolsChangedFn = fn
	e.mu.Unlock()
}

// Reload reloads the tool process and refreshes the bridge.
func (e *Engine) Reload(ctx context.Context) error {
	newTools, err := e.pm.Reload(ctx)
	if err != nil {
		return fmt.Errorf("reload: %w", err)
	}
	e.be.UpdateTools(newTools)
	newNames := make([]string, len(newTools))
	for i, t := range newTools {
		newNames[i] = t.Name
	}
	e.tlm.SetRegistered(newNames)
	e.br.SyncTools()
	e.br.SyncResources()
	e.br.SyncPrompts()
	return nil
}

// toolResultText extracts text content from a CallToolResult as a convenience.
func toolResultText(r *mcp.CallToolResult) string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	var parts []string
	for _, c := range r.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	b, _ := json.Marshal(parts)
	return string(b)
}
