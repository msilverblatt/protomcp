package protomcp

import (
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

type ToolManager struct {
	sendFn func(*pb.Envelope) error
}

func newToolManager(sendFn func(*pb.Envelope) error) *ToolManager {
	return &ToolManager{sendFn: sendFn}
}

func (m *ToolManager) Enable(names ...string) {
	m.sendFn(&pb.Envelope{
		Msg: &pb.Envelope_EnableTools{
			EnableTools: &pb.EnableToolsRequest{ToolNames: names},
		},
	})
}

func (m *ToolManager) Disable(names ...string) {
	m.sendFn(&pb.Envelope{
		Msg: &pb.Envelope_DisableTools{
			DisableTools: &pb.DisableToolsRequest{ToolNames: names},
		},
	})
}
