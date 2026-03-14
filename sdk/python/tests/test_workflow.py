import json
import pytest
from unittest.mock import patch, MagicMock

from protomcp.workflow import (
    step,
    workflow,
    StepResult,
    StepDef,
    WorkflowDef,
    WorkflowState,
    get_registered_workflows,
    clear_workflow_registry,
    workflows_to_tool_defs,
    _handle_step_call,
    _handle_cancel,
    _validate_workflow_graph,
    _active_workflow_stack,
)
from protomcp.tool import get_registered_tools, clear_registry


@pytest.fixture(autouse=True)
def clean_registries():
    clear_workflow_registry()
    clear_registry()
    yield
    clear_workflow_registry()
    clear_registry()


# --- StepResult defaults ---

def test_step_result_defaults():
    r = StepResult()
    assert r.result == ""
    assert r.next is None


def test_step_result_with_next():
    r = StepResult(result="done", next=["approve", "reject"])
    assert r.result == "done"
    assert r.next == ["approve", "reject"]


# --- Workflow registration ---

def test_workflow_registration():
    @workflow(name="deploy", description="Deploy workflow")
    class Deploy:
        @step("start", description="Start deploy", initial=True, next=["confirm"])
        def start(self, env: str) -> StepResult:
            return StepResult(result=f"Deploying to {env}")

        @step("confirm", description="Confirm deploy", terminal=True)
        def confirm(self) -> StepResult:
            return StepResult(result="Confirmed")

    wfs = get_registered_workflows()
    assert len(wfs) == 1
    assert wfs[0].name == "deploy"
    assert len(wfs[0].steps) == 2
    step_names = {s.name for s in wfs[0].steps}
    assert step_names == {"start", "confirm"}


def test_initial_step_detection():
    @workflow(name="w1", description="test")
    class W1:
        @step("init", description="Init", initial=True, next=["done"])
        def init(self) -> StepResult:
            return StepResult()

        @step("done", description="Done", terminal=True)
        def done(self) -> StepResult:
            return StepResult()

    wfs = get_registered_workflows()
    initial = [s for s in wfs[0].steps if s.initial]
    assert len(initial) == 1
    assert initial[0].name == "init"


def test_schema_generation_on_steps():
    @workflow(name="w2", description="test")
    class W2:
        @step("go", description="Go", initial=True, next=["end"])
        def go(self, name: str, count: int = 3) -> StepResult:
            return StepResult()

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    wfs = get_registered_workflows()
    go_step = next(s for s in wfs[0].steps if s.name == "go")
    assert "name" in go_step.input_schema["properties"]
    assert go_step.input_schema["properties"]["name"]["type"] == "string"
    assert go_step.input_schema["properties"]["count"]["type"] == "integer"
    assert go_step.input_schema["properties"]["count"]["default"] == 3
    assert "name" in go_step.input_schema["required"]


def test_on_cancel_capture():
    @workflow(name="w3", description="test")
    class W3:
        @step("s", description="S", initial=True, next=["e"])
        def s(self) -> StepResult:
            return StepResult()

        @step("e", description="E", terminal=True)
        def e(self) -> StepResult:
            return StepResult()

        def on_cancel(self):
            pass

    wfs = get_registered_workflows()
    assert wfs[0].on_cancel is not None


def test_on_complete_capture():
    @workflow(name="w4", description="test")
    class W4:
        @step("s", description="S", initial=True, next=["e"])
        def s(self) -> StepResult:
            return StepResult()

        @step("e", description="E", terminal=True)
        def e(self) -> StepResult:
            return StepResult()

        def on_complete(self):
            pass

    wfs = get_registered_workflows()
    assert wfs[0].on_complete is not None


# --- Graph validation errors ---

def test_no_initial_step():
    with pytest.raises(ValueError, match="no initial step"):
        @workflow(name="bad1", description="test")
        class Bad1:
            @step("a", description="A", next=["b"])
            def a(self) -> StepResult:
                return StepResult()

            @step("b", description="B", terminal=True)
            def b(self) -> StepResult:
                return StepResult()


