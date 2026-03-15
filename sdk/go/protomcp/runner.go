package protomcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

var firstToolCallOnce sync.Once

var Log *ServerLogger

// mwHandlers maps middleware name to handler during runtime.
var mwHandlers map[string]func(phase, toolName, argsJSON, resultJSON string, isError bool) map[string]interface{}

// activeCancels maps request_id to cancel function for in-flight tool calls.
var activeCancels map[string]context.CancelFunc
var cancelsMu sync.Mutex

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
	activeCancels = make(map[string]context.CancelFunc)

	StartSidecars("server_start")
	defer StopAllSidecars()

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
			sendDisableHiddenTools(tp)
		case env.GetCallTool() != nil:
			handleCallTool(tp, env.GetCallTool(), reqID)
		case env.GetReload() != nil:
			handleReload(tp, reqID)
		case env.GetMiddlewareIntercept() != nil:
			handleMiddlewareIntercept(tp, env.GetMiddlewareIntercept(), reqID)
		case env.GetListResourcesRequest() != nil:
			handleListResources(tp, reqID)
		case env.GetListResourceTemplatesRequest() != nil:
			handleListResourceTemplates(tp, reqID)
		case env.GetReadResourceRequest() != nil:
			handleReadResource(tp, env.GetReadResourceRequest(), reqID)
		case env.GetListPromptsRequest() != nil:
			handleListPrompts(tp, reqID)
		case env.GetGetPromptRequest() != nil:
			handleGetPrompt(tp, env.GetGetPromptRequest(), reqID)
		case env.GetCompletionRequest() != nil:
			handleCompletion(tp, env.GetCompletionRequest(), reqID)
		case env.GetCancel() != nil:
			cancelsMu.Lock()
			if cancel, ok := activeCancels[env.GetCancel().GetRequestId()]; ok {
				cancel()
			}
			cancelsMu.Unlock()
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

func sendDisableHiddenTools(tp *Transport) {
	var hidden []string
	for _, t := range GetRegisteredTools() {
		if t.Hidden {
			hidden = append(hidden, t.Name)
		}
	}
	if len(hidden) == 0 {
		return
	}
	tp.Send(&pb.Envelope{
		Msg: &pb.Envelope_DisableTools{
			DisableTools: &pb.DisableToolsRequest{ToolNames: hidden},
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

func buildResultJSON(text string) string {
	content := []map[string]string{{"type": "text", "text": text}}
	data, err := json.Marshal(content)
	if err != nil {
		return `[{"type":"text","text":""}]`
	}
	return string(data)
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
					ResultJson: buildResultJSON("Tool not found: " + req.Name),
				},
			},
		})
		return
	}

	var args map[string]interface{}
	if req.ArgumentsJson != "" {
		if err := json.Unmarshal([]byte(req.ArgumentsJson), &args); err != nil {
			tp.Send(&pb.Envelope{
				RequestId: reqID,
				Msg: &pb.Envelope_CallResult{
					CallResult: &pb.CallToolResponse{
						IsError:    true,
						ResultJson: buildResultJSON("Invalid arguments JSON: " + err.Error()),
						Error: &pb.ToolError{
							ErrorCode:  "INVALID_INPUT",
							Message:    fmt.Sprintf("Failed to parse arguments: %s", err.Error()),
							Suggestion: "Ensure arguments is valid JSON",
							Retryable:  false,
						},
					},
				},
			})
			return
		}
	}
	if args == nil {
		args = map[string]interface{}{}
	}

	// Start first_tool_call sidecars on first invocation
	firstToolCallOnce.Do(func() { StartSidecars("first_tool_call") })

	// Resolve server contexts
	ctxValues := ResolveContexts(args)
	for k, v := range ctxValues {
		args[k] = v
	}

	callCtx, cancel := context.WithCancel(context.Background())
	cancelsMu.Lock()
	activeCancels[reqID] = cancel
	cancelsMu.Unlock()

	ctx := ToolContext{
		Ctx:           callCtx,
		ProgressToken: req.ProgressToken,
		sendFn:        func(env *pb.Envelope) error { return tp.Send(env) },
	}

	// Emit telemetry start
	EmitTelemetry(ToolCallEvent{
		ToolName: req.Name,
		Phase:    "start",
		Args:     args,
	})

	startTime := time.Now()

	// Build local middleware chain
	wrapped := BuildMiddlewareChain(req.Name, handler)
	result := wrapped(ctx, args)

	durationMs := int(time.Since(startTime).Milliseconds())

	// Emit telemetry success/error
	if result.IsError {
		EmitTelemetry(ToolCallEvent{
			ToolName:   req.Name,
			Phase:      "error",
			Args:       args,
			Result:     result.ResultText,
			DurationMs: durationMs,
			Message:    result.Message,
		})
	} else {
		EmitTelemetry(ToolCallEvent{
			ToolName:   req.Name,
			Phase:      "success",
			Args:       args,
			Result:     result.ResultText,
			DurationMs: durationMs,
		})
	}

	cancel()
	cancelsMu.Lock()
	delete(activeCancels, reqID)
	cancelsMu.Unlock()

	resp := &pb.CallToolResponse{
		IsError:      result.IsError,
		ResultJson:   buildResultJSON(result.ResultText),
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

	// Use raw sideband for large payloads to avoid protobuf overhead.
	chunkThreshold := 65536
	if v := os.Getenv("PROTOMCP_CHUNK_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			chunkThreshold = n
		}
	}

	resultBytes := []byte(resp.ResultJson)
	if len(resultBytes) > chunkThreshold {
		tp.SendRaw(reqID, "result_json", resultBytes)
	} else {
		tp.Send(&pb.Envelope{
			RequestId: reqID,
			Msg:       &pb.Envelope_CallResult{CallResult: resp},
		})
	}
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

