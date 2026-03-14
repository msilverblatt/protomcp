package protomcp

import "sync"

// ToolCallEvent represents a telemetry event for tool calls.
type ToolCallEvent struct {
	ToolName   string
	Action     string
	Phase      string // "start", "success", "error"
	Args       map[string]interface{}
	Result     string
	Error      error
	DurationMs int
	Progress   int
	Total      int
	Message    string
}

var (
	telemetrySinks []func(ToolCallEvent)
	telemetryMu    sync.Mutex
)

// TelemetrySink registers a telemetry handler that receives tool call events.
func TelemetrySink(handler func(ToolCallEvent)) {
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	telemetrySinks = append(telemetrySinks, handler)
}

// EmitTelemetry sends an event to all registered sinks. Fail-safe: swallows panics.
func EmitTelemetry(event ToolCallEvent) {
	telemetryMu.Lock()
	sinks := make([]func(ToolCallEvent), len(telemetrySinks))
	copy(sinks, telemetrySinks)
	telemetryMu.Unlock()

	for _, sink := range sinks {
		func() {
			defer func() { recover() }()
			sink(event)
		}()
	}
}

// GetTelemetrySinks returns the count of registered sinks.
func GetTelemetrySinks() int {
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	return len(telemetrySinks)
}

// ClearTelemetrySinks removes all registered telemetry sinks.
func ClearTelemetrySinks() {
	telemetryMu.Lock()
	defer telemetryMu.Unlock()
	telemetrySinks = nil
}
