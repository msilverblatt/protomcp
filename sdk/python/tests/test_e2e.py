"""End-to-end integration tests exercising multiple features together."""

import os
import tempfile
import time

from protomcp.tool import tool, get_registered_tools, clear_registry
from protomcp.group import (
    tool_group,
    action,
    clear_group_registry,
    _dispatch_group_action,
    _group_registry,
)
from protomcp.local_middleware import (
    local_middleware,
    clear_local_middleware,
    build_middleware_chain,
)
from protomcp.server_context import server_context, clear_context_registry
from protomcp.telemetry import (
    telemetry_sink,
    ToolCallEvent,
    emit_telemetry,
    clear_telemetry_sinks,
)
from protomcp.result import ToolResult
from protomcp.discovery import configure, discover_handlers, reset_config


def _clear_all():
    clear_registry()
    clear_group_registry()
    clear_local_middleware()
    clear_context_registry()
    clear_telemetry_sinks()
    reset_config()


# ---------------------------------------------------------------------------
# test_tool_group_with_middleware_and_telemetry
# ---------------------------------------------------------------------------

def test_tool_group_with_middleware_and_telemetry():
    _clear_all()

    telemetry_events = []

    @telemetry_sink
    def capture(event: ToolCallEvent):
        telemetry_events.append(event)

    @local_middleware(priority=10)
    def timing_mw(ctx, tool_name, args, next_handler):
        start = time.monotonic()
        result = next_handler(ctx, args)
        elapsed = time.monotonic() - start
        if isinstance(result, ToolResult):
            return ToolResult(
                result=result.result + f" [timing={elapsed:.4f}s]",
                is_error=result.is_error,
            )
        return str(result) + f" [timing={elapsed:.4f}s]"

    @tool_group(name="math", description="Math operations")
    class MathGroup:
        @action("add", description="Add two numbers")
        def add(self, a: int, b: int):
            return ToolResult(result=str(a + b))

        @action("multiply", description="Multiply two numbers")
        def multiply(self, a: int, b: int):
            return ToolResult(result=str(a * b))

    tools = get_registered_tools()
    assert len(tools) == 2
    tool_names = [t.name for t in tools]
    assert "math.add" in tool_names

    add_tool = next(t for t in tools if t.name == "math.add")
    handler = add_tool.handler
    chain = build_middleware_chain("math.add", handler)

    emit_telemetry(ToolCallEvent(tool_name="math.add", action="add", phase="start", args={"a": 2, "b": 3}))
    result = chain(None, {"a": 2, "b": 3})
    emit_telemetry(ToolCallEvent(tool_name="math.add", action="add", phase="success", args={}, result=str(result)))

    # Handler was called correctly — result contains the sum
    assert "5" in str(result)
    # Middleware modified the result — timing info appended
    assert "[timing=" in str(result)
    # Telemetry captured start + success
    phases = [e.phase for e in telemetry_events]
    assert "start" in phases
    assert "success" in phases


# ---------------------------------------------------------------------------
# test_tool_group_with_validation_and_hints
# ---------------------------------------------------------------------------

def test_tool_group_with_validation_and_hints():
    _clear_all()

    @tool_group(name="deploy", description="Deployment tool")
    class DeployGroup:
        @action(
            "run",
            description="Run a deployment",
            requires=["env"],
            enum_fields={"env": ["staging", "production", "development"]},
            hints={
                "prod_warning": {
                    "condition": lambda kwargs: kwargs.get("env") == "production",
                    "message": "Warning: deploying to production!",
                },
            },
        )
        def run(self, env: str, tag: str = "latest"):
            return ToolResult(result=f"Deployed {tag} to {env}")

    group = _group_registry[-1]

    # Missing required field
    result = _dispatch_group_action(group, action="run")
    assert isinstance(result, ToolResult)
    assert result.is_error is True
    assert result.error_code == "MISSING_REQUIRED"
    assert "env" in result.result

    # Invalid enum — fuzzy suggestion
    result = _dispatch_group_action(group, action="run", env="prodution")
    assert isinstance(result, ToolResult)
    assert result.is_error is True
    assert result.error_code == "INVALID_ENUM"
    assert result.suggestion is not None
    assert "production" in result.suggestion

    # Valid call with hint triggered
    result = _dispatch_group_action(group, action="run", env="production", tag="v2.0")
    assert isinstance(result, ToolResult)
    assert result.is_error is False
    assert "Deployed v2.0 to production" in result.result
    assert "Warning: deploying to production!" in result.result


# ---------------------------------------------------------------------------
# test_tool_group_with_server_context
# ---------------------------------------------------------------------------

def test_tool_group_with_server_context():
    _clear_all()

    @server_context("project_dir", expose=False)
    def resolve_project_dir(args: dict):
        return "/home/user/myproject"

    @tool_group(name="files", description="File operations")
    class FilesGroup:
        @action("list", description="List files in project dir")
        def list_files(self, project_dir: str):
            return ToolResult(result=f"Files in {project_dir}")

    group = _group_registry[-1]
    result = _dispatch_group_action(group, action="list")
    assert isinstance(result, ToolResult)
    assert result.is_error is False
    assert "/home/user/myproject" in result.result


# ---------------------------------------------------------------------------
# test_separate_strategy_with_middleware
# ---------------------------------------------------------------------------

