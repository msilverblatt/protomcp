import importlib
import importlib.util
import os
import sys
from pathlib import Path
from typing import Any

_config: dict[str, Any] = {}
_loaded_modules: dict[str, Any] = {}

def configure(handlers_dir: str = "", hot_reload: bool = False):
    _config["handlers_dir"] = handlers_dir
    _config["hot_reload"] = hot_reload

def get_config() -> dict[str, Any]:
    return dict(_config)

def reset_config():
    _config.clear()
    _loaded_modules.clear()

def discover_handlers():
    handlers_dir = _config.get("handlers_dir")
    if not handlers_dir:
        return
    handlers_path = Path(handlers_dir).resolve()
    if not handlers_path.is_dir():
        return
    hot_reload = _config.get("hot_reload", False)
    if hot_reload and _loaded_modules:
        from protomcp.tool import clear_registry
        from protomcp.group import clear_group_registry
        from protomcp.workflow import clear_workflow_registry
        from protomcp.server_context import clear_context_registry
        from protomcp.local_middleware import clear_local_middleware
        clear_registry()
        clear_group_registry()
        clear_workflow_registry()
        clear_context_registry()
        clear_local_middleware()
        _loaded_modules.clear()
    for py_file in sorted(handlers_path.glob("*.py")):
        if py_file.name.startswith("_"):
            continue
        module_name = f"_protomcp_handler_{py_file.stem}"
        spec = importlib.util.spec_from_file_location(module_name, str(py_file))
        if spec is None or spec.loader is None:
            continue
        module = importlib.util.module_from_spec(spec)
        sys.modules[module_name] = module
        spec.loader.exec_module(module)
        _loaded_modules[str(py_file)] = module
