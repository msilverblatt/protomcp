from protomcp import tool
from protomcp.runner import run
import sys

@tool(description="Crash the process")
def crash() -> str:
    sys.exit(1)

if __name__ == "__main__":
    run()
