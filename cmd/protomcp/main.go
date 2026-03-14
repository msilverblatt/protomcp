package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/internal/bridge"
	"github.com/msilverblatt/protomcp/internal/cli"
	"github.com/msilverblatt/protomcp/internal/config"
	"github.com/msilverblatt/protomcp/internal/playground"
	"github.com/msilverblatt/protomcp/internal/process"
	"github.com/msilverblatt/protomcp/internal/reload"
	"github.com/msilverblatt/protomcp/internal/testengine"
	"github.com/msilverblatt/protomcp/internal/toollist"
	"github.com/msilverblatt/protomcp/internal/validate"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Setup slog
	logLevel := parseLogLevel(cfg.LogLevel)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if cfg.Command == "validate" {
		runValidate(ctx, cfg)
		return
	}

	if cfg.Command == "test" {
		runTest(ctx, cfg)
		return
	}

	if cfg.Command == "playground" {
		runPlayground(ctx, cfg)
		return
	}

	// 1. Create tool list manager
	tlm := toollist.New()

	// 2. Determine runtime
	var runtimeCmd string
	var runtimeArgs []string
	if cfg.Runtime != "" {
		runtimeCmd = cfg.Runtime
		runtimeArgs = []string{cfg.File}
	} else {
		runtimeCmd, runtimeArgs = config.RuntimeCommand(cfg.File)
	}

	// 3. Start process manager
	pm := process.NewManager(process.ManagerConfig{
		File:        cfg.File,
		RuntimeCmd:  runtimeCmd,
		RuntimeArgs: runtimeArgs,
		SocketPath:  cfg.SocketPath,
		MaxRetries:  3,
		CallTimeout: cfg.CallTimeout,
	})

	tools, err := pm.Start(ctx)
	if err != nil {
		slog.Error("failed to start tool process", "error", err)
		os.Exit(1)
	}

	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
		slog.Info("tool registered", "name", t.Name)
	}
	tlm.SetRegistered(toolNames)

	// 4. Create tool backend that combines process manager + tool list
	backend := &toolBackend{pm: pm, tlm: tlm, allTools: tools}

	// 5. Create bridge (replaces custom mcp.NewHandler)
	b := bridge.New(backend, logger)
	b.SetToolListMutationHandler(func(enable, disable []string) {
		if len(enable) > 0 {
			tlm.Enable(enable)
		}
		if len(disable) > 0 {
			tlm.Disable(disable)
		}
		b.SyncTools()
	})

	// 6. Sync tools, resources, and prompts from backend into the official mcp.Server
	b.SyncTools()
	b.SyncResources()
	b.SyncPrompts()

	// 7. Wire process manager callbacks
	pm.OnProgress(func(msg *pb.ProgressNotification) {
		// TODO: wire to official SDK session notifications (needs ServerSession)
		slog.Debug("progress notification", "token", msg.ProgressToken, "progress", msg.Progress)
	})
	pm.OnLog(func(msg *pb.LogMessage) {
		// TODO: wire to official SDK session notifications (needs ServerSession)
		slog.Debug("log notification", "level", msg.Level, "message", msg.DataJson)
	})
	pm.OnEnableTools(func(names []string) {
		tlm.Enable(names)
		b.SyncTools()
	})
	pm.OnDisableTools(func(names []string) {
		tlm.Disable(names)
		b.SyncTools()
	})

	// 8. Start file watcher (dev mode only)
	if cfg.Command == "dev" {
		w, err := reload.NewWatcher(cfg.File, nil, func() {
			slog.Info("file changed, reloading...")
			newTools, err := pm.Reload(ctx)
			if err != nil {
				slog.Error("reload failed", "error", err)
				return
			}
			backend.UpdateTools(newTools)
			newNames := make([]string, len(newTools))
			for i, t := range newTools {
				newNames[i] = t.Name
			}
			oldActive := tlm.GetActive()
			tlm.SetRegistered(newNames)
			newActive := tlm.GetActive()
			if !slicesEqual(oldActive, newActive) {
				slog.Info("tool list changed, syncing tools")
			}
			b.SyncTools()
			b.SyncResources()
			b.SyncPrompts()
		})
		if err != nil {
			slog.Error("failed to create file watcher", "error", err)
			os.Exit(1)
		}
		go w.Start(ctx)
		defer w.Stop()
	}

	// 9. Start transport (blocks)
	slog.Info("protomcp started", "command", cfg.Command, "transport", cfg.Transport, "file", cfg.File)

	switch cfg.Transport {
	case "stdio":
		if err := b.Server.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
			slog.Error("transport error", "error", err)
			os.Exit(1)
		}

	case "http", "sse":
		handler := sdkmcp.NewStreamableHTTPHandler(func(r *http.Request) *sdkmcp.Server {
			return b.Server
		}, nil)
		addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		slog.Info("listening", "addr", addr)
		srv := &http.Server{Addr: addr, Handler: handler}
		go func() {
			<-ctx.Done()
			srv.Close()
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("transport error", "error", err)
			os.Exit(1)
		}

	case "ws", "grpc":
		// TODO: WS and GRPC transports are not yet supported with the official SDK.
		fmt.Fprintf(os.Stderr, "error: transport %q is not yet supported; use stdio or http\n", cfg.Transport)
		os.Exit(1)

	default:
		// Default to stdio
		if err := b.Server.Run(ctx, &sdkmcp.StdioTransport{}); err != nil {
			slog.Error("transport error", "error", err)
			os.Exit(1)
		}
	}
}

