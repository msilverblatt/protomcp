from dataclasses import dataclass
from typing import Any, Callable, Optional

_sink_registry: list[Callable] = []

@dataclass
class ToolCallEvent:
    tool_name: str
    phase: str  # "start", "success", "error", "progress"
    args: dict
    action: str = ""
    result: str = ""
    error: Optional[Exception] = None
    duration_ms: int = 0
    progress: int = 0
    total: int = 0
    message: str = ""

def telemetry_sink(func: Callable) -> Callable:
    _sink_registry.append(func)
    return func

def get_telemetry_sinks() -> list[Callable]:
    return list(_sink_registry)

def clear_telemetry_sinks():
    _sink_registry.clear()

def emit_telemetry(event: ToolCallEvent):
    for sink in _sink_registry:
        try:
            sink(event)
        except Exception:
            pass
