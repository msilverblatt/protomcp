package envelope_test

import (
	"bytes"
	"strings"
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

func TestReadRaw(t *testing.T) {
	var buf bytes.Buffer

	// Write a RawHeader envelope
	payload := []byte(strings.Repeat("X", 100000))
	header := &pb.Envelope{
		Msg: &pb.Envelope_RawHeader{
			RawHeader: &pb.RawHeader{
				RequestId: "req-1",
				FieldName: "result_json",
				Size:      uint64(len(payload)),
			},
		},
	}
	if err := envelope.Write(&buf, header); err != nil {
		t.Fatal(err)
	}

	// Write raw bytes directly (no protobuf framing)
	buf.Write(payload)

	// Write a normal envelope after the raw bytes
	normal := &pb.Envelope{
		RequestId: "req-2",
		Msg: &pb.Envelope_CallResult{
			CallResult: &pb.CallToolResponse{ResultJson: `[{"type":"text","text":"ok"}]`},
		},
	}
	if err := envelope.Write(&buf, normal); err != nil {
		t.Fatal(err)
	}

	reader := &buf

	// ReadRaw should return the RawHeader envelope + the raw payload
	env, raw, err := envelope.ReadRaw(reader)
	if err != nil {
		t.Fatal(err)
	}
	rh := env.GetRawHeader()
	if rh == nil {
		t.Fatal("expected RawHeader")
	}
	if rh.RequestId != "req-1" {
		t.Errorf("request_id = %q, want req-1", rh.RequestId)
	}
	if len(raw) != len(payload) {
		t.Errorf("raw length = %d, want %d", len(raw), len(payload))
	}
	if string(raw) != string(payload) {
		t.Error("raw bytes don't match")
	}

	// Next read should return the normal envelope with no raw bytes
	env2, raw2, err := envelope.ReadRaw(reader)
	if err != nil {
		t.Fatal(err)
	}
	if env2.GetCallResult() == nil {
		t.Fatal("expected CallResult")
	}
	if raw2 != nil {
		t.Error("expected nil raw for non-RawHeader envelope")
	}
}
