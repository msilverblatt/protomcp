import protomcp.protomcp_pb2 as pb

_transport = None

def _get_transport():
    if _transport is None:
        raise RuntimeError("protomcp not connected")
    return _transport

def _init(transport):
    global _transport
    _transport = transport

def enable(tool_names: list[str]) -> list[str]:
    t = _get_transport()
    env = pb.Envelope(enable_tools=pb.EnableToolsRequest(tool_names=tool_names))
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)

def disable(tool_names: list[str]) -> list[str]:
    t = _get_transport()
    env = pb.Envelope(disable_tools=pb.DisableToolsRequest(tool_names=tool_names))
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)

def set_allowed(tool_names: list[str]) -> list[str]:
    t = _get_transport()
    env = pb.Envelope(set_allowed=pb.SetAllowedRequest(tool_names=tool_names))
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)

def set_blocked(tool_names: list[str]) -> list[str]:
    t = _get_transport()
    env = pb.Envelope(set_blocked=pb.SetBlockedRequest(tool_names=tool_names))
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)

def get_active_tools() -> list[str]:
    t = _get_transport()
    env = pb.Envelope(get_active_tools=pb.GetActiveToolsRequest())
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)

def batch(enable=None, disable=None, allow=None, block=None) -> list[str]:
    t = _get_transport()
    env = pb.Envelope(batch=pb.BatchUpdateRequest(
        enable=enable or [], disable=disable or [],
        allow=allow or [], block=block or [],
    ))
    t.send(env)
    resp = t.recv()
    return list(resp.active_tools.tool_names)
