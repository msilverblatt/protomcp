package playground

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/msilverblatt/protomcp/internal/testengine"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

// Server serves the playground frontend and REST/WebSocket API.
type Server struct {
	engine *testengine.Engine
	hub    *Hub
	logger *slog.Logger
}

// NewServer creates a new playground Server.
func NewServer(engine *testengine.Engine, logger *slog.Logger) *Server {
	return &Server{
		engine: engine,
		hub:    NewHub(),
		logger: logger,
	}
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	mux := http.NewServeMux()

	// Embedded frontend
	distFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		return err
	}
	mux.Handle("GET /", http.FileServer(http.FS(distFS)))

	// REST API
	mux.HandleFunc("GET /api/tools", s.handleListTools)
	mux.HandleFunc("GET /api/resources", s.handleListResources)
	mux.HandleFunc("GET /api/prompts", s.handleListPrompts)
	mux.HandleFunc("POST /api/call", s.handleCallTool)
	mux.HandleFunc("POST /api/resource/read", s.handleReadResource)
	mux.HandleFunc("POST /api/prompt/get", s.handleGetPrompt)
	mux.HandleFunc("POST /api/reload", s.handleReload)
	mux.HandleFunc("GET /api/trace", s.handleGetTrace)

	// WebSocket
	mux.HandleFunc("GET /ws", s.handleWebSocket)

	// Wire trace events to hub
	traceCh := s.engine.Trace().Subscribe()
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.engine.Trace().Unsubscribe(traceCh)
				return
			case entry := <-traceCh:
				s.hub.Broadcast(Event{
					Type: "trace",
					Data: entry,
				})
			}
		}
	}()

	// Start hub
	go s.hub.Run(ctx)

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	s.logger.Info("playground listening", "addr", addr)
	return srv.ListenAndServe()
}
