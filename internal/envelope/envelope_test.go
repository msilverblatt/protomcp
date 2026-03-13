package envelope_test

import (
	"bytes"
	"testing"

	"github.com/msilverblatt/protomcp/internal/envelope"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestWriteAndReadEnvelope(t *testing.T) {
	env := &pb.Envelope{
		Msg: &pb.Envelope_ListTools{
			ListTools: &pb.ListToolsRequest{},
		},
		RequestId: "test-123",
	}

	var buf bytes.Buffer
	err := envelope.Write(&buf, env)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := envelope.Read(&buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if got.RequestId != "test-123" {
		t.Errorf("RequestId = %q, want %q", got.RequestId, "test-123")
	}

	if got.GetListTools() == nil {
		t.Error("expected ListToolsRequest, got nil")
	}
}

func TestWriteAndReadCallTool(t *testing.T) {
	env := &pb.Envelope{
		Msg: &pb.Envelope_CallTool{
			CallTool: &pb.CallToolRequest{
				Name:          "search",
				ArgumentsJson: `{"query": "hello"}`,
			},
		},
		RequestId: "call-456",
	}

	var buf bytes.Buffer
	err := envelope.Write(&buf, env)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	got, err := envelope.Read(&buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	ct := got.GetCallTool()
	if ct == nil {
		t.Fatal("expected CallToolRequest, got nil")
	}
	if ct.Name != "search" {
		t.Errorf("Name = %q, want %q", ct.Name, "search")
	}
	if ct.ArgumentsJson != `{"query": "hello"}` {
		t.Errorf("ArgumentsJson = %q, want %q", ct.ArgumentsJson, `{"query": "hello"}`)
	}
}

func TestReadEmptyBuffer(t *testing.T) {
	var buf bytes.Buffer
	_, err := envelope.Read(&buf)
	if err == nil {
		t.Error("expected error reading empty buffer, got nil")
	}
}

func TestMultipleEnvelopes(t *testing.T) {
	var buf bytes.Buffer

	envs := []*pb.Envelope{
		{Msg: &pb.Envelope_ListTools{ListTools: &pb.ListToolsRequest{}}, RequestId: "1"},
		{Msg: &pb.Envelope_Reload{Reload: &pb.ReloadRequest{}}, RequestId: "2"},
		{Msg: &pb.Envelope_ListTools{ListTools: &pb.ListToolsRequest{}}, RequestId: "3"},
	}

	for _, env := range envs {
		if err := envelope.Write(&buf, env); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	for i, want := range envs {
		got, err := envelope.Read(&buf)
		if err != nil {
			t.Fatalf("Read %d failed: %v", i, err)
		}
		if got.RequestId != want.RequestId {
			t.Errorf("envelope %d: RequestId = %q, want %q", i, got.RequestId, want.RequestId)
		}
	}
}
