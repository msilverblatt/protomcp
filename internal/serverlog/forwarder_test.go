package serverlog

import (
	"testing"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestForwarder_ForwardsAboveLevel(t *testing.T) {
	var sent []map[string]any
	fwd := NewForwarder("info", func(n map[string]any) { sent = append(sent, n) })
	fwd.HandleLog(&pb.LogMessage{Level: "warning", DataJson: `{"msg":"rate limit"}`})
	if len(sent) != 1 {
		t.Fatalf("expected 1, got %d", len(sent))
	}
	if sent[0]["method"] != "notifications/message" {
		t.Fatalf("wrong method: %v", sent[0]["method"])
	}
}

func TestForwarder_FiltersBelowLevel(t *testing.T) {
	var sent []map[string]any
	fwd := NewForwarder("warning", func(n map[string]any) { sent = append(sent, n) })
	fwd.HandleLog(&pb.LogMessage{Level: "info", DataJson: `{"msg":"hello"}`})
	if len(sent) != 0 {
		t.Fatalf("expected 0 (filtered), got %d", len(sent))
	}
}
