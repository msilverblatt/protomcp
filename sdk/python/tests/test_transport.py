import socket
import struct
import sys
import os
import threading

sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'gen'))
import protomcp_pb2 as pb

from protomcp.transport import Transport


def test_send_recv_roundtrip():
    """Test send/recv using a unix socketpair."""
    s1, s2 = socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM)

    try:
        # Create a transport and inject the socket directly
        transport = Transport.__new__(Transport)
        transport._sock = s1

        # Build an envelope to send
        env = pb.Envelope(
            list_tools=pb.ListToolsRequest(),
            request_id="test-123",
        )

        # Send from transport side
        transport.send(env)

        # Read from s2 side (simulating the other end)
        length_bytes = s2.recv(4)
        length = struct.unpack(">I", length_bytes)[0]
        data = s2.recv(length)

        received = pb.Envelope()
        received.ParseFromString(data)

        assert received.request_id == "test-123"
        assert received.HasField("list_tools")

        # Now test recv: send a response back through s2
        resp = pb.Envelope(
            tool_list=pb.ToolListResponse(tools=[
                pb.ToolDefinition(name="my_tool", description="A tool"),
            ]),
            request_id="test-123",
        )
        resp_data = resp.SerializeToString()
        s2.sendall(struct.pack(">I", len(resp_data)) + resp_data)

        # Receive via transport
        result = transport.recv()
        assert result.request_id == "test-123"
        assert result.HasField("tool_list")
        assert len(result.tool_list.tools) == 1
        assert result.tool_list.tools[0].name == "my_tool"
    finally:
        s1.close()
        s2.close()


def test_connection_closed():
    """Test that recv raises ConnectionError when socket closes."""
    s1, s2 = socket.socketpair(socket.AF_UNIX, socket.SOCK_STREAM)

    transport = Transport.__new__(Transport)
    transport._sock = s1

    s2.close()

    try:
        transport.recv()
        assert False, "Should have raised ConnectionError"
    except ConnectionError:
        pass
    finally:
        s1.close()
