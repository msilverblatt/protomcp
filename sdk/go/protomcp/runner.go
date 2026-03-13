package protomcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

var Log *ServerLogger

func Run() {
	socketPath := os.Getenv("PROTOMCP_SOCKET")
	if socketPath == "" {
		fmt.Fprintln(os.Stderr, "PROTOMCP_SOCKET not set — run via 'pmcp dev'")
		os.Exit(1)
	}

	tp := NewTransport(socketPath)
	if err := tp.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer tp.Close()

	Log = NewServerLogger(func(env *pb.Envelope) error { return tp.Send(env) }, "protomcp-go")

	for {
		env, err := tp.Recv()
		if err != nil {
			break
		}

		reqID := env.GetRequestId()

		switch {
		case env.GetListTools() != nil:
			handleListTools(tp, reqID)
			sendHandshakeComplete(tp)
		case env.GetCallTool() != nil:
			handleCallTool(tp, env.GetCallTool(), reqID)
		case env.GetReload() != nil:
			handleReload(tp, reqID)
		case env.GetMiddlewareIntercept() != nil:
			handleMiddlewareIntercept(tp, env.GetMiddlewareIntercept(), reqID)
		}
	}
}

func handleListTools(tp *Transport, reqID string) {
	tools := GetRegisteredTools()
	var defs []*pb.ToolDefinition
	for _, t := range tools {
		defs = append(defs, &pb.ToolDefinition{
			Name:            t.Name,
			Description:     t.Desc,
			InputSchemaJson: t.InputSchemaJSON(),
			DestructiveHint: t.Destructive,
			IdempotentHint:  t.Idempotent,
			ReadOnlyHint:    t.ReadOnly,
			OpenWorldHint:   t.OpenWorld,
			TaskSupport:     t.TaskSupport,
		})
	}
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ToolList{
			ToolList: &pb.ToolListResponse{Tools: defs},
		},
	})
}

func sendHandshakeComplete(tp *Transport) {
	tp.Send(&pb.Envelope{
		Msg: &pb.Envelope_ReloadResponse{
			ReloadResponse: &pb.ReloadResponse{Success: true},
		},
	})
}

func handleCallTool(tp *Transport, req *pb.CallToolRequest, reqID string) {
	tools := GetRegisteredTools()
	var handler func(ToolContext, map[string]interface{}) ToolResult
	for _, t := range tools {
		if t.Name == req.Name {
			handler = t.HandlerFn
			break
		}
	}

	if handler == nil {
		tp.Send(&pb.Envelope{
			RequestId: reqID,
			Msg: &pb.Envelope_CallResult{
				CallResult: &pb.CallToolResponse{
					IsError:    true,
					ResultJson: fmt.Sprintf(`[{"type":"text","text":"Tool not found: %s"}]`, req.Name),
				},
			},
		})
		return
	}

	var args map[string]interface{}
	if req.ArgumentsJson != "" {
		json.Unmarshal([]byte(req.ArgumentsJson), &args)
	}
	if args == nil {
		args = map[string]interface{}{}
	}

	ctx := ToolContext{
		Ctx:           context.Background(),
		ProgressToken: req.ProgressToken,
		sendFn:        func(env *pb.Envelope) error { return tp.Send(env) },
	}

	result := handler(ctx, args)

	resp := &pb.CallToolResponse{
		IsError:      result.IsError,
		ResultJson:   fmt.Sprintf(`[{"type":"text","text":"%s"}]`, result.ResultText),
		EnableTools:  result.EnableTools,
		DisableTools: result.DisableTools,
	}
	if result.IsError && result.ErrorCode != "" {
		resp.Error = &pb.ToolError{
			ErrorCode:  result.ErrorCode,
			Message:    result.Message,
			Suggestion: result.Suggestion,
			Retryable:  result.Retryable,
		}
	}

	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg:       &pb.Envelope_CallResult{CallResult: resp},
	})
}

func handleReload(tp *Transport, reqID string) {
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ReloadResponse{
			ReloadResponse: &pb.ReloadResponse{Success: true},
		},
	})
	handleListTools(tp, "")
	sendHandshakeComplete(tp)
}

func handleMiddlewareIntercept(tp *Transport, req *pb.MiddlewareInterceptRequest, reqID string) {
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_MiddlewareInterceptResponse{
			MiddlewareInterceptResponse: &pb.MiddlewareInterceptResponse{
				ArgumentsJson: req.ArgumentsJson,
				ResultJson:    req.ResultJson,
			},
		},
	})
}
