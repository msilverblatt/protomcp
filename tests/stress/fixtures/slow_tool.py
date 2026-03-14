#!/usr/bin/env python3
"""Slow echo tool fixture for stress tests.

Same as echo_tool.py but adds a configurable delay (from the 'delay' argument)
before responding, to simulate tools that take time.
"""

import os
import sys
import struct
import socket
import json
import time

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
                resp = pb.Envelope()
                resp.request_id = env.request_id
                t = resp.tool_list.tools.add()
                t.name = "slow_echo"
                t.description = "Echo with delay"
                t.input_schema_json = json.dumps({
                    "type": "object",
                    "properties": {
                        "message": {"type": "string"},
                        "delay": {"type": "number"},
                    },
                    "required": ["message"],
                })
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                req = env.call_tool
                args = json.loads(req.arguments_json) if req.arguments_json else {}
                delay = float(args.get("delay", 0))
                if delay > 0:
                    time.sleep(delay)
                resp = pb.Envelope()
                resp.request_id = env.request_id
                resp.call_result.is_error = False
                resp.call_result.result_json = json.dumps(
                    [{"type": "text", "text": args.get("message", "")}]
                )
                write_envelope(sock, resp)

            elif env.HasField('reload'):
                reload_resp = pb.Envelope()
                reload_resp.request_id = env.request_id
                reload_resp.reload_response.success = True
                write_envelope(sock, reload_resp)

                unsolicited = pb.Envelope()
                t = unsolicited.tool_list.tools.add()
                t.name = "slow_echo"
                t.description = "Echo with delay"
                t.input_schema_json = json.dumps({
                    "type": "object",
                    "properties": {
                        "message": {"type": "string"},
                        "delay": {"type": "number"},
                    },
                    "required": ["message"],
                })
                write_envelope(sock, unsolicited)

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        sock.close()


if __name__ == '__main__':
    main()
