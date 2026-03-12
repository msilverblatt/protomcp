package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	pb "github.com/protomcp/protomcp/gen/proto/protomcp"
	"github.com/protomcp/protomcp/internal/config"
	"github.com/protomcp/protomcp/internal/mcp"
	"github.com/protomcp/protomcp/internal/middleware"
	"github.com/protomcp/protomcp/internal/process"
	"github.com/protomcp/protomcp/internal/reload"
	"github.com/protomcp/protomcp/internal/toollist"
	"github.com/protomcp/protomcp/internal/transport"
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

	// 5. Create MCP handler
	handler := mcp.NewHandler(backend)
	handler.SetToolListMutationHandler(func(enable, disable []string) {
		if len(enable) > 0 {
			tlm.Enable(enable)
		}
		if len(disable) > 0 {
			tlm.Disable(disable)
		}
	})

	// 6. Apply middleware
	chain := middleware.Chain(
		func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
			return handler.Handle(ctx, req)
		},
		middleware.Logging(logger),
		middleware.ErrorFormatting(),
	)

	// 7. Create transport
	tp := createTransport(cfg)

	// 8. Start file watcher (dev mode only)
	if cfg.Command == "dev" {
		w, err := reload.NewWatcher(cfg.File, nil, func() {
			slog.Info("file changed, reloading...")
			newTools, err := pm.Reload(ctx)
			if err != nil {
				slog.Error("reload failed", "error", err)
				return
			}
			newNames := make([]string, len(newTools))
			for i, t := range newTools {
				newNames[i] = t.Name
			}
			oldActive := tlm.GetActive()
			tlm.SetRegistered(newNames)
			newActive := tlm.GetActive()
			if !slicesEqual(oldActive, newActive) {
				slog.Info("tool list changed, notifying client")
				tp.SendNotification(mcp.ListChangedNotification())
			}
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
	if err := tp.Start(ctx, func(ctx context.Context, req mcp.JSONRPCRequest) (*mcp.JSONRPCResponse, error) {
		return chain(ctx, req)
	}); err != nil {
		slog.Error("transport error", "error", err)
		os.Exit(1)
	}
}

// toolBackend implements mcp.ToolBackend
type toolBackend struct {
	pm       *process.Manager
	tlm      *toollist.Manager
	allTools []*pb.ToolDefinition
}

func (b *toolBackend) ActiveTools() []*pb.ToolDefinition {
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

func (b *toolBackend) CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error) {
	return b.pm.CallTool(ctx, name, argsJSON)
}

func createTransport(cfg *config.Config) transport.Transport {
	switch cfg.Transport {
	case "stdio":
		return transport.NewStdio()
	case "http":
		return transport.NewHTTPTransport(cfg.Host, cfg.Port)
	case "sse":
		return transport.NewSSETransport(cfg.Host, cfg.Port)
	case "ws":
		return transport.NewWSTransport(cfg.Host, cfg.Port)
	case "grpc":
		return transport.NewGRPCTransport(cfg.Host, cfg.Port)
	default:
		return transport.NewStdio()
	}
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
