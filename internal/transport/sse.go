package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// SSETransport implements the Transport interface using Server-Sent Events.
// POST requests to / contain JSON-RPC requests; responses and notifications
// are streamed to connected SSE clients at /sse.
type SSETransport struct {
	host string
	port int

	mu      sync.RWMutex
	clients map[chan []byte]struct{}

	server   *http.Server
	listener net.Listener
}

// NewSSETransport creates a new SSETransport.
func NewSSETransport(host string, port int) *SSETransport {
	return &SSETransport{
		host:    host,
		port:    port,
		clients: make(map[chan []byte]struct{}),
	}
}

func (s *SSETransport) addClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[ch] = struct{}{}
}

func (s *SSETransport) removeClient(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.clients, ch)
}

func (s *SSETransport) broadcast(data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

// Start starts the HTTP server and blocks until ctx is cancelled.
func (s *SSETransport) Start(ctx context.Context, handler RequestHandler) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req mcp.JSONRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		resp, err := handler(r.Context(), req)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if resp == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		data, err := json.Marshal(resp)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		// Broadcast response to SSE clients.
		s.broadcast(data)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ch := make(chan []byte, 32)
		s.addClient(ch)
		defer s.removeClient(ch)

		for {
			select {
			case <-r.Context().Done():
				return
			case data := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	})

	var err error
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("sse listen %s: %w", addr, err)
	}

	s.server = &http.Server{Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		_ = s.server.Close()
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// SendNotification broadcasts a JSON-RPC notification to all SSE clients.
func (s *SSETransport) SendNotification(notification mcp.JSONRPCNotification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	s.broadcast(data)
	return nil
}

// Close shuts down the HTTP server.
func (s *SSETransport) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}
