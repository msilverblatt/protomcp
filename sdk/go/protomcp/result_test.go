package protomcp_test

import (
	"testing"

	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func TestToolResultBasic(t *testing.T) {
	r := protomcp.Result("hello")
	if r.ResultText != "hello" {
		t.Errorf("ResultText = %q, want %q", r.ResultText, "hello")
	}
	if r.IsError {
		t.Error("should not be error")
	}
}

func TestToolResultError(t *testing.T) {
	r := protomcp.ErrorResult("failed", "INVALID", "try again", true)
	if !r.IsError {
		t.Error("should be error")
	}
	if r.ErrorCode != "INVALID" {
		t.Errorf("ErrorCode = %q, want %q", r.ErrorCode, "INVALID")
	}
}

func TestToolResultEnableDisable(t *testing.T) {
	r := protomcp.Result("ok")
	r.EnableTools = []string{"admin_panel"}
	r.DisableTools = []string{"login"}
	if len(r.EnableTools) != 1 {
		t.Error("expected 1 enable tool")
	}
}