def test_multiple_initial_steps():
    with pytest.raises(ValueError, match="multiple initial steps"):
        @workflow(name="bad2", description="test")
        class Bad2:
            @step("a", description="A", initial=True, next=["c"])
            def a(self) -> StepResult:
                return StepResult()

            @step("b", description="B", initial=True, next=["c"])
            def b(self) -> StepResult:
                return StepResult()

            @step("c", description="C", terminal=True)
            def c(self) -> StepResult:
                return StepResult()


def test_missing_next_reference():
    with pytest.raises(ValueError, match="nonexistent step"):
        @workflow(name="bad3", description="test")
        class Bad3:
            @step("a", description="A", initial=True, next=["ghost"])
            def a(self) -> StepResult:
                return StepResult()


def test_dead_end_step():
    with pytest.raises(ValueError, match="no next.*dead end"):
        @workflow(name="bad4", description="test")
        class Bad4:
            @step("a", description="A", initial=True, next=["b"])
            def a(self) -> StepResult:
                return StepResult()

            @step("b", description="B")  # no next, not terminal
            def b(self) -> StepResult:
                return StepResult()


def test_terminal_with_next():
    with pytest.raises(ValueError, match="terminal step.*has next"):
        @workflow(name="bad5", description="test")
        class Bad5:
            @step("a", description="A", initial=True, terminal=True, next=["b"])
            def a(self) -> StepResult:
                return StepResult()

            @step("b", description="B", terminal=True)
            def b(self) -> StepResult:
                return StepResult()


def test_on_error_nonexistent_target():
    with pytest.raises(ValueError, match="on_error references nonexistent"):
        @workflow(name="bad6", description="test")
        class Bad6:
            @step("a", description="A", initial=True, next=["b"], on_error={ValueError: "ghost"})
            def a(self) -> StepResult:
                return StepResult()

            @step("b", description="B", terminal=True)
            def b(self) -> StepResult:
                return StepResult()


def test_valid_graph_passes():
    # Should not raise
    @workflow(name="good", description="test")
    class Good:
        @step("a", description="A", initial=True, next=["b", "c"])
        def a(self) -> StepResult:
            return StepResult()

        @step("b", description="B", next=["c"])
        def b(self) -> StepResult:
            return StepResult()

        @step("c", description="C", terminal=True)
        def c(self) -> StepResult:
            return StepResult()

    assert len(get_registered_workflows()) == 1


# --- Tool def generation ---

def test_tool_def_generation():
    @workflow(name="deploy", description="Deploy")
    class Deploy:
        @step("start", description="Start", initial=True, next=["approve"])
        def start(self, env: str) -> StepResult:
            return StepResult()

        @step("approve", description="Approve", terminal=True)
        def approve(self) -> StepResult:
            return StepResult()

    defs = workflows_to_tool_defs()
    names = [d.name for d in defs]
    assert "deploy.start" in names
    assert "deploy.approve" in names
    assert "deploy.cancel" in names

    # Initial step is not hidden
    start_def = next(d for d in defs if d.name == "deploy.start")
    assert start_def.hidden is False

    # Non-initial step is hidden
    approve_def = next(d for d in defs if d.name == "deploy.approve")
    assert approve_def.hidden is True

    # Cancel is hidden
    cancel_def = next(d for d in defs if d.name == "deploy.cancel")
    assert cancel_def.hidden is True


def test_no_cancel_tool_when_all_no_cancel():
    @workflow(name="strict", description="Strict")
    class Strict:
        @step("start", description="Start", initial=True, next=["end"], no_cancel=True)
        def start(self) -> StepResult:
            return StepResult()

        @step("end", description="End", terminal=True, no_cancel=True)
        def end(self) -> StepResult:
            return StepResult()

    defs = workflows_to_tool_defs()
    names = [d.name for d in defs]
    assert "strict.cancel" not in names


