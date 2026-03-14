"""Benchmark fixture using the protomcp SDK runner.

Unlike echo_tool.py (raw protocol), this goes through the SDK's runner
which includes chunked streaming for large payloads when
PROTOMCP_CHUNK_THRESHOLD is set.
"""
from protomcp import tool
from protomcp.runner import run

@tool(description="Echo the input back")
def echo(message: str) -> str:
    return message

@tool(description="Return a string of the requested size in bytes")
def generate(size: int) -> str:
    return "X" * size

if __name__ == "__main__":
    run()
