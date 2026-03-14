#!/usr/bin/env python3
"""Echo tool fixture for stress tests.

Connects to the unix socket at PROTOMCP_SOCKET, speaks length-prefixed
protobuf, and handles ListToolsRequest, CallToolRequest, and ReloadRequest.

This is a raw-socket implementation (no SDK dependency) so the test fixtures
are fully self-contained.
"""

import os
import sys
import struct
import socket
import json
import time

# Add the generated protobuf code to the path.
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


def make_tool_list(request_id=""):
    env = pb.Envelope()
    env.request_id = request_id
    t = env.tool_list.tools.add()
    t.name = "echo"
    t.description = "Echo the input back"
    t.input_schema_json = json.dumps({
        "type": "object",
        "properties": {"message": {"type": "string"}},
        "required": ["message"],
    })
    return env


def main():
    socket_path = os.environ.get('PROTOMCP_SOCKET')
    if not socket_path:
        print("PROTOMCP_SOCKET not set", file=sys.stderr)
        sys.exit(1)

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(socket_path)

    try:
        while True:
            env = read_envelope(sock)

            if env.HasField('list_tools'):
                resp = make_tool_list(env.request_id)
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                req = env.call_tool
                resp = pb.Envelope()
                resp.request_id = env.request_id
                # Echo the arguments back as the result.
                resp.call_result.is_error = False
                resp.call_result.result_json = json.dumps(
                    [{"type": "text", "text": req.arguments_json}]
                )
                write_envelope(sock, resp)

            elif env.HasField('reload'):
                reload_resp = pb.Envelope()
                reload_resp.request_id = env.request_id
                reload_resp.reload_response.success = True
                write_envelope(sock, reload_resp)
                # Send unsolicited tool list after reload.
                write_envelope(sock, make_tool_list(""))

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        sock.close()


if __name__ == '__main__':
    main()
