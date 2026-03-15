from protomcp import tool
from protomcp.prompt import prompt, PromptArg, PromptMessage
from protomcp.runner import run


@tool(description="Echo a message back")
def echo(message: str) -> str:
    return message


@prompt(
    description="Generate a greeting",
    arguments=[PromptArg(name="name", description="Name to greet", required=True)],
)
def greet(name: str) -> list:
    return [PromptMessage(role="user", content=f"Please greet {name} warmly.")]


if __name__ == "__main__":
    run()
