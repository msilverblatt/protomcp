#!/usr/bin/env python3
"""FastMCP server for comparison benchmarks.

Provides the same tools as the protomcp echo fixture so we can do
apples-to-apples comparison across multiple workload types.
"""

import sys
import json
import hashlib

try:
    from fastmcp import FastMCP
except ImportError:
    print("FastMCP not installed. Install with: pip install fastmcp", file=sys.stderr)
    sys.exit(1)

mcp = FastMCP("benchmark")


@mcp.tool()
def echo(message: str) -> str:
    """Echo a message back."""
    return message


@mcp.tool()
def add(a: int, b: int) -> str:
    """Add two numbers."""
    return str(a + b)


@mcp.tool()
def compute(iterations: int) -> str:
    """CPU-bound work: hash a string N times."""
    result = "seed"
    for _ in range(iterations):
        result = hashlib.sha256(result.encode()).hexdigest()
    return result


@mcp.tool()
def generate(size: int) -> str:
    """Return a string of the requested size in bytes."""
    return "X" * size


@mcp.tool()
def parse_json(data: str) -> str:
    """Parse JSON and return it serialized back."""
    parsed = json.loads(data)
    return json.dumps(parsed)


if __name__ == "__main__":
    mcp.run()
