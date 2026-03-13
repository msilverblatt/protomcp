package protomcp_test

import (
	"context"
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func TestToolContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tc := protomcp.NewToolContext(ctx, "tok", func(env *pb.Envelope) error { return nil })
	if tc.IsCancelled() {
		t.Error("should not be cancelled yet")
	}
	cancel()
	if !tc.IsCancelled() {
		t.Error("should be cancelled")
	}
}
