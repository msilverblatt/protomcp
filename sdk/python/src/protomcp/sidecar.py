import atexit
import os
import subprocess
import time
import urllib.request
from dataclasses import dataclass
from typing import Callable

_sidecar_registry: list["SidecarDef"] = []
_running_processes: dict[str, subprocess.Popen] = {}

@dataclass
class SidecarDef:
    name: str
    command: list[str]
    health_check: str = ""
    start_on: str = "first_tool_call"
    restart_on_version_mismatch: bool = False
    health_timeout: float = 30.0
    health_interval: float = 1.0
    shutdown_timeout: float = 3.0

    @property
    def pid_file_path(self) -> str:
        return os.path.expanduser(f"~/.protomcp/sidecars/{self.name}.pid")

def sidecar(name: str, command: list[str], health_check: str = "", start_on: str = "first_tool_call",
            restart_on_version_mismatch: bool = False, health_timeout: float = 30.0):
    def decorator(func: Callable) -> Callable:
        _sidecar_registry.append(SidecarDef(
            name=name, command=command, health_check=health_check,
            start_on=start_on, restart_on_version_mismatch=restart_on_version_mismatch,
            health_timeout=health_timeout,
        ))
        return func
    return decorator

def get_registered_sidecars() -> list[SidecarDef]:
    return list(_sidecar_registry)

def clear_sidecar_registry():
    _sidecar_registry.clear()

def _check_health(sc: SidecarDef) -> bool:
    if not sc.health_check:
        return True
    try:
        with urllib.request.urlopen(sc.health_check, timeout=5) as resp:
            return resp.status == 200
    except Exception:
        return False

def _start_sidecar(sc: SidecarDef):
    if sc.name in _running_processes:
        proc = _running_processes[sc.name]
        if proc.poll() is None and _check_health(sc):
            return
    pid_dir = os.path.dirname(sc.pid_file_path)
    os.makedirs(pid_dir, exist_ok=True)
    proc = subprocess.Popen(sc.command, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    _running_processes[sc.name] = proc
    with open(sc.pid_file_path, "w") as f:
        f.write(str(proc.pid))
    if sc.health_check:
        deadline = time.monotonic() + sc.health_timeout
        while time.monotonic() < deadline:
            if _check_health(sc):
                return
            time.sleep(sc.health_interval)

def _stop_sidecar(sc: SidecarDef):
    proc = _running_processes.pop(sc.name, None)
    if proc is None or proc.poll() is not None:
        _cleanup_pid_file(sc)
        return
    try:
        proc.terminate()
        try:
            proc.wait(timeout=sc.shutdown_timeout)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait(timeout=5)
    except ProcessLookupError:
        pass
    _cleanup_pid_file(sc)

def _cleanup_pid_file(sc: SidecarDef):
    try:
        os.remove(sc.pid_file_path)
    except FileNotFoundError:
        pass

def start_sidecars(trigger: str):
    for sc in _sidecar_registry:
        if sc.start_on == trigger:
            _start_sidecar(sc)

def stop_all_sidecars():
    for sc in _sidecar_registry:
        _stop_sidecar(sc)

atexit.register(stop_all_sidecars)
