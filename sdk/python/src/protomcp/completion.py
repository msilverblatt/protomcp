from dataclasses import dataclass
from typing import Callable

_completion_registry: dict[tuple[str, str, str], Callable] = {}

@dataclass
class CompletionResult:
    values: list[str]
    total: int = 0
    has_more: bool = False

def completion(ref_type: str, ref_name: str, argument_name: str):
    """Register a completion provider for a specific ref+argument combination.

    ref_type: "ref/prompt" or "ref/resource"
    ref_name: name of the prompt or URI of the resource
    argument_name: which argument to complete
    """
    def decorator(func: Callable) -> Callable:
        _completion_registry[(ref_type, ref_name, argument_name)] = func
        return func
    return decorator

def get_completion_handler(ref_type: str, ref_name: str, argument_name: str):
    return _completion_registry.get((ref_type, ref_name, argument_name))
