package testengine

import (
	"testing"
	"time"
)

func TestTraceWriter(t *testing.T) {
	tl := NewTraceLog()
	w := tl.Writer()

	// Write log lines as the LoggingTransport would
	lines := []string{
		"write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n",
		"read: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":\"2024-11-05\"}}\n",
		"write: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/initialized\"}\n",
	}
	for _, line := range lines {
		_, err := w.Write([]byte(line))
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Give the background goroutine time to process
	time.Sleep(100 * time.Millisecond)

	entries := tl.Entries()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Direction != "send" {
		t.Errorf("entry 0 direction = %q, want %q", entries[0].Direction, "send")
	}
	if entries[0].Method != "initialize" {
		t.Errorf("entry 0 method = %q, want %q", entries[0].Method, "initialize")
	}

	if entries[1].Direction != "recv" {
		t.Errorf("entry 1 direction = %q, want %q", entries[1].Direction, "recv")
	}
	if entries[1].Method != "initialize response" {
		t.Errorf("entry 1 method = %q, want %q", entries[1].Method, "initialize response")
	}

	if entries[2].Direction != "send" {
		t.Errorf("entry 2 direction = %q, want %q", entries[2].Direction, "send")
	}
	if entries[2].Method != "notifications/initialized" {
		t.Errorf("entry 2 method = %q, want %q", entries[2].Method, "notifications/initialized")
	}
}

func TestTraceLogSubscribe(t *testing.T) {
	tl := NewTraceLog()
	ch := tl.Subscribe()
	defer tl.Unsubscribe(ch)

	w := tl.Writer()
	_, err := w.Write([]byte("write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	select {
	case entry := <-ch:
		if entry.Direction != "send" {
			t.Errorf("direction = %q, want %q", entry.Direction, "send")
		}
		if entry.Method != "tools/list" {
			t.Errorf("method = %q, want %q", entry.Method, "tools/list")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for subscriber notification")
	}
}

func TestTraceLogClear(t *testing.T) {
	tl := NewTraceLog()
	w := tl.Writer()

	_, err := w.Write([]byte("write: {\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"test\"}\n"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	if len(tl.Entries()) == 0 {
		t.Fatal("expected entries before clear")
	}

	tl.Clear()

	if len(tl.Entries()) != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", len(tl.Entries()))
	}
}
