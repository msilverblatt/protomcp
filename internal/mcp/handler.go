package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/msilverblatt/protomcp/internal/cancel"
	"github.com/msilverblatt/protomcp/internal/tasks"
	pb "github.com/msilverblatt/protomcp/gen/proto/protomcp"
)

// ToolBackend is the interface the handler uses to communicate with the tool process.
type ToolBackend interface {
	ActiveTools() []*pb.ToolDefinition
	CallTool(ctx context.Context, name, argsJSON string) (*pb.CallToolResponse, error)
}

// ToolListMutationHandler is called when a tool call returns enable_tools/disable_tools.
type ToolListMutationHandler func(enable, disable []string)

type Handler struct {
	backend            ToolBackend
	onToolListMutation ToolListMutationHandler
	cancelTracker      *cancel.Tracker
	taskManager        *tasks.Manager
}

func NewHandler(backend ToolBackend) *Handler {
	return &Handler{
		backend:       backend,
		cancelTracker: cancel.NewTracker(),
		taskManager:   tasks.NewManager(),
	}
}

func (h *Handler) SetToolListMutationHandler(fn ToolListMutationHandler) {
	h.onToolListMutation = fn
}

func (h *Handler) CancelTracker() *cancel.Tracker {
	return h.cancelTracker
}

func (h *Handler) TaskManager() *tasks.Manager {
	return h.taskManager
}

func (h *Handler) Handle(ctx context.Context, req JSONRPCRequest) (*JSONRPCResponse, error) {
	switch req.Method {
	case "initialize":
		return h.handleInitialize(req)
	case "notifications/initialized":
		return nil, nil // No response for notifications
	case "notifications/cancelled":
		return h.handleCancelled(req)
	case "tools/list":
		return h.handleToolsList(req)
	case "tools/call":
		return h.handleToolsCall(ctx, req)
	case "tasks/get":
		return h.handleTasksGet(req)
	case "tasks/cancel":
		return h.handleTasksCancel(req)
	default:
		return h.methodNotFound(req)
	}
}

func (h *Handler) handleInitialize(req JSONRPCRequest) (*JSONRPCResponse, error) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: Capabilities{
			Tools: &ToolsCapability{ListChanged: true},
		},
		ServerInfo: ServerInfo{
			Name:    "protomcp",
			Version: "1.0.0",
		},
	}
	return h.success(req.ID, result)
}

func (h *Handler) handleCancelled(req JSONRPCRequest) (*JSONRPCResponse, error) {
	var params CancelledParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return h.invalidParams(req.ID, fmt.Sprintf("invalid params: %v", err))
		}
	}
	if params.RequestID != "" {
		h.cancelTracker.Cancel(params.RequestID)
	}
	return nil, nil // notifications don't get responses
}

func (h *Handler) handleToolsList(req JSONRPCRequest) (*JSONRPCResponse, error) {
	tools := h.backend.ActiveTools()
	mcpTools := make([]MCPTool, 0, len(tools))
	for _, t := range tools {
		tool := MCPTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: json.RawMessage(t.InputSchemaJson),
		}
		if t.OutputSchemaJson != "" {
			tool.OutputSchema = json.RawMessage(t.OutputSchemaJson)
		}
		annotations := &MCPToolAnnotations{}
		hasAnnotation := false
		if t.Title != "" {
			annotations.Title = t.Title
			hasAnnotation = true
		}
		if t.ReadOnlyHint {
			annotations.ReadOnlyHint = t.ReadOnlyHint
			hasAnnotation = true
		}
		if t.DestructiveHint {
			annotations.DestructiveHint = t.DestructiveHint
			hasAnnotation = true
		}
		if t.IdempotentHint {
			annotations.IdempotentHint = t.IdempotentHint
			hasAnnotation = true
		}
		if t.OpenWorldHint {
			annotations.OpenWorldHint = t.OpenWorldHint
			hasAnnotation = true
		}
		if hasAnnotation {
			tool.Annotations = annotations
		}
		mcpTools = append(mcpTools, tool)
	}
	return h.success(req.ID, ToolsListResult{Tools: mcpTools})
}

func (h *Handler) handleToolsCall(ctx context.Context, req JSONRPCRequest) (*JSONRPCResponse, error) {
	var params ToolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return h.invalidParams(req.ID, fmt.Sprintf("invalid params: %v", err))
	}

	argsJSON := "{}"
	if params.Arguments != nil {
		argsJSON = string(params.Arguments)
	}

	// Track the request for cancellation if we have a request ID
	reqIDStr := string(req.ID)
	if reqIDStr != "" {
		h.cancelTracker.TrackCall(reqIDStr)
		defer h.cancelTracker.Complete(reqIDStr)
	}

	resp, err := h.backend.CallTool(ctx, params.Name, argsJSON)
	if err != nil {
		return h.internalError(req.ID, err.Error())
	}

	// Handle tool list mutations
	if h.onToolListMutation != nil && (len(resp.EnableTools) > 0 || len(resp.DisableTools) > 0) {
		h.onToolListMutation(resp.EnableTools, resp.DisableTools)
	}

	// Parse the result_json as content array
	var content []ContentItem
	if resp.ResultJson != "" {
		if err := json.Unmarshal([]byte(resp.ResultJson), &content); err != nil {
			// If it's not a content array, wrap it as text
			content = []ContentItem{{Type: "text", Text: resp.ResultJson}}
		}
	}

	result := ToolsCallResult{
		Content: content,
		IsError: resp.IsError,
	}

	if resp.StructuredContentJson != "" {
		result.StructuredContent = json.RawMessage(resp.StructuredContentJson)
	}

	return h.success(req.ID, result)
}

func (h *Handler) handleTasksGet(req JSONRPCRequest) (*JSONRPCResponse, error) {
	var params TasksGetParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return h.invalidParams(req.ID, fmt.Sprintf("invalid params: %v", err))
		}
	}
	state, err := h.taskManager.GetStatus(params.TaskID)
	if err != nil {
		return h.invalidParams(req.ID, err.Error())
	}
	return h.success(req.ID, TasksGetResult{
		TaskID:  params.TaskID,
		State:   state.State,
		Message: state.Message,
	})
}

func (h *Handler) handleTasksCancel(req JSONRPCRequest) (*JSONRPCResponse, error) {
	var params TasksCancelParams
	if req.Params != nil {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return h.invalidParams(req.ID, fmt.Sprintf("invalid params: %v", err))
		}
	}
	if err := h.taskManager.UpdateStatus(params.TaskID, "cancelled", "cancelled by client"); err != nil {
		return h.invalidParams(req.ID, err.Error())
	}
	return h.success(req.ID, map[string]any{})
}

func (h *Handler) success(id json.RawMessage, result interface{}) (*JSONRPCResponse, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  data,
	}, nil
}

func (h *Handler) methodNotFound(req JSONRPCRequest) (*JSONRPCResponse, error) {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &JSONRPCError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
	}, nil
}

func (h *Handler) invalidParams(id json.RawMessage, msg string) (*JSONRPCResponse, error) {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: -32602, Message: msg},
	}, nil
}

func (h *Handler) internalError(id json.RawMessage, msg string) (*JSONRPCResponse, error) {
	return &JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: -32603, Message: msg},
	}, nil
}

// ListChangedNotification creates a tools/list_changed notification.
func ListChangedNotification() JSONRPCNotification {
	return JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/tools/list_changed",
	}
}
