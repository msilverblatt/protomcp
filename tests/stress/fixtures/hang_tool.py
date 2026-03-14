#!/usr/bin/env python3
"""Hanging tool fixture for stress tests.

Responds to ListToolsRequest normally but never responds to CallToolRequest,
simulating a tool process that hangs.
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
                t.name = "hang"
                t.description = "This tool hangs forever"
                t.input_schema_json = '{"type":"object","properties":{}}'
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                # Never respond -- hang forever.
                while True:
                    time.sleep(3600)

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        sock.close()


if __name__ == '__main__':
    main()
