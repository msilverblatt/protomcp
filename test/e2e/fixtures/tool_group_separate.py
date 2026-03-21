from protomcp import tool_group, action, ToolResult
from protomcp.runner import run

@tool_group("db", description="Database operations")
class DatabaseTools:
    @action("query", description="Run a SQL query")
    def query(self, sql: str) -> str:
        return f"Results for: {sql}"

    @action("insert", description="Insert a record")
    def insert(self, table: str, data: str) -> str:
        return f"Inserted into {table}: {data}"

if __name__ == "__main__":
    run()
