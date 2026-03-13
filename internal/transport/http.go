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

// HTTPTransport implements the Transport interface using plain HTTP.
// POST requests to / return JSON-RPC responses directly.
// Notifications are streamed to clients connected to /events as SSE.
type HTTPTransport struct {
	host string
	port int

	mu      sync.RWMutex
	clients map[chan []byte]struct{}

	server   *http.Server
	listener net.Listener
}

// NewHTTPTransport creates a new HTTPTransport.
func NewHTTPTransport(host string, port int) *HTTPTransport {
	return &HTTPTransport{
		host:    host,
		port:    port,
		clients: make(map[chan []byte]struct{}),
	}
}

func (h *HTTPTransport) addClient(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = struct{}{}
}

func (h *HTTPTransport) removeClient(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
}

func (h *HTTPTransport) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- data:
		default:
		}
	}
}

// Start starts the HTTP server and blocks until ctx is cancelled.
func (h *HTTPTransport) Start(ctx context.Context, handler RequestHandler) error {
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

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
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
		h.addClient(ch)
		defer h.removeClient(ch)

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
	addr := fmt.Sprintf("%s:%d", h.host, h.port)
	h.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("http listen %s: %w", addr, err)
	}

	h.server = &http.Server{Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		if err := h.server.Serve(h.listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	select {
	case <-ctx.Done():
		_ = h.server.Close()
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

// SendNotification broadcasts a JSON-RPC notification to /events SSE clients.
func (h *HTTPTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	h.broadcast(data)
	return nil
}

// Close shuts down the HTTP server.
func (h *HTTPTransport) Close() error {
	if h.server != nil {
		return h.server.Close()
	}
	return nil
}
