from dataclasses import dataclass
from typing import Any, Callable

_context_registry: list["ContextDef"] = []

@dataclass
class ContextDef:
    param_name: str
    resolver: Callable[[dict], Any]
    expose: bool  # whether param should appear in tool schemas

def server_context(param_name: str, expose: bool = True):
    """Register a context resolver that injects a value into tool handlers."""
    def decorator(func: Callable) -> Callable:
        _context_registry.append(ContextDef(
            param_name=param_name, resolver=func, expose=expose,
        ))
        return func
    return decorator

def get_registered_contexts() -> list[ContextDef]:
    return list(_context_registry)

def clear_context_registry():
    _context_registry.clear()

def resolve_contexts(args: dict) -> dict[str, Any]:
    """Run all registered context resolvers against args. Returns resolved values."""
    resolved: dict[str, Any] = {}
    for ctx_def in _context_registry:
        resolved[ctx_def.param_name] = ctx_def.resolver(args)
    return resolved

def get_hidden_context_params() -> set[str]:
    """Return param names that should NOT appear in tool schemas (expose=False)."""
    return {c.param_name for c in _context_registry if not c.expose}
