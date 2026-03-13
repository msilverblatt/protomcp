package protomcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

var Log *ServerLogger

// mwHandlers maps middleware name to handler during runtime.
var mwHandlers map[string]func(phase, toolName, argsJSON, resultJSON string, isError bool) map[string]interface{}

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
	mwHandlers = make(map[string]func(phase, toolName, argsJSON, resultJSON string, isError bool) map[string]interface{})

	for {
		env, err := tp.Recv()
		if err != nil {
			break
		}

		reqID := env.GetRequestId()

		switch {
		case env.GetListTools() != nil:
			handleListTools(tp, reqID)
			sendMiddlewareRegistrations(tp)
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

func sendMiddlewareRegistrations(tp *Transport) {
	mws := GetRegisteredMiddleware()
	for _, mw := range mws {
		mwHandlers[mw.Name] = mw.Handler
		tp.Send(&pb.Envelope{
			Msg: &pb.Envelope_RegisterMiddleware{
				RegisterMiddleware: &pb.RegisterMiddlewareRequest{
					Name:     mw.Name,
					Priority: mw.Priority,
				},
			},
		})
		// Wait for acknowledgment
		if _, err := tp.Recv(); err != nil {
			return
		}
	}
	sendHandshakeComplete(tp)
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
	sendMiddlewareRegistrations(tp)
}

func handleMiddlewareIntercept(tp *Transport, req *pb.MiddlewareInterceptRequest, reqID string) {
	resp := &pb.MiddlewareInterceptResponse{
		ArgumentsJson: req.ArgumentsJson,
		ResultJson:    req.ResultJson,
	}

	handler, ok := mwHandlers[req.MiddlewareName]
	if ok && handler != nil {
		result := handler(req.Phase, req.ToolName, req.ArgumentsJson, req.ResultJson, req.IsError)
		if result != nil {
			if v, ok := result["reject"].(bool); ok && v {
				resp.Reject = true
				if r, ok := result["reject_reason"].(string); ok {
					resp.RejectReason = r
				}
			}
			if v, ok := result["arguments_json"].(string); ok {
				resp.ArgumentsJson = v
			}
			if v, ok := result["result_json"].(string); ok {
				resp.ResultJson = v
			}
		}
	}

	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg:       &pb.Envelope_MiddlewareInterceptResponse{MiddlewareInterceptResponse: resp},
	})
}