def test_separate_strategy_with_middleware():
    _clear_all()

    @local_middleware(priority=10)
    def tag_mw(ctx, tool_name, args, next_handler):
        result = next_handler(ctx, args)
        return str(result) + " [tagged]"

    @tool_group(name="db", description="Database ops", strategy="separate")
    class DbGroup:
        @action("query", description="Run a query")
        def query(self, sql: str):
            return f"Result of: {sql}"

        @action("migrate", description="Run migrations")
        def migrate(self, version: str):
            return f"Migrated to {version}"

    tools = get_registered_tools()
    tool_names = [t.name for t in tools]
    assert "db.query" in tool_names
    assert "db.migrate" in tool_names

    query_tool = next(t for t in tools if t.name == "db.query")
    chain = build_middleware_chain("db.query", query_tool.handler)
    result = chain(None, {"sql": "SELECT 1"})
    assert "Result of: SELECT 1" in str(result)
    assert "[tagged]" in str(result)


# ---------------------------------------------------------------------------
# test_individual_tool_with_middleware_and_telemetry
# ---------------------------------------------------------------------------

def test_individual_tool_with_middleware_and_telemetry():
    _clear_all()

    telemetry_events = []

    @telemetry_sink
    def capture(event: ToolCallEvent):
        telemetry_events.append(event)

    @local_middleware(priority=10)
    def prefix_mw(ctx, tool_name, args, next_handler):
        result = next_handler(ctx, args)
        return f"[{tool_name}] " + str(result)

    @tool(description="Say hello")
    def greet(name: str):
        return f"Hello, {name}!"

    tools = get_registered_tools()
    greet_tool = next(t for t in tools if t.name == "greet")

    chain = build_middleware_chain("greet", greet_tool.handler)
    emit_telemetry(ToolCallEvent(tool_name="greet", phase="start", args={"name": "Alice"}))
    result = chain(None, {"name": "Alice"})
    emit_telemetry(ToolCallEvent(tool_name="greet", phase="success", args={}, result=str(result)))

    assert "Hello, Alice!" in str(result)
    assert "[greet]" in str(result)
    phases = [e.phase for e in telemetry_events]
    assert "start" in phases
    assert "success" in phases


# ---------------------------------------------------------------------------
# test_multiple_groups_coexist
# ---------------------------------------------------------------------------

def test_multiple_groups_coexist():
    _clear_all()

    @tool_group(name="alpha", description="Alpha group")
    class AlphaGroup:
        @action("do_a", description="Action A")
        def do_a(self):
            return "alpha_a"

    @tool_group(name="beta", description="Beta group")
    class BetaGroup:
        @action("do_b", description="Action B")
        def do_b(self):
            return "beta_b"

    @tool(description="Standalone tool")
    def standalone():
        return "standalone_result"

    tools = get_registered_tools()
    tool_names = [t.name for t in tools]
    assert "alpha.do_a" in tool_names
    assert "beta.do_b" in tool_names
    assert "standalone" in tool_names

    # Dispatch each independently
    alpha_group = _group_registry[0]
    beta_group = _group_registry[1]

    result_a = _dispatch_group_action(alpha_group, action="do_a")
    assert str(result_a) == "alpha_a"

    result_b = _dispatch_group_action(beta_group, action="do_b")
    assert str(result_b) == "beta_b"

    standalone_tool = next(t for t in tools if t.name == "standalone")
    result_s = standalone_tool.handler()
    assert result_s == "standalone_result"


# ---------------------------------------------------------------------------
# test_telemetry_captures_errors
# ---------------------------------------------------------------------------

def test_telemetry_captures_errors():
    _clear_all()

    telemetry_events = []

    @telemetry_sink
    def capture(event: ToolCallEvent):
        telemetry_events.append(event)

    @tool_group(name="risky", description="Risky operations")
    class RiskyGroup:
        @action("explode", description="This will fail")
        def explode(self):
            raise ValueError("boom!")

    tools = get_registered_tools()
    risky_tool = next(t for t in tools if t.name == "risky.explode")

    chain = build_middleware_chain("risky.explode", risky_tool.handler)
    emit_telemetry(ToolCallEvent(tool_name="risky.explode", action="explode", phase="start", args={}))

    error_caught = None
    try:
        chain(None, {})
    except ValueError as e:
        error_caught = e
        emit_telemetry(ToolCallEvent(tool_name="risky.explode", action="explode", phase="error", args={}, error=e))

    assert error_caught is not None
    assert str(error_caught) == "boom!"

    phases = [e.phase for e in telemetry_events]
    assert "start" in phases
    assert "error" in phases

    error_event = next(e for e in telemetry_events if e.phase == "error")
    assert error_event.error is not None
    assert "boom" in str(error_event.error)


# ---------------------------------------------------------------------------
# test_discovery_registers_groups
# ---------------------------------------------------------------------------

def test_discovery_registers_groups():
    _clear_all()

    handler_code = '''\
from protomcp.group import tool_group, action

@tool_group(name="discovered", description="Auto-discovered group")
class DiscoveredGroup:
    @action("ping", description="Ping action")
    def ping(self):
        return "pong"
'''

    with tempfile.TemporaryDirectory() as tmpdir:
        handler_file = os.path.join(tmpdir, "my_handler.py")
        with open(handler_file, "w") as f:
            f.write(handler_code)

        configure(handlers_dir=tmpdir)
        discover_handlers()

        tools = get_registered_tools()
        tool_names = [t.name for t in tools]
        assert "discovered.ping" in tool_names

        discovered_tool = next(t for t in tools if t.name == "discovered.ping")
        result = discovered_tool.handler()
        assert str(result) == "pong"
