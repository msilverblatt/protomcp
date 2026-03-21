from protomcp import tool
from protomcp.runner import run

@tool(description="Original tool")
def original() -> str:
    return "v1"

if __name__ == "__main__":
    run()