// toolBackend implements bridge.ProcessBackend
type toolBackend struct {
	pm       *process.Manager
	tlm      *toollist.Manager
	mu       sync.RWMutex
	allTools []*pb.ToolDefinition
}

func (b *toolBackend) ActiveTools() []*pb.ToolDefinition {
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

func (b *toolBackend) UpdateTools(tools []*pb.ToolDefinition) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.allTools = tools
}

func (b *toolBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return b.pm.CallTool(ctx, name, argsJSON)
}

func (b *toolBackend) CallToolStream(ctx context.Context, name, argsJSON string) (<-chan process.StreamEvent, error) {
	return b.pm.CallToolStream(ctx, name, argsJSON)
}

func (b *toolBackend) ListResources(ctx context.Context) ([]*pb.ResourceDefinition, error) {
	return b.pm.ListResources(ctx)
}

func (b *toolBackend) ListResourceTemplates(ctx context.Context) ([]*pb.ResourceTemplateDefinition, error) {
	return b.pm.ListResourceTemplates(ctx)
}

func (b *toolBackend) ReadResource(ctx context.Context, uri string) (*pb.ReadResourceResponse, error) {
	return b.pm.ReadResource(ctx, uri)
}

func (b *toolBackend) ListPrompts(ctx context.Context) ([]*pb.PromptDefinition, error) {
	return b.pm.ListPrompts(ctx)
}

func (b *toolBackend) GetPrompt(ctx context.Context, name, argsJSON string) (*pb.GetPromptResponse, error) {
	return b.pm.GetPrompt(ctx, name, argsJSON)
}

func (b *toolBackend) Complete(ctx context.Context, refType, refName, argName, argValue string) (*pb.CompletionResponse, error) {
	return b.pm.Complete(ctx, refType, refName, argName, argValue)
}

func (b *toolBackend) SendSamplingResponse(reqID string, resp *pb.SamplingResponse) error {
	return b.pm.SendSamplingResponse(reqID, resp)
}

func (b *toolBackend) OnSampling(fn func(*pb.SamplingRequest, string)) {
	b.pm.OnSampling(fn)
}

func (b *toolBackend) SendListRootsResponse(reqID string, resp *pb.ListRootsResponse) error {
	return b.pm.SendListRootsResponse(reqID, resp)
}

func (b *toolBackend) OnListRoots(fn func(string)) {
	b.pm.OnListRoots(fn)
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func runValidate(ctx context.Context, cfg *config.Config) {
	var runtimeCmd string
	var runtimeArgs []string
	if cfg.Runtime != "" {
		runtimeCmd = cfg.Runtime
		runtimeArgs = []string{cfg.File}
	} else {
		runtimeCmd, runtimeArgs = config.RuntimeCommand(cfg.File)
	}

	pm := process.NewManager(process.ManagerConfig{
		File:        cfg.File,
		RuntimeCmd:  runtimeCmd,
		RuntimeArgs: runtimeArgs,
		SocketPath:  cfg.SocketPath,
		MaxRetries:  1,
		CallTimeout: 30 * time.Second,
	})

	tools, err := pm.Start(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to start tool process: %v\n", err)
		os.Exit(1)
	}
	defer pm.Stop()

	result := validate.Tools(tools, cfg.Strict)

	if cfg.Format == "json" {
		fmt.Println(result.FormatJSON())
	} else {
		fmt.Print(result.FormatText())
	}

	if !result.Pass {
		os.Exit(1)
	}
}

func runTest(ctx context.Context, cfg *config.Config) {
	var err error
	switch cfg.TestSubcommand {
	case "list":
		err = cli.RunTestList(ctx, cfg.File, cfg.Format)
	case "call":
		err = cli.RunTestCall(ctx, cfg.File, cfg.TestToolName, cfg.TestArgs, cfg.Format, cfg.ShowTrace)
	case "scenario":
		fmt.Fprintf(os.Stderr, "test scenario: coming soon\n")
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runPlayground(ctx context.Context, cfg *config.Config) {
	eng := testengine.New(cfg.File, testengine.WithLogger(slog.Default()))
	if err := eng.Start(ctx); err != nil {
		slog.Error("failed to start engine", "error", err)
		os.Exit(1)
	}
	defer eng.Stop()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := playground.NewServer(eng, slog.Default())
	if err := srv.ListenAndServe(ctx, addr); err != nil && err != http.ErrServerClosed {
		slog.Error("playground error", "error", err)
		os.Exit(1)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