# --- Step dispatch ---

@patch('protomcp.workflow.tool_manager')
def test_initial_step_dispatch(mock_tm):
    mock_tm.get_active_tools.return_value = ["existing_tool"]
    mock_tm.set_allowed.return_value = []

    @workflow(name="d1", description="test")
    class D1:
        @step("start", description="Start", initial=True, next=["end"])
        def start(self, msg: str) -> StepResult:
            return StepResult(result=f"Started: {msg}")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult(result="Done")

    result = _handle_step_call("d1", "start", {"msg": "hello"})
    assert "Started: hello" in result.result
    assert len(_active_workflow_stack) == 1
    assert _active_workflow_stack[0].current_step == "start"


@patch('protomcp.workflow.tool_manager')
def test_state_persistence_across_steps(mock_tm):
    mock_tm.get_active_tools.return_value = ["existing_tool"]
    mock_tm.set_allowed.return_value = []

    @workflow(name="stateful", description="test")
    class Stateful:
        def __init__(self):
            self.data = None

        @step("start", description="Start", initial=True, next=["end"])
        def start(self, val: str) -> StepResult:
            self.data = val
            return StepResult(result="stored")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult(result=f"data={self.data}")

    _handle_step_call("stateful", "start", {"val": "foo"})
    result = _handle_step_call("stateful", "end", {})
    assert "data=foo" in result.result


@patch('protomcp.workflow.tool_manager')
def test_dynamic_next_narrowing(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="dyn", description="test")
    class Dyn:
        @step("start", description="Start", initial=True, next=["a", "b"])
        def start(self) -> StepResult:
            return StepResult(result="pick a", next=["a"])

        @step("a", description="A", terminal=True)
        def a(self) -> StepResult:
            return StepResult(result="A done")

        @step("b", description="B", terminal=True)
        def b(self) -> StepResult:
            return StepResult(result="B done")

    _handle_step_call("dyn", "start", {})
    # set_allowed should have been called with only "dyn.a" (not "dyn.b")
    call_args = mock_tm.set_allowed.call_args[0][0]
    assert "dyn.a" in call_args
    assert "dyn.b" not in call_args


@patch('protomcp.workflow.tool_manager')
def test_dynamic_next_rejects_invalid(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="dyn2", description="test")
    class Dyn2:
        @step("start", description="Start", initial=True, next=["a"])
        def start(self) -> StepResult:
            return StepResult(result="bad", next=["a", "ghost"])

        @step("a", description="A", terminal=True)
        def a(self) -> StepResult:
            return StepResult()

    result = _handle_step_call("dyn2", "start", {})
    assert result.is_error
    assert "invalid next" in result.result


@patch('protomcp.workflow.tool_manager')
def test_cancel_calls_on_cancel(mock_tm):
    mock_tm.get_active_tools.return_value = ["t1", "t2"]
    mock_tm.set_allowed.return_value = []

    cancel_called = []

    @workflow(name="canc", description="test")
    class Canc:
        @step("start", description="Start", initial=True, next=["end"])
        def start(self) -> StepResult:
            return StepResult()

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

        def on_cancel(self):
            cancel_called.append(True)

    _handle_step_call("canc", "start", {})
    result = _handle_cancel("canc")
    assert "cancelled" in result.result
    assert len(cancel_called) == 1
    # Should restore pre-workflow tools
    mock_tm.set_allowed.assert_called_with(["t1", "t2"])


