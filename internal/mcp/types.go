package mcp

import "encoding/json"

// JSON-RPC 2.0
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// MCP Initialize
type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCP Tools
type ToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

type MCPToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    bool   `json:"readOnlyHint,omitempty"`
	DestructiveHint bool   `json:"destructiveHint,omitempty"`
	IdempotentHint  bool   `json:"idempotentHint,omitempty"`
	OpenWorldHint   bool   `json:"openWorldHint,omitempty"`
}

type MCPTool struct {
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	InputSchema  json.RawMessage     `json:"inputSchema"`
	OutputSchema json.RawMessage     `json:"outputSchema,omitempty"`
	Annotations  *MCPToolAnnotations `json:"annotations,omitempty"`
}

type ToolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      *ToolsCallMeta  `json:"_meta,omitempty"`
}

type ToolsCallMeta struct {
	ProgressToken string `json:"progressToken,omitempty"`
}

type ToolsCallResult struct {
	Content          []ContentItem   `json:"content"`
	IsError          bool            `json:"isError,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Tasks
type TasksGetParams struct {
	TaskID string `json:"taskId"`
}

type TasksGetResult struct {
	TaskID  string `json:"taskId"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type TasksCancelParams struct {
	TaskID string `json:"taskId"`
}

// Cancellation notification params
type CancelledParams struct {
	RequestID string `json:"requestId"`
	Reason    string `json:"reason,omitempty"`
}
