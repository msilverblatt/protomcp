"""Real Python SDK process exercised by the Go/MCP integration test."""
import asyncio
from pathlib import Path

from protomcp import tool, ToolResult
from protomcp.runner import run


@tool("Return structured rows after an async suspension", read_only=True)
async def rows() -> ToolResult:
    await asyncio.sleep(0)
    return ToolResult(result="row preview", structured_content={"rows": [[1, "x" * 70000]], "count": 1})


@tool("Suspend until cancelled; record cleanup")
async def wait(marker: str) -> ToolResult:
    Path(marker + ".started").write_text("started")
    try:
        await asyncio.sleep(60)
        return ToolResult(result="unexpected completion")
    finally:
        Path(marker).write_text("cleaned")


@tool("Check dispatcher remains responsive")
def ping() -> ToolResult:
    return ToolResult(result="pong")


if __name__ == "__main__":
    run()
