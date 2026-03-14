package protomcp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"

	"github.com/klauspost/compress/zstd"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"google.golang.org/protobuf/proto"
)

type Transport struct {
	socketPath string
	conn       net.Conn
}

func NewTransport(socketPath string) *Transport {
	return &Transport{socketPath: socketPath}
}

func (t *Transport) Connect() error {
	conn, err := net.Dial("unix", t.socketPath)
	if err != nil {
		return fmt.Errorf("connect to socket: %w", err)
	}
	t.conn = conn
	return nil
}

func (t *Transport) Send(env *pb.Envelope) error {
	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(data)))
	if _, err := t.conn.Write(length); err != nil {
		return err
	}
	_, err = t.conn.Write(data)
	return err
}

// SendRaw sends a RawHeader envelope followed by raw payload bytes.
// This avoids protobuf serialization overhead for large payloads.
// If the payload exceeds PROTOMCP_COMPRESS_THRESHOLD (default 64KB),
// it is zstd-compressed before sending.
func (t *Transport) SendRaw(requestID, fieldName string, data []byte) error {
	compression := ""
	var uncompressedSize uint64
	threshold := 65536
	if v := os.Getenv("PROTOMCP_COMPRESS_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			threshold = n
		}
	}
	if len(data) > threshold {
		encoder, err := zstd.NewWriter(nil)
		if err != nil {
			return fmt.Errorf("create zstd encoder: %w", err)
		}
		compressed := encoder.EncodeAll(data, make([]byte, 0, len(data)))
		encoder.Close()
		uncompressedSize = uint64(len(data))
		data = compressed
		compression = "zstd"
	}

	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId:        requestID,
				FieldName:        fieldName,
				Size:             uint64(len(data)),
				Compression:      compression,
				UncompressedSize: uncompressedSize,
			},
		},
	}
	headerBytes, err := proto.Marshal(header)
	if err != nil {
		return fmt.Errorf("marshal raw header: %w", err)
	}
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(headerBytes)))

	// Write length-prefixed header + raw payload
	buf := make([]byte, 0, 4+len(headerBytes)+len(data))
	buf = append(buf, length...)
	buf = append(buf, headerBytes...)
	buf = append(buf, data...)
	_, err = t.conn.Write(buf)
	return err
}

func (t *Transport) Recv() (*pb.Envelope, error) {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(t.conn, lengthBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lengthBuf)
	data := make([]byte, length)
	if _, err := io.ReadFull(t.conn, data); err != nil {
		return nil, err
	}
	env := &pb.Envelope{}
	if err := proto.Unmarshal(data, env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}
	return env, nil
}

func (t *Transport) Close() {
	if t.conn != nil {
		t.conn.Close()
	}
}
