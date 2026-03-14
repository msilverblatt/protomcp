package protomcp

type ToolResult struct {
	ResultText   string
	IsError      bool
	ErrorCode    string
	Message      string
	Suggestion   string
	Retryable    bool
	EnableTools  []string
	DisableTools []string
}

func Result(text string) ToolResult {
	return ToolResult{ResultText: text}
}

func ErrorResult(text, errorCode, suggestion string, retryable bool) ToolResult {
	return ToolResult{
		ResultText: text,
		IsError:    true,
		ErrorCode:  errorCode,
		Message:    text,
		Suggestion: suggestion,
		Retryable:  retryable,
	}
}
