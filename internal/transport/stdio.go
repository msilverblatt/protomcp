package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"

	"github.com/msilverblatt/protomcp/internal/mcp"
)

// StdioTransport reads newline-delimited JSON-RPC messages from a reader
// and writes JSON-RPC responses to a writer.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer
	mu     sync.Mutex
}

// NewStdio creates a StdioTransport using os.Stdin and os.Stdout.
func NewStdio() *StdioTransport {
	return &StdioTransport{reader: os.Stdin, writer: os.Stdout}
}

// NewStdioWithIO creates a StdioTransport with custom reader and writer (for testing).
func NewStdioWithIO(r io.Reader, w io.Writer) *StdioTransport {
	return &StdioTransport{reader: r, writer: w}
}

// Start reads lines from the reader, unmarshals them as JSONRPCRequest,
// calls the handler, and writes the response. It blocks until the reader
// is exhausted or the context is cancelled.
func (s *StdioTransport) Start(ctx context.Context, handler RequestHandler) error {
	scanner := bufio.NewScanner(s.reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req mcp.JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		resp, err := handler(ctx, req)
		if err != nil {
			continue
		}

		// Notifications (no ID) don't get a response written back.
		if req.ID == nil {
			continue
		}

		// If handler returned nil response, skip writing.
		if resp == nil {
			continue
		}

		s.mu.Lock()
		data, err := json.Marshal(resp)
		if err != nil {
			s.mu.Unlock()
			continue
		}
		data = append(data, '\n')
		_, _ = s.writer.Write(data)
		s.mu.Unlock()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// SendNotification writes a JSON-RPC notification to the writer.
func (s *StdioTransport) SendNotification(notification mcp.JSONRPCNotification) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.writer.Write(data)
	return err
}

// Close is a no-op for stdio transport.
func (s *StdioTransport) Close() error {
	return nil
}
