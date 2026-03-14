package testengine

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"
)

// TraceEntry records a single JSON-RPC message.
type TraceEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // "send" or "recv"
	Raw       string    `json:"raw"`       // full JSON-RPC message
	Method    string    `json:"method,omitempty"`
}

// TraceLog provides thread-safe storage and pub/sub for protocol trace entries.
type TraceLog struct {
	mu          sync.RWMutex
	entries     []TraceEntry
	subscribers map[chan TraceEntry]struct{}
}

// NewTraceLog creates a new TraceLog.
func NewTraceLog() *TraceLog {
	return &TraceLog{
		subscribers: make(map[chan TraceEntry]struct{}),
	}
}

// Writer returns an io.Writer that parses LoggingTransport output lines
// and records them as TraceEntries.
func (t *TraceLog) Writer() io.Writer {
	return &traceWriter{log: t}
}

// Entries returns a snapshot of all recorded entries.
func (t *TraceLog) Entries() []TraceEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]TraceEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

// Subscribe returns a channel that receives new entries as they are recorded.
func (t *TraceLog) Subscribe() chan TraceEntry {
	ch := make(chan TraceEntry, 64)
	t.mu.Lock()
	t.subscribers[ch] = struct{}{}
	t.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscription channel.
func (t *TraceLog) Unsubscribe(ch chan TraceEntry) {
	t.mu.Lock()
	delete(t.subscribers, ch)
	t.mu.Unlock()
}

// Clear removes all recorded entries.
func (t *TraceLog) Clear() {
	t.mu.Lock()
	t.entries = nil
	t.mu.Unlock()
}

func (t *TraceLog) add(entry TraceEntry) {
	t.mu.Lock()
	t.entries = append(t.entries, entry)
	for ch := range t.subscribers {
		select {
		case ch <- entry:
		default:
		}
	}
	t.mu.Unlock()
}

// traceWriter implements io.Writer and parses LoggingTransport output.
// Lines have the form:
//
//	write: {"jsonrpc":"2.0",...}
//	read: {"jsonrpc":"2.0",...}
type traceWriter struct {
	log  *TraceLog
	buf  []byte
	scan *bufio.Scanner
	pr   *io.PipeReader
	pw   *io.PipeWriter
	once sync.Once
}

func (w *traceWriter) init() {
	w.pr, w.pw = io.Pipe()
	w.scan = bufio.NewScanner(w.pr)
	w.scan.Buffer(make([]byte, 1024*1024), 1024*1024)
	go w.readLines()
}

func (w *traceWriter) Write(p []byte) (int, error) {
	w.once.Do(w.init)
	return w.pw.Write(p)
}

func (w *traceWriter) readLines() {
	for w.scan.Scan() {
		line := w.scan.Text()
		w.parseLine(line)
	}
}

func (w *traceWriter) parseLine(line string) {
	var direction string
	var payload string

	if strings.HasPrefix(line, "write: ") {
		direction = "send"
		payload = strings.TrimPrefix(line, "write: ")
	} else if strings.HasPrefix(line, "read: ") {
		direction = "recv"
		payload = strings.TrimPrefix(line, "read: ")
	} else {
		return
	}

	entry := TraceEntry{
		Timestamp: time.Now(),
		Direction: direction,
		Raw:       payload,
	}

	// Try to extract method from JSON
	var msg struct {
		Method string `json:"method"`
	}
	if json.Unmarshal([]byte(payload), &msg) == nil && msg.Method != "" {
		entry.Method = msg.Method
	}

	w.log.add(entry)
}
