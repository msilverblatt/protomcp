from dataclasses import dataclass
from typing import Callable, Any

_middleware_registry: list["MiddlewareDef"] = []

@dataclass
class MiddlewareDef:
    name: str
    priority: int
    handler: Callable  # (phase, tool_name, args_json, result_json, is_error) -> dict

def middleware(name: str, priority: int = 100):
    def decorator(func: Callable) -> Callable:
        _middleware_registry.append(MiddlewareDef(
            name=name,
            priority=priority,
            handler=func,
        ))
        return func
    return decorator

def get_registered_middleware() -> list[MiddlewareDef]:
    return list(_middleware_registry)

def clear_middleware_registry():
    _middleware_registry.clear()
