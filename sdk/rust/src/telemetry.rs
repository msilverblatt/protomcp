use std::sync::Mutex;

pub struct ToolCallEvent {
    pub tool_name: String,
    pub action: String,
    pub phase: String,
    pub duration_ms: i64,
    pub args_json: String,
    pub result_text: String,
    pub is_error: bool,
    pub error_code: String,
}

impl ToolCallEvent {
    pub fn new(tool_name: &str, phase: &str) -> Self {
        Self {
            tool_name: tool_name.to_string(),
            action: String::new(),
            phase: phase.to_string(),
            duration_ms: 0,
            args_json: String::new(),
            result_text: String::new(),
            is_error: false,
            error_code: String::new(),
        }
    }
}

type SinkFn = Box<dyn Fn(&ToolCallEvent) + Send + Sync>;

static SINK_REGISTRY: Mutex<Vec<SinkFn>> = Mutex::new(Vec::new());

/// Register a telemetry sink that receives ToolCallEvents.
pub fn telemetry_sink(handler: impl Fn(&ToolCallEvent) + Send + Sync + 'static) {
    SINK_REGISTRY.lock().unwrap().push(Box::new(handler));
}

/// Emit a telemetry event to all registered sinks.
/// Fail-safe: catches panics in individual sinks so one bad sink
/// doesn't crash the process.
pub fn emit_telemetry(event: ToolCallEvent) {
    let guard = SINK_REGISTRY.lock().unwrap();
    for sink in guard.iter() {
        let _ = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            sink(&event);
        }));
    }
}

pub fn clear_telemetry_sinks() {
    SINK_REGISTRY.lock().unwrap().clear();
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicI32, Ordering};

    #[test]
    fn test_register_and_emit() {
        clear_telemetry_sinks();

        let count = Arc::new(AtomicI32::new(0));
        let count2 = count.clone();
        telemetry_sink(move |event| {
            assert_eq!(event.tool_name, "my_tool");
            assert_eq!(event.phase, "start");
            count2.fetch_add(1, Ordering::SeqCst);
        });

        emit_telemetry(ToolCallEvent::new("my_tool", "start"));
        assert_eq!(count.load(Ordering::SeqCst), 1);

        clear_telemetry_sinks();
    }

    #[test]
    fn test_multiple_sinks() {
        clear_telemetry_sinks();

        let count_a = Arc::new(AtomicI32::new(0));
        let count_b = Arc::new(AtomicI32::new(0));
        let ca = count_a.clone();
        let cb = count_b.clone();

        telemetry_sink(move |_| { ca.fetch_add(1, Ordering::SeqCst); });
        telemetry_sink(move |_| { cb.fetch_add(1, Ordering::SeqCst); });

        emit_telemetry(ToolCallEvent::new("tool", "success"));
        assert_eq!(count_a.load(Ordering::SeqCst), 1);
        assert_eq!(count_b.load(Ordering::SeqCst), 1);

        clear_telemetry_sinks();
    }

    #[test]
    fn test_panic_in_sink_does_not_crash() {
        clear_telemetry_sinks();

        let reached = Arc::new(AtomicI32::new(0));
        let reached2 = reached.clone();

        telemetry_sink(|_| { panic!("bad sink"); });
        telemetry_sink(move |_| { reached2.fetch_add(1, Ordering::SeqCst); });

        // Should not panic, and second sink should still be called
        emit_telemetry(ToolCallEvent::new("tool", "error"));
        assert_eq!(reached.load(Ordering::SeqCst), 1);

        clear_telemetry_sinks();
    }

    #[test]
    fn test_event_fields() {
        clear_telemetry_sinks();

        let captured_name = Arc::new(Mutex::new(String::new()));
        let cn = captured_name.clone();

        telemetry_sink(move |event| {
            *cn.lock().unwrap() = format!(
                "{}:{}:{}ms:err={}",
                event.tool_name, event.phase, event.duration_ms, event.is_error
            );
        });

        let mut event = ToolCallEvent::new("calc", "success");
        event.duration_ms = 42;
        event.is_error = false;
        emit_telemetry(event);

        assert_eq!(*captured_name.lock().unwrap(), "calc:success:42ms:err=false");

        clear_telemetry_sinks();
    }

    #[test]
    fn test_clear() {
        clear_telemetry_sinks();
        telemetry_sink(|_| {});
        telemetry_sink(|_| {});
        {
            let guard = SINK_REGISTRY.lock().unwrap();
            assert_eq!(guard.len(), 2);
        }
        clear_telemetry_sinks();
        {
            let guard = SINK_REGISTRY.lock().unwrap();
            assert_eq!(guard.len(), 0);
        }
    }
}
