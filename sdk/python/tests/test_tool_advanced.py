import json
from dataclasses import dataclass
from protomcp.tool import tool, get_registered_tools, clear_registry

def test_tool_with_output_type():
    clear_registry()

    @dataclass
    class SearchResult:
        title: str
        score: float

    @tool(description="Search", output_type=SearchResult)
    def search(query: str) -> SearchResult:
        return SearchResult(title="test", score=0.9)

    tools = get_registered_tools()
    assert tools[0].output_schema_json != ""
    schema = json.loads(tools[0].output_schema_json)
    assert "title" in schema["properties"]
    assert "score" in schema["properties"]

def test_tool_with_metadata():
    clear_registry()

    @tool(description="Delete doc", title="Delete Document", destructive=True, idempotent=True)
    def delete_doc(doc_id: str) -> str:
        return "deleted"

    tools = get_registered_tools()
    assert tools[0].title == "Delete Document"
    assert tools[0].destructive_hint is True
    assert tools[0].idempotent_hint is True

def test_tool_with_task_support():
    clear_registry()

    @tool(description="Long task", task_support=True)
    def long_task(data: str) -> str:
        return "done"

    tools = get_registered_tools()
    assert tools[0].task_support is True
