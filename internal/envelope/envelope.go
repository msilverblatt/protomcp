package envelope

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"google.golang.org/protobuf/proto"
)

const maxMessageSize = 10 * 1024 * 1024 // 10MB

// Write serializes an Envelope and writes it with a 4-byte big-endian length prefix.
func Write(w io.Writer, env *pb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write envelope data: %w", err)
	}

	return nil
}

// Read reads a length-prefixed Envelope from the reader.
func Read(r io.Reader) (*pb.Envelope, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("read length prefix: %w", err)
	}

	if length > maxMessageSize {
		return nil, fmt.Errorf("message size %d exceeds max %d", length, maxMessageSize)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read envelope data: %w", err)
	}

	env := &pb.Envelope{}
	if err := proto.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return env, nil
}

// ReadRaw reads a length-prefixed Envelope. If the envelope contains a
// RawHeader, it also reads the subsequent raw bytes from the reader.
// Returns (envelope, rawBytes, error). rawBytes is nil for non-RawHeader messages.
func ReadRaw(r io.Reader) (*pb.Envelope, []byte, error) {
	env, err := Read(r)
	if err != nil {
		return nil, nil, err
	}

	rh := env.GetRawHeader()
	if rh == nil {
		return env, nil, nil
	}

	// Read raw bytes that follow the RawHeader
	raw := make([]byte, rh.Size)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, nil, fmt.Errorf("read raw payload (%d bytes): %w", rh.Size, err)
	}

	// Decompress if the payload was compressed
	if rh.Compression == "zstd" {
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			return nil, nil, fmt.Errorf("create zstd decoder: %w", err)
		}
		defer decoder.Close()
		decompressed, err := decoder.DecodeAll(raw, make([]byte, 0, rh.UncompressedSize))
		if err != nil {
			return nil, nil, fmt.Errorf("zstd decompress (%d bytes): %w", rh.Size, err)
		}
		raw = decompressed
	}

	return env, raw, nil
}
