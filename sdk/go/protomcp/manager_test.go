package protomcp

import (
	"testing"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

func TestToolManager_Enable(t *testing.T) {
	var sent *pb.Envelope
	mgr := newToolManager(func(env *pb.Envelope) error {
		sent = env
		return nil
	})

	mgr.Enable("debug_dump", "trace_calls")

	if sent == nil {
		t.Fatal("expected envelope")
	}
	req := sent.GetEnableTools()
	if req == nil {
		t.Fatal("expected EnableToolsRequest")
	}
	if len(req.ToolNames) != 2 {
		t.Fatalf("expected 2 tool names, got %d", len(req.ToolNames))
	}
	if req.ToolNames[0] != "debug_dump" {
		t.Fatalf("expected debug_dump, got %s", req.ToolNames[0])
	}
}

func TestToolManager_Disable(t *testing.T) {
	var sent *pb.Envelope
	mgr := newToolManager(func(env *pb.Envelope) error {
		sent = env
		return nil
	})

	mgr.Disable("admin_panel")

	if sent == nil {
		t.Fatal("expected envelope")
	}
	req := sent.GetDisableTools()
	if req == nil {
		t.Fatal("expected DisableToolsRequest")
	}
	if len(req.ToolNames) != 1 || req.ToolNames[0] != "admin_panel" {
		t.Fatalf("expected [admin_panel], got %v", req.ToolNames)
	}
}
