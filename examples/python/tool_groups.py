# examples/python/tool_groups.py
# Demonstrates tool groups with per-action schemas, validation, and strategies.
# Run: pmcp dev examples/python/tool_groups.py

from typing import Literal, Optional
from protomcp import tool_group, action, ToolResult


@tool_group("db", description="Database operations", strategy="union")
class DatabaseTools:
    """Union strategy: all actions exposed as a single tool with discriminated input."""

    @action("query", description="Run a read-only SQL query", requires=["sql"])
    def query(self, sql: str, limit: int = 100) -> ToolResult:
        return ToolResult(result=f"Executed: {sql} (limit {limit})")

    @action(
        "insert",
        description="Insert a record into a table",
        requires=["table", "data"],
        enum_fields={"table": ["users", "events", "logs"]},
    )
    def insert(self, table: str, data: str) -> ToolResult:
        return ToolResult(result=f"Inserted into {table}: {data}")

    @action("migrate", description="Run a schema migration", requires=["version"])
    def migrate(self, version: str, dry_run: bool = False) -> ToolResult:
        mode = "dry run" if dry_run else "applied"
        return ToolResult(result=f"Migration {version} {mode}")


@tool_group("files", description="File operations", strategy="separate")
class FileTools:
    """Separate strategy: each action becomes its own tool (files.read, files.write, etc.)."""

    @action("read", description="Read a file by path", requires=["path"])
    def read(self, path: str) -> ToolResult:
        return ToolResult(result=f"Contents of {path}")

    @action("write", description="Write content to a file", requires=["path", "content"])
    def write(self, path: str, content: str) -> ToolResult:
        return ToolResult(result=f"Wrote {len(content)} bytes to {path}")

    @action(
        "search",
        description="Search files by pattern",
        requires=["pattern"],
        enum_fields={"scope": ["workspace", "project", "global"]},
    )
    def search(self, pattern: str, scope: Literal["workspace", "project", "global"] = "workspace") -> ToolResult:
        return ToolResult(result=f"Searching '{pattern}' in {scope}")


if __name__ == "__main__":
    from protomcp.runner import run
    run()
