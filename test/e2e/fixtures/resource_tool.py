from protomcp import tool
from protomcp.resource import resource, resource_template, ResourceContent
from protomcp.runner import run


@tool(description="Echo a message back")
def echo(message: str) -> str:
    return message


@resource(uri="config://app", description="App configuration", name="app_config")
def app_config(uri: str) -> ResourceContent:
    return ResourceContent(uri=uri, text='{"debug": true}', mime_type="application/json")


@resource_template(
    uri_template="notes://{id}",
    description="Get a note by ID",
    name="note",
)
def get_note(uri: str) -> ResourceContent:
    note_id = uri.split("://")[1]
    return ResourceContent(uri=uri, text=f"Note {note_id} content", mime_type="text/plain")


if __name__ == "__main__":
    run()
