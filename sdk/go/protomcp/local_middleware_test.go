package protomcp

import (
	"context"
	"strings"
	"testing"
)

func TestLocalMiddlewareChain(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	var callOrder []string

	LocalMiddleware(10, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		callOrder = append(callOrder, "mw10-before")
		result := next(ctx, args)
		callOrder = append(callOrder, "mw10-after")
		return result
	})

	LocalMiddleware(20, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		callOrder = append(callOrder, "mw20-before")
		result := next(ctx, args)
		callOrder = append(callOrder, "mw20-after")
		return result
	})

	handler := func(ctx ToolContext, args map[string]interface{}) ToolResult {
		callOrder = append(callOrder, "handler")
		return Result("done")
	}

	chain := BuildMiddlewareChain("test_tool", handler)
	ctx := ToolContext{Ctx: context.Background()}
	result := chain(ctx, map[string]interface{}{})

	if result.ResultText != "done" {
		t.Errorf("expected 'done', got '%s'", result.ResultText)
	}

	expected := "mw10-before,mw20-before,handler,mw20-after,mw10-after"
	got := strings.Join(callOrder, ",")
	if got != expected {
		t.Errorf("call order = %s, want %s", got, expected)
	}
}

func TestLocalMiddlewareCanModifyArgs(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	LocalMiddleware(10, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		args["injected"] = "yes"
		return next(ctx, args)
	})

	handler := func(ctx ToolContext, args map[string]interface{}) ToolResult {
		if args["injected"] != "yes" {
			return ErrorResult("missing injected", "", "", false)
		}
		return Result("ok")
	}

	chain := BuildMiddlewareChain("test_tool", handler)
	ctx := ToolContext{Ctx: context.Background()}
	result := chain(ctx, map[string]interface{}{})

	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
}

func TestLocalMiddlewareCanShortCircuit(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	LocalMiddleware(10, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		return ErrorResult("blocked", "BLOCKED", "", false)
	})

	handlerCalled := false
	handler := func(ctx ToolContext, args map[string]interface{}) ToolResult {
		handlerCalled = true
		return Result("ok")
	}

	chain := BuildMiddlewareChain("test_tool", handler)
	ctx := ToolContext{Ctx: context.Background()}
	result := chain(ctx, map[string]interface{}{})

	if !result.IsError {
		t.Error("expected error from short circuit")
	}
	if handlerCalled {
		t.Error("handler should not have been called")
	}
}

func TestLocalMiddlewareNoMiddleware(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	handler := func(ctx ToolContext, args map[string]interface{}) ToolResult {
		return Result("direct")
	}

	chain := BuildMiddlewareChain("test_tool", handler)
	ctx := ToolContext{Ctx: context.Background()}
	result := chain(ctx, map[string]interface{}{})

	if result.ResultText != "direct" {
		t.Errorf("expected 'direct', got '%s'", result.ResultText)
	}
}

func TestLocalMiddlewareToolNamePassed(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	var receivedToolName string
	LocalMiddleware(10, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		receivedToolName = toolName
		return next(ctx, args)
	})

	handler := func(ctx ToolContext, args map[string]interface{}) ToolResult {
		return Result("ok")
	}

	chain := BuildMiddlewareChain("my_tool", handler)
	ctx := ToolContext{Ctx: context.Background()}
	chain(ctx, map[string]interface{}{})

	if receivedToolName != "my_tool" {
		t.Errorf("expected tool name 'my_tool', got '%s'", receivedToolName)
	}
}

func TestGetLocalMiddlewareSortedByPriority(t *testing.T) {
	ClearLocalMiddleware()
	defer ClearLocalMiddleware()

	LocalMiddleware(30, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		return next(ctx, args)
	})
	LocalMiddleware(10, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		return next(ctx, args)
	})
	LocalMiddleware(20, func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult {
		return next(ctx, args)
	})

	mws := GetLocalMiddleware()
	if len(mws) != 3 {
		t.Fatalf("expected 3, got %d", len(mws))
	}
	if mws[0].Priority != 10 || mws[1].Priority != 20 || mws[2].Priority != 30 {
		t.Errorf("not sorted: %d, %d, %d", mws[0].Priority, mws[1].Priority, mws[2].Priority)
	}
}
