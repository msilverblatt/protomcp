# examples/python/resources_and_prompts.py
# Demonstrates resources, prompts, and completions — the full MCP feature set.
# Run: pmcp dev examples/python/resources_and_prompts.py

from protomcp import (
    tool, ToolResult,
    resource, resource_template, ResourceContent,
    prompt, PromptArg, PromptMessage,
    completion, CompletionResult,
)

# ─── Resources ───────────────────────────────────────────────────────────────
# Resources expose data that MCP clients can read — config files, database
# records, API responses, or any data your tools work with.

NOTES = {
    "meeting-2024-01-15": "Discussed Q1 roadmap. Action items: hire 2 engineers.",
    "meeting-2024-01-22": "Sprint review. Shipped auth system. Demo went well.",
    "meeting-2024-02-01": "Retrospective. Need better test coverage.",
}

@resource(
    uri="notes://index",
    description="List of all available meeting notes",
    mime_type="application/json",
)
def notes_index(uri: str) -> ResourceContent:
    """Returns a JSON list of all note IDs."""
    import json
    ids = list(NOTES.keys())
    return ResourceContent(uri=uri, text=json.dumps(ids), mime_type="application/json")


@resource_template(
    uri_template="notes://{note_id}",
    description="Read a specific meeting note by ID",
    mime_type="text/plain",
)
def read_note(uri: str) -> ResourceContent:
    """Reads a single note. The URI is notes://<note_id>."""
    note_id = uri.replace("notes://", "")
    text = NOTES.get(note_id, f"Note not found: {note_id}")
    return ResourceContent(uri=uri, text=text)


# ─── Prompts ─────────────────────────────────────────────────────────────────
# Prompts are reusable message templates. The MCP client can list them and
# fill in arguments to generate context-rich conversations.

@prompt(
    description="Summarize a meeting note",
    arguments=[
        PromptArg(name="note_id", description="ID of the note to summarize", required=True),
        PromptArg(name="style", description="Summary style: brief, detailed, or bullet"),
    ],
)
def summarize_note(note_id: str, style: str = "brief") -> list[PromptMessage]:
    text = NOTES.get(note_id, f"Note not found: {note_id}")
    return [
        PromptMessage(role="user", content=f"Summarize this meeting note in a {style} style:\n\n{text}"),
    ]


@prompt(
    description="Compare two meeting notes",
    arguments=[
        PromptArg(name="note_a", description="First note ID", required=True),
        PromptArg(name="note_b", description="Second note ID", required=True),
    ],
)
def compare_notes(note_a: str, note_b: str) -> list[PromptMessage]:
    text_a = NOTES.get(note_a, "Not found")
    text_b = NOTES.get(note_b, "Not found")
    return [
        PromptMessage(
            role="user",
            content=f"Compare these two meeting notes and highlight what changed:\n\nNote A ({note_a}):\n{text_a}\n\nNote B ({note_b}):\n{text_b}",
        ),
    ]


# ─── Completions ─────────────────────────────────────────────────────────────
# Completions provide autocomplete suggestions for prompt arguments.
# The MCP client calls these as the user types.

@completion("ref/prompt", "summarize_note", "note_id")
def complete_note_id(value: str) -> CompletionResult:
    """Suggest note IDs matching the partial input."""
    matches = [nid for nid in NOTES if nid.startswith(value)]
    return CompletionResult(values=matches, total=len(matches))


@completion("ref/prompt", "summarize_note", "style")
def complete_style(value: str) -> list[str]:
    """Suggest summary styles."""
    styles = ["brief", "detailed", "bullet"]
    return [s for s in styles if s.startswith(value)]


# ─── Tools ───────────────────────────────────────────────────────────────────
# Tools still work exactly the same. You can mix tools, resources, and prompts
# in one file.

@tool("Search meeting notes by keyword")
def search_notes(query: str) -> ToolResult:
    import json
    matches = {nid: text for nid, text in NOTES.items() if query.lower() in text.lower()}
    return ToolResult(result=json.dumps(matches))
