#!/usr/bin/env python3
"""Benchmark tool fixture for protomcp.

Provides the same tools as the FastMCP comparison fixture so we can do
apples-to-apples comparison across multiple workload types.
"""

import os
import sys
import struct
import socket
import json
import hashlib

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '..', 'sdk', 'python', 'gen'))
import protomcp_pb2 as pb


def read_envelope(sock):
    length_bytes = _recv_exactly(sock, 4)
    length = struct.unpack('>I', length_bytes)[0]
    data = _recv_exactly(sock, length)
    env = pb.Envelope()
    env.ParseFromString(data)
    return env


def write_envelope(sock, env):
    data = env.SerializeToString()
    length = struct.pack('>I', len(data))
    sock.sendall(length + data)


def _recv_exactly(sock, n):
    buf = bytearray()
    while len(buf) < n:
        chunk = sock.recv(n - len(buf))
        if not chunk:
            raise ConnectionError("socket closed")
        buf.extend(chunk)
    return bytes(buf)


TOOLS = [
    {
        "name": "echo",
        "description": "Echo the input back",
        "schema": {
            "type": "object",
            "properties": {"message": {"type": "string"}},
            "required": ["message"],
        },
    },
    {
        "name": "add",
        "description": "Add two numbers",
        "schema": {
            "type": "object",
            "properties": {"a": {"type": "integer"}, "b": {"type": "integer"}},
            "required": ["a", "b"],
        },
    },
    {
        "name": "compute",
        "description": "CPU-bound work: hash a string N times",
        "schema": {
            "type": "object",
            "properties": {"iterations": {"type": "integer"}},
            "required": ["iterations"],
        },
    },
    {
        "name": "generate",
        "description": "Return a string of the requested size in bytes",
        "schema": {
            "type": "object",
            "properties": {"size": {"type": "integer"}},
            "required": ["size"],
        },
    },
    {
        "name": "parse_json",
        "description": "Parse JSON and return it serialized back",
        "schema": {
            "type": "object",
            "properties": {"data": {"type": "string"}},
            "required": ["data"],
        },
    },
]


def handle_call(req):
    args = json.loads(req.arguments_json) if req.arguments_json else {}

    if req.name == "echo":
        return json.dumps([{"type": "text", "text": args.get("message", "")}])
    elif req.name == "add":
        result = args.get("a", 0) + args.get("b", 0)
        return json.dumps([{"type": "text", "text": str(result)}])
    elif req.name == "compute":
        iterations = args.get("iterations", 1)
        result = "seed"
        for _ in range(iterations):
            result = hashlib.sha256(result.encode()).hexdigest()
        return json.dumps([{"type": "text", "text": result}])
    elif req.name == "generate":
        size = args.get("size", 0)
        return json.dumps([{"type": "text", "text": "X" * size}])
    elif req.name == "parse_json":
        data = args.get("data", "{}")
        parsed = json.loads(data)
        return json.dumps([{"type": "text", "text": json.dumps(parsed)}])
    else:
        return json.dumps([{"type": "text", "text": f"unknown tool: {req.name}"}])


def main():
    socket_path = os.environ.get('PROTOMCP_SOCKET')
    if not socket_path:
        print("PROTOMCP_SOCKET not set", file=sys.stderr)
        sys.exit(1)

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # Increase socket buffers to prevent blocking under concurrent load from
    # the Go process manager writing multiple requests simultaneously.
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 1024 * 1024)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 1024 * 1024)
    sock.connect(socket_path)

    try:
        while True:
            env = read_envelope(sock)

            if env.HasField('list_tools'):
                resp = pb.Envelope()
                resp.request_id = env.request_id
                for tool_def in TOOLS:
                    t = resp.tool_list.tools.add()
                    t.name = tool_def["name"]
                    t.description = tool_def["description"]
                    t.input_schema_json = json.dumps(tool_def["schema"])
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                resp = pb.Envelope()
                resp.request_id = env.request_id
                resp.call_result.is_error = False
                resp.call_result.result_json = handle_call(env.call_tool)
                write_envelope(sock, resp)

            elif env.HasField('reload'):
                reload_resp = pb.Envelope()
                reload_resp.request_id = env.request_id
                reload_resp.reload_response.success = True
                write_envelope(sock, reload_resp)
                unsolicited = pb.Envelope()
                for tool_def in TOOLS:
                    t = unsolicited.tool_list.tools.add()
                    t.name = tool_def["name"]
                    t.description = tool_def["description"]
                    t.input_schema_json = json.dumps(tool_def["schema"])
                write_envelope(sock, unsolicited)

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        sock.close()


if __name__ == '__main__':
    main()
