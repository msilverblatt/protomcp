package progress

import (
	"testing"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestProgressProxy_ForwardsToMCP(t *testing.T) {
	var sent []map[string]any
	proxy := NewProxy(func(notification map[string]any) {
		sent = append(sent, notification)
	})
	proxy.HandleProgress(&pb.ProgressNotification{
		ProgressToken: "tok-1",
		Progress:      5,
		Total:         10,
		Message:       "Processing item 5",
	})
	if len(sent) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(sent))
	}
	if sent[0]["method"] != "notifications/progress" {
		t.Fatalf("expected notifications/progress, got %v", sent[0]["method"])
	}
}

func TestProgressProxy_DropsWhenNoToken(t *testing.T) {
	var sent []map[string]any
	proxy := NewProxy(func(notification map[string]any) {
		sent = append(sent, notification)
	})
	proxy.HandleProgress(&pb.ProgressNotification{
		ProgressToken: "",
		Progress:      1,
	})
	if len(sent) != 0 {
		t.Fatalf("expected 0 notifications for empty token, got %d", len(sent))
	}
}
