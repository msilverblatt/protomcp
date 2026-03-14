package playground

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// Event is a JSON message sent over WebSocket connections.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub manages WebSocket client connections and broadcasts events.
type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]context.CancelFunc
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]context.CancelFunc),
	}
}

// Run waits for context cancellation and closes all connections.
func (h *Hub) Run(ctx context.Context) {
	<-ctx.Done()
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn, cancel := range h.clients {
		cancel()
		conn.Close(websocket.StatusGoingAway, "server shutting down")
	}
	h.clients = make(map[*websocket.Conn]context.CancelFunc)
}

// Add registers a client connection.
func (h *Hub) Add(conn *websocket.Conn, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = cancel
}

// Remove unregisters a client connection.
func (h *Hub) Remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.clients[conn]; ok {
		cancel()
		delete(h.clients, conn)
	}
}

// Broadcast sends an event to all connected clients.
func (h *Hub) Broadcast(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.Lock()
	clients := make(map[*websocket.Conn]context.CancelFunc, len(h.clients))
	for conn, cancel := range h.clients {
		clients[conn] = cancel
	}
	h.mu.Unlock()

	for conn := range clients {
		err := conn.Write(context.Background(), websocket.MessageText, data)
		if err != nil {
			h.Remove(conn)
		}
	}
}

// handleWebSocket upgrades an HTTP connection to WebSocket and registers it.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Error("websocket accept failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	s.hub.Add(conn, cancel)

	// Send connection event
	data, _ := json.Marshal(Event{Type: "connected"})
	conn.Write(ctx, websocket.MessageText, data)

	// Read loop: wait for client to close
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			break
		}
	}

	s.hub.Remove(conn)
	conn.Close(websocket.StatusNormalClosure, "")
}
