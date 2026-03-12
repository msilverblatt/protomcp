#!/usr/bin/env python3
"""Echo tool test fixture for process manager tests.

Connects to a unix socket specified by PROTOMCP_SOCKET env var,
reads/writes length-prefixed protobuf envelopes, and responds to
ListToolsRequest, CallToolRequest, and ReloadRequest.
"""

import os
import sys
import struct
import socket
import json

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', '..', 'sdk', 'python', 'gen'))
import protomcp_pb2 as pb


def read_envelope(sock):
    """Read a length-prefixed protobuf Envelope from the socket."""
    length_bytes = b''
    while len(length_bytes) < 4:
        chunk = sock.recv(4 - len(length_bytes))
        if not chunk:
            raise ConnectionError("connection closed while reading length prefix")
        length_bytes += chunk

    length = struct.unpack('>I', length_bytes)[0]

    data = b''
    while len(data) < length:
        chunk = sock.recv(length - len(data))
        if not chunk:
            raise ConnectionError("connection closed while reading envelope data")
        data += chunk

    env = pb.Envelope()
    env.ParseFromString(data)
    return env


def write_envelope(sock, env):
    """Write a length-prefixed protobuf Envelope to the socket."""
    data = env.SerializeToString()
    length = struct.pack('>I', len(data))
    sock.sendall(length + data)


def make_tool_list_response(request_id=""):
    """Create a ToolListResponse envelope."""
    env = pb.Envelope()
    env.request_id = request_id
    tool = env.tool_list.tools.add()
    tool.name = "echo"
    tool.description = "Echo back the input"
    tool.input_schema_json = '{"type":"object","properties":{"message":{"type":"string"}},"required":["message"]}'
    return env


def handle_call_tool(request_env):
    """Handle a CallToolRequest by echoing args back."""
    req = request_env.call_tool
    env = pb.Envelope()
    env.request_id = request_env.request_id
    env.call_result.is_error = False
    env.call_result.result_json = req.arguments_json
    return env


def main():
    socket_path = os.environ.get('PROTOMCP_SOCKET')
    if not socket_path:
        print("PROTOMCP_SOCKET env var not set", file=sys.stderr)
        sys.exit(1)

    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.connect(socket_path)

    try:
        while True:
            env = read_envelope(sock)

            if env.HasField('list_tools'):
                resp = make_tool_list_response(env.request_id)
                write_envelope(sock, resp)

            elif env.HasField('call_tool'):
                resp = handle_call_tool(env)
                write_envelope(sock, resp)

            elif env.HasField('reload'):
                # Send ReloadResponse with success=true
                reload_resp = pb.Envelope()
                reload_resp.request_id = env.request_id
                reload_resp.reload_response.success = True
                write_envelope(sock, reload_resp)

                # Immediately send updated ToolListResponse (no request_id)
                tool_list_resp = make_tool_list_response("")
                write_envelope(sock, tool_list_resp)

            else:
                # Unknown message type, ignore
                pass

    except (ConnectionError, BrokenPipeError):
        pass
    finally:
        sock.close()


if __name__ == '__main__':
    main()