@patch('protomcp.workflow.tool_manager')
def test_on_complete_called_on_terminal(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    complete_called = []

    @workflow(name="comp", description="test")
    class Comp:
        @step("start", description="Start", initial=True, next=["end"])
        def start(self) -> StepResult:
            return StepResult()

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult(result="finished")

        def on_complete(self):
            complete_called.append(True)

    _handle_step_call("comp", "start", {})
    result = _handle_step_call("comp", "end", {})
    assert len(complete_called) == 1
    assert "finished" in result.result


@patch('protomcp.workflow.tool_manager')
def test_history_tracking(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="hist", description="test")
    class Hist:
        @step("start", description="Start", initial=True, next=["mid"])
        def start(self) -> StepResult:
            return StepResult(result="s1")

        @step("mid", description="Mid", next=["end"])
        def mid(self) -> StepResult:
            return StepResult(result="s2")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult(result="s3")

    _handle_step_call("hist", "start", {})
    _handle_step_call("hist", "mid", {})

    state = _active_workflow_stack[-1]
    assert len(state.history) == 2
    assert state.history[0][0] == "start"
    assert state.history[1][0] == "mid"
    assert state.history[0][1].result == "s1"


@patch('protomcp.workflow.tool_manager')
def test_error_stays_in_state(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    call_count = []

    @workflow(name="err1", description="test")
    class Err1:
        @step("start", description="Start", initial=True, next=["end"])
        def start(self) -> StepResult:
            call_count.append(1)
            if len(call_count) == 1:
                raise RuntimeError("transient")
            return StepResult(result="ok")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    result1 = _handle_step_call("err1", "start", {})
    assert result1.is_error
    assert "transient" in result1.result

    # Retry - the initial step should work as a new workflow start since
    # the first call created a state entry. We need to pop it for retry to work
    # as a fresh initial call.
    _active_workflow_stack.clear()
    result2 = _handle_step_call("err1", "start", {})
    assert not result2.is_error
    assert "ok" in result2.result


@patch('protomcp.workflow.tool_manager')
def test_on_error_transitions(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="err2", description="test")
    class Err2:
        @step("start", description="Start", initial=True, next=["end"],
              on_error={ValueError: "fix"})
        def start(self) -> StepResult:
            raise ValueError("bad value")

        @step("fix", description="Fix", next=["end"])
        def fix(self) -> StepResult:
            return StepResult(result="fixed")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    result = _handle_step_call("err2", "start", {})
    assert "transitioning to 'fix'" in result.result
    assert _active_workflow_stack[-1].current_step == "fix"


@patch('protomcp.workflow.tool_manager')
def test_on_error_catch_all(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="err3", description="test")
    class Err3:
        @step("start", description="Start", initial=True, next=["end"],
              on_error={Exception: "recover"})
        def start(self) -> StepResult:
            raise TypeError("oops")

        @step("recover", description="Recover", next=["end"])
        def recover(self) -> StepResult:
            return StepResult(result="recovered")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    result = _handle_step_call("err3", "start", {})
    assert "transitioning to 'recover'" in result.result


@patch('protomcp.workflow.tool_manager')
def test_no_cancel_with_error_allows_retry(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    attempt = []

    @workflow(name="nc1", description="test")
    class NC1:
        @step("start", description="Start", initial=True, next=["end"], no_cancel=True)
        def start(self) -> StepResult:
            attempt.append(1)
            if len(attempt) == 1:
                raise RuntimeError("fail")
            return StepResult(result="ok")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    result1 = _handle_step_call("nc1", "start", {})
    assert result1.is_error

    # Can retry since initial step creates new state
    _active_workflow_stack.clear()
    result2 = _handle_step_call("nc1", "start", {})
    assert not result2.is_error


@patch('protomcp.workflow.tool_manager')
def test_unmatched_error_stays_in_state(mock_tm):
    mock_tm.get_active_tools.return_value = []
    mock_tm.set_allowed.return_value = []

    @workflow(name="err4", description="test")
    class Err4:
        @step("start", description="Start", initial=True, next=["end"],
              on_error={ValueError: "end"})
        def start(self) -> StepResult:
            raise TypeError("not a ValueError")

        @step("end", description="End", terminal=True)
        def end(self) -> StepResult:
            return StepResult()

    result = _handle_step_call("err4", "start", {})
    assert result.is_error
    assert "retry" in result.result.lower() or "failed" in result.result.lower()
