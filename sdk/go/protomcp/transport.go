package protomcp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

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
