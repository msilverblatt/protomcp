import pytest
from protomcp.telemetry import (
    telemetry_sink,
    ToolCallEvent,
    emit_telemetry,
    clear_telemetry_sinks,
    get_telemetry_sinks,
)


@pytest.fixture(autouse=True)
def _clean_sinks():
    clear_telemetry_sinks()
    yield
    clear_telemetry_sinks()


def test_register_sink():
    events = []

    @telemetry_sink
    def my_sink(event):
        events.append(event)

    sinks = get_telemetry_sinks()
    assert len(sinks) == 1
    assert sinks[0] is my_sink


def test_emit_start():
    events = []

    @telemetry_sink
    def capture(event):
        events.append(event)

    evt = ToolCallEvent(tool_name="my_tool", phase="start", args={"key": "val"}, action="do_thing")
    emit_telemetry(evt)

    assert len(events) == 1
    assert events[0].tool_name == "my_tool"
    assert events[0].phase == "start"
    assert events[0].args == {"key": "val"}
    assert events[0].action == "do_thing"


def test_emit_success():
    events = []

    @telemetry_sink
    def capture(event):
        events.append(event)

    evt = ToolCallEvent(
        tool_name="my_tool", phase="success", args={},
        result="some result", duration_ms=42,
    )
    emit_telemetry(evt)

    assert len(events) == 1
    assert events[0].phase == "success"
    assert events[0].result == "some result"
    assert events[0].duration_ms == 42


def test_emit_error():
    events = []

    @telemetry_sink
    def capture(event):
        events.append(event)

    err = ValueError("boom")
    evt = ToolCallEvent(
        tool_name="my_tool", phase="error", args={},
        error=err, duration_ms=10,
    )
    emit_telemetry(evt)

    assert len(events) == 1
    assert events[0].phase == "error"
    assert events[0].error is err
    assert events[0].duration_ms == 10


def test_sink_failure_swallowed():
    """A failing sink should not prevent other sinks from running or raise."""
    events = []

    @telemetry_sink
    def bad_sink(event):
        raise RuntimeError("sink exploded")

    @telemetry_sink
    def good_sink(event):
        events.append(event)

    evt = ToolCallEvent(tool_name="t", phase="start", args={})
    emit_telemetry(evt)  # should not raise

    assert len(events) == 1


def test_multiple_sinks():
    a_events = []
    b_events = []

    @telemetry_sink
    def sink_a(event):
        a_events.append(event)

    @telemetry_sink
    def sink_b(event):
        b_events.append(event)

    evt = ToolCallEvent(tool_name="x", phase="start", args={})
    emit_telemetry(evt)

    assert len(a_events) == 1
    assert len(b_events) == 1


def test_progress_event():
    events = []

    @telemetry_sink
    def capture(event):
        events.append(event)

    evt = ToolCallEvent(
        tool_name="download", phase="progress", args={},
        progress=50, total=100, message="halfway",
    )
    emit_telemetry(evt)

    assert len(events) == 1
    assert events[0].phase == "progress"
    assert events[0].progress == 50
    assert events[0].total == 100
    assert events[0].message == "halfway"
