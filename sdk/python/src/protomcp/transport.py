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

    def send_chunked(self, request_id: str, field_name: str, data: bytes,
                     chunk_size: int = 65536):
        """Send a large field as StreamHeader + StreamChunk messages."""
        header = pb.Envelope(
            request_id=request_id,
            stream_header=pb.StreamHeader(
                field_name=field_name,
                total_size=len(data),
                chunk_size=chunk_size,
            ),
        )
        self.send(header)

        offset = 0
        while offset < len(data):
            end = min(offset + chunk_size, len(data))
            is_final = (end >= len(data))
            chunk_env = pb.Envelope(
                request_id=request_id,
                stream_chunk=pb.StreamChunk(
                    data=data[offset:end],
                    final=is_final,
                ),
            )
            self.send(chunk_env)
            offset = end

    def send_raw(self, request_id: str, field_name: str, data: bytes):
        """Send a large field as a RawHeader + raw bytes (no protobuf wrapping on payload)."""
        compression = ""
        uncompressed_size = 0
        threshold = int(os.environ.get("PROTOMCP_COMPRESS_THRESHOLD", "65536"))
        if len(data) > threshold:
            import zstandard
            compressor = zstandard.ZstdCompressor()
            uncompressed_size = len(data)
            data = compressor.compress(data)
            compression = "zstd"
        header = pb.Envelope(
            raw_header=pb.RawHeader(
                request_id=request_id,
                field_name=field_name,
                size=len(data),
                compression=compression,
                uncompressed_size=uncompressed_size,
            ),
        )
        # Send the protobuf header normally
        header_bytes = header.SerializeToString()
        length = struct.pack(">I", len(header_bytes))
        # Send header + raw payload in one sendall to minimize syscalls
        self._sock.sendall(length + header_bytes + data)

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
