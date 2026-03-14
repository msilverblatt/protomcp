package protomcp

import (
	"fmt"
	"testing"
)

func TestTelemetrySinkReceivesEvents(t *testing.T) {
	ClearTelemetrySinks()
	defer ClearTelemetrySinks()

	var received []ToolCallEvent

	TelemetrySink(func(event ToolCallEvent) {
		received = append(received, event)
	})

	EmitTelemetry(ToolCallEvent{
		ToolName: "test_tool",
		Phase:    "start",
		Args:     map[string]interface{}{"key": "val"},
	})
	EmitTelemetry(ToolCallEvent{
		ToolName:   "test_tool",
		Phase:      "success",
		Result:     "ok",
		DurationMs: 42,
	})

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Phase != "start" {
		t.Errorf("expected phase 'start', got '%s'", received[0].Phase)
	}
	if received[1].Phase != "success" {
		t.Errorf("expected phase 'success', got '%s'", received[1].Phase)
	}
	if received[1].DurationMs != 42 {
		t.Errorf("expected duration 42, got %d", received[1].DurationMs)
	}
}

func TestTelemetryMultipleSinks(t *testing.T) {
	ClearTelemetrySinks()
	defer ClearTelemetrySinks()

	var count1, count2 int

	TelemetrySink(func(event ToolCallEvent) { count1++ })
	TelemetrySink(func(event ToolCallEvent) { count2++ })

	EmitTelemetry(ToolCallEvent{ToolName: "t", Phase: "start"})

	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both sinks called once, got %d and %d", count1, count2)
	}
}

func TestTelemetryPanicSafe(t *testing.T) {
	ClearTelemetrySinks()
	defer ClearTelemetrySinks()

	var called bool

	TelemetrySink(func(event ToolCallEvent) {
		panic("boom")
	})
	TelemetrySink(func(event ToolCallEvent) {
		called = true
	})

	// Should not panic
	EmitTelemetry(ToolCallEvent{ToolName: "t", Phase: "start"})

	if !called {
		t.Error("second sink should still be called despite first panicking")
	}
}

func TestTelemetryErrorEvent(t *testing.T) {
	ClearTelemetrySinks()
	defer ClearTelemetrySinks()

	var received ToolCallEvent

	TelemetrySink(func(event ToolCallEvent) {
		received = event
	})

	EmitTelemetry(ToolCallEvent{
		ToolName:   "fail_tool",
		Phase:      "error",
		Error:      fmt.Errorf("something went wrong"),
		DurationMs: 100,
		Message:    "failure reason",
	})

	if received.Phase != "error" {
		t.Errorf("expected phase 'error', got '%s'", received.Phase)
	}
	if received.Error == nil || received.Error.Error() != "something went wrong" {
		t.Errorf("expected error, got %v", received.Error)
	}
	if received.Message != "failure reason" {
		t.Errorf("expected message 'failure reason', got '%s'", received.Message)
	}
}

func TestClearTelemetrySinks(t *testing.T) {
	ClearTelemetrySinks()
	defer ClearTelemetrySinks()

	var called bool
	TelemetrySink(func(event ToolCallEvent) { called = true })

	ClearTelemetrySinks()
	EmitTelemetry(ToolCallEvent{ToolName: "t", Phase: "start"})

	if called {
		t.Error("sink should not be called after clear")
	}
}