func uriMatchesTemplate(template, uri string) bool {
	parts := strings.Split(template, "{")
	if len(parts) == 0 {
		return template == uri
	}
	if !strings.HasPrefix(uri, parts[0]) {
		return false
	}
	remaining := uri[len(parts[0]):]
	for _, part := range parts[1:] {
		closeBrace := strings.Index(part, "}")
		if closeBrace < 0 {
			return false
		}
		suffix := part[closeBrace+1:]
		if suffix == "" {
			if remaining == "" {
				return false
			}
			remaining = ""
		} else {
			idx := strings.Index(remaining, suffix)
			if idx <= 0 {
				return false
			}
			remaining = remaining[idx+len(suffix):]
		}
	}
	return remaining == ""
}

func handleListResources(tp *Transport, reqID string) {
	resources := GetRegisteredResources()
	var defs []*pb.ResourceDefinition
	for _, r := range resources {
		defs = append(defs, &pb.ResourceDefinition{
			Uri:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
			Size:        r.Size,
		})
	}
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ResourceListResponse{
			ResourceListResponse: &pb.ResourceListResponse{Resources: defs},
		},
	})
}

func handleListResourceTemplates(tp *Transport, reqID string) {
	templates := GetRegisteredResourceTemplates()
	var defs []*pb.ResourceTemplateDefinition
	for _, t := range templates {
		defs = append(defs, &pb.ResourceTemplateDefinition{
			UriTemplate: t.URITemplate,
			Name:        t.Name,
			Description: t.Description,
			MimeType:    t.MimeType,
		})
	}
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ResourceTemplateListResponse{
			ResourceTemplateListResponse: &pb.ResourceTemplateListResponse{Templates: defs},
		},
	})
}

func handleReadResource(tp *Transport, req *pb.ReadResourceRequest, reqID string) {
	uri := req.GetUri()

	// Try static resources first.
	for _, r := range GetRegisteredResources() {
		if r.URI == uri && r.HandlerFn != nil {
			contents := r.HandlerFn()
			sendResourceContents(tp, reqID, contents)
			return
		}
	}

	// Try resource templates.
	for _, t := range GetRegisteredResourceTemplates() {
		if t.HandlerFn != nil && uriMatchesTemplate(t.URITemplate, uri) {
			contents := t.HandlerFn(uri)
			if len(contents) > 0 {
				sendResourceContents(tp, reqID, contents)
				return
			}
		}
	}

	// No matching resource found.
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ReadResourceResponse{
			ReadResourceResponse: &pb.ReadResourceResponse{},
		},
	})
}

func sendResourceContents(tp *Transport, reqID string, contents []ResourceContent) {
	var pbContents []*pb.ResourceContent
	for _, c := range contents {
		pbContents = append(pbContents, &pb.ResourceContent{
			Uri:      c.URI,
			MimeType: c.MimeType,
			Text:     c.Text,
			Blob:     c.Blob,
		})
	}
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_ReadResourceResponse{
			ReadResourceResponse: &pb.ReadResourceResponse{Contents: pbContents},
		},
	})
}

func handleListPrompts(tp *Transport, reqID string) {
	prompts := GetRegisteredPrompts()
	var defs []*pb.PromptDefinition
	for _, p := range prompts {
		var args []*pb.PromptArgument
		for _, a := range p.Arguments {
			args = append(args, &pb.PromptArgument{
				Name:        a.Name,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		defs = append(defs, &pb.PromptDefinition{
			Name:        p.Name,
			Description: p.Description,
			Arguments:   args,
		})
	}
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_PromptListResponse{
			PromptListResponse: &pb.PromptListResponse{Prompts: defs},
		},
	})
}

func handleGetPrompt(tp *Transport, req *pb.GetPromptRequest, reqID string) {
	prompts := GetRegisteredPrompts()
	for _, p := range prompts {
		if p.Name == req.GetName() && p.HandlerFn != nil {
			var args map[string]string
			if req.GetArgumentsJson() != "" {
				if err := json.Unmarshal([]byte(req.GetArgumentsJson()), &args); err != nil {
					args = map[string]string{}
				}
			}
			if args == nil {
				args = map[string]string{}
			}
			desc, messages := p.HandlerFn(args)
			var pbMsgs []*pb.PromptMessage
			for _, m := range messages {
				pbMsgs = append(pbMsgs, &pb.PromptMessage{
					Role:        m.Role,
					ContentJson: m.ContentJSON,
				})
			}
			tp.Send(&pb.Envelope{
				RequestId: reqID,
				Msg: &pb.Envelope_GetPromptResponse{
					GetPromptResponse: &pb.GetPromptResponse{
						Description: desc,
						Messages:    pbMsgs,
					},
				},
			})
			return
		}
	}
	// Prompt not found — return empty response.
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_GetPromptResponse{
			GetPromptResponse: &pb.GetPromptResponse{},
		},
	})
}

func handleCompletion(tp *Transport, req *pb.CompletionRequest, reqID string) {
	handler, ok := GetCompletionHandler(req.GetRefType(), req.GetRefName(), req.GetArgumentName())
	if !ok || handler == nil {
		tp.Send(&pb.Envelope{
			RequestId: reqID,
			Msg: &pb.Envelope_CompletionResponse{
				CompletionResponse: &pb.CompletionResponse{},
			},
		})
		return
	}
	result := handler(req.GetArgumentValue())
	tp.Send(&pb.Envelope{
		RequestId: reqID,
		Msg: &pb.Envelope_CompletionResponse{
			CompletionResponse: &pb.CompletionResponse{
				Values:  result.Values,
				Total:   result.Total,
				HasMore: result.HasMore,
			},
		},
	})
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
