# examples/python/advanced_server.py
# Demonstrates server_context, local_middleware, telemetry_sink, and tool_group together.
# Run: pmcp dev examples/python/advanced_server.py

import os
import time
from protomcp import (
    tool_group, action, ToolResult,
    server_context, local_middleware, telemetry_sink, ToolCallEvent,
)


# --- Server context: inject project_dir into handlers that declare it ---

@server_context("project_dir", expose=False)
def resolve_project_dir(args: dict) -> str:
    """Resolve the current project directory from env or cwd."""
    return os.environ.get("PROJECT_DIR", os.getcwd())


# --- Local middleware: timing and error formatting ---

@local_middleware(priority=10)
def timing_middleware(ctx, tool_name: str, args: dict, next_handler):
    """Wrap every tool call with timing info."""
    start = time.time()
    result = next_handler(ctx, args)
    elapsed_ms = (time.time() - start) * 1000
    if isinstance(result, ToolResult) and not result.is_error:
        return ToolResult(result=f"{result.result}\n[{elapsed_ms:.1f}ms]")
    return result


@local_middleware(priority=20)
def error_format_middleware(ctx, tool_name: str, args: dict, next_handler):
    """Catch exceptions and return structured errors."""
    try:
        return next_handler(ctx, args)
    except FileNotFoundError as e:
        return ToolResult(
            result=str(e),
            is_error=True,
            error_code="FILE_NOT_FOUND",
            suggestion="Check that the path exists",
            retryable=False,
        )
    except Exception as e:
        return ToolResult(
            result=f"Internal error: {e}",
            is_error=True,
            error_code="INTERNAL",
            retryable=True,
        )


# --- Telemetry sink: log all tool call events ---

@telemetry_sink
def log_events(event: ToolCallEvent):
    """Log telemetry events to stderr (visible in pmcp dev logs)."""
    if event.phase == "start":
        print(f"[telemetry] {event.tool_name} started (action={event.action})")
    elif event.phase == "success":
        print(f"[telemetry] {event.tool_name} completed in {event.duration_ms}ms")
    elif event.phase == "error":
        print(f"[telemetry] {event.tool_name} failed: {event.error}")


# --- Tool group using all features ---

@tool_group("project", description="Project management operations")
class ProjectTools:

    @action("list_files", description="List files in the project directory")
    def list_files(self, project_dir: str, pattern: str = "*") -> ToolResult:
        import fnmatch
        entries = os.listdir(project_dir)
        matched = [e for e in entries if fnmatch.fnmatch(e, pattern)]
        return ToolResult(result="\n".join(matched) if matched else "No files found")

    @action(
        "read_config",
        description="Read a project config file",
        requires=["filename"],
        enum_fields={"filename": ["pyproject.toml", "setup.cfg", "package.json", "Cargo.toml"]},
    )
    def read_config(self, project_dir: str, filename: str) -> ToolResult:
        path = os.path.join(project_dir, filename)
        if not os.path.isfile(path):
            raise FileNotFoundError(f"Config not found: {path}")
        with open(path) as f:
            return ToolResult(result=f.read())

    @action("stats", description="Show project directory stats")
    def stats(self, project_dir: str) -> ToolResult:
        total = sum(1 for _ in os.scandir(project_dir))
        return ToolResult(result=f"{total} entries in {project_dir}")


if __name__ == "__main__":
    from protomcp.runner import run
    run()
