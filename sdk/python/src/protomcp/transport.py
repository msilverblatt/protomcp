import socket
import struct
import sys
import os

# Add gen directory to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'gen'))
import protomcp_pb2 as pb

class Transport:
    def __init__(self, socket_path: str):
        self._socket_path = socket_path
        self._sock: socket.socket | None = None

    def connect(self):
        self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self._sock.connect(self._socket_path)

    def send(self, envelope: pb.Envelope):
        data = envelope.SerializeToString()
        length = struct.pack(">I", len(data))
        self._sock.sendall(length + data)

    def recv(self) -> pb.Envelope:
        length_bytes = self._recv_exactly(4)
        length = struct.unpack(">I", length_bytes)[0]
        data = self._recv_exactly(length)
        env = pb.Envelope()
        env.ParseFromString(data)
        return env

    def close(self):
        if self._sock:
            self._sock.close()

    def _recv_exactly(self, n: int) -> bytes:
        buf = bytearray()
        while len(buf) < n:
            chunk = self._sock.recv(n - len(buf))
            if not chunk:
                raise ConnectionError("socket closed")
            buf.extend(chunk)
        return bytes(buf)
