# examples/python/basic.py
# A minimal protomcp tool — adds two numbers.
# Run: pmcp dev examples/python/basic.py

from protomcp import tool, ToolResult

@tool("Add two numbers")
def add(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a + b))

@tool("Multiply two numbers")
def multiply(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a * b))
