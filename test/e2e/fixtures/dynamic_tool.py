from protomcp import tool, ToolResult
from protomcp.runner import run

@tool(description="Authenticate")
def auth(token: str) -> ToolResult:
    if token == "valid":
        return ToolResult(
            result="Authenticated",
            enable_tools=["admin_action"],
            disable_tools=["auth"],
        )
    return ToolResult(is_error=True, message="Invalid token", suggestion="Use 'valid' as token")

@tool(description="Admin action (hidden until auth)")
def admin_action() -> str:
    return "admin stuff done"

if __name__ == "__main__":
    run()
