from dataclasses import dataclass, field
from typing import Callable

_prompt_registry: list["PromptDef"] = []

@dataclass
class PromptArg:
    name: str
    description: str = ""
    required: bool = False

@dataclass
class PromptMessage:
    role: str  # "user" or "assistant"
    content: str  # text content
    content_type: str = "text"  # for future extension

@dataclass
class PromptDef:
    name: str
    description: str
    handler: Callable  # (**kwargs) -> list[PromptMessage] or PromptMessage
    arguments: list[PromptArg] = field(default_factory=list)

def prompt(description: str, arguments: list[PromptArg] | None = None):
    """Decorator to register a prompt handler."""
    def decorator(func: Callable) -> Callable:
        _prompt_registry.append(PromptDef(
            name=func.__name__,
            description=description,
            handler=func,
            arguments=arguments or [],
        ))
        return func
    return decorator

def get_registered_prompts() -> list[PromptDef]:
    return list(_prompt_registry)

def clear_prompt_registry():
    _prompt_registry.clear()
