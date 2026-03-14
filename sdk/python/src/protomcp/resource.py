import json
from dataclasses import dataclass, field
from typing import Callable, Optional

_resource_registry: list["ResourceDef"] = []
_resource_template_registry: list["ResourceTemplateDef"] = []

@dataclass
class ResourceDef:
    uri: str
    name: str
    description: str
    handler: Callable  # (uri: str) -> ResourceContent or list[ResourceContent]
    mime_type: str = ""
    size: int = 0

@dataclass
class ResourceContent:
    uri: str
    text: str = ""
    blob: bytes = b""
    mime_type: str = ""

@dataclass
class ResourceTemplateDef:
    uri_template: str
    name: str
    description: str
    handler: Callable  # (uri: str) -> ResourceContent or list[ResourceContent]
    mime_type: str = ""

def resource(uri: str, description: str, name: str = "", mime_type: str = ""):
    """Decorator to register a resource handler."""
    def decorator(func: Callable) -> Callable:
        _resource_registry.append(ResourceDef(
            uri=uri,
            name=name or func.__name__,
            description=description,
            handler=func,
            mime_type=mime_type,
        ))
        return func
    return decorator

def resource_template(uri_template: str, description: str, name: str = "", mime_type: str = ""):
    """Decorator to register a resource template handler."""
    def decorator(func: Callable) -> Callable:
        _resource_template_registry.append(ResourceTemplateDef(
            uri_template=uri_template,
            name=name or func.__name__,
            description=description,
            handler=func,
            mime_type=mime_type,
        ))
        return func
    return decorator

def get_registered_resources() -> list[ResourceDef]:
    return list(_resource_registry)

def get_registered_resource_templates() -> list[ResourceTemplateDef]:
    return list(_resource_template_registry)
