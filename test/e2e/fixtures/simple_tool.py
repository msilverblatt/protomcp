from protomcp import tool
from protomcp.runner import run

@tool(description="Echo a message back")
def echo(message: str) -> str:
    return message

@tool(description="Add two numbers")
def add(a: int, b: int) -> int:
    return a + b

if __name__ == "__main__":
    run()
