package protomcp_test

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func TestTransportConnectAndClose(t *testing.T) {
	dir := t.TempDir()
	sockPath := filepath.Join(dir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	os.Setenv("PROTOMCP_SOCKET", sockPath)
	defer os.Unsetenv("PROTOMCP_SOCKET")

	tp := protomcp.NewTransport(sockPath)

	// Accept in background
	go func() { listener.Accept() }()

	if err := tp.Connect(); err != nil {
		t.Fatal(err)
	}
	tp.Close()
}
