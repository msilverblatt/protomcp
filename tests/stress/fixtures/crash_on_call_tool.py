#!/usr/bin/env python3
"""Tool fixture that crashes mid-call for connection chaos tests.

Responds to ListToolsRequest normally, but exits on the first CallToolRequest.
"""

import os
import sys
import struct
import socket
import json

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
                t.name = "crash_me"
                t.description = "Crashes when called"
                t.input_schema_json = '{"type":"object","properties":{}}'
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                # Crash: close socket abruptly and exit.
                sock.close()
                sys.exit(1)

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        try:
            sock.close()
        except:
            pass


if __name__ == '__main__':
    main()
