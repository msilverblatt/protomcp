package protomcp

import (
	"context"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

type ToolContext struct {
	Ctx           context.Context
	ProgressToken string
	sendFn        func(*pb.Envelope) error
}

func NewToolContext(ctx context.Context, progressToken string, sendFn func(*pb.Envelope) error) *ToolContext {
	return &ToolContext{Ctx: ctx, ProgressToken: progressToken, sendFn: sendFn}
}

func (tc *ToolContext) ReportProgress(progress, total int64, message string) error {
	env := &pb.Envelope{
		Msg: &pb.Envelope_Progress{
			Progress: &pb.ProgressNotification{
				ProgressToken: tc.ProgressToken,
				Progress:      progress,
				Total:         total,
				Message:       message,
			},
		},
	}
	return tc.sendFn(env)
}

func (tc *ToolContext) IsCancelled() bool {
	return tc.Ctx.Err() != nil
}
