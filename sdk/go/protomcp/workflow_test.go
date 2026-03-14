package protomcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func workflowTestCtx() ToolContext {
	return ToolContext{Ctx: context.Background()}
}

func setupWorkflowTest(t *testing.T) {
	t.Helper()
	ClearWorkflowRegistry()
	ClearRegistry()
	ClearGroupRegistry()
	t.Cleanup(func() {
		ClearWorkflowRegistry()
		ClearRegistry()
		ClearGroupRegistry()
		ToolManagerAdapter.GetActiveTools = nil
		ToolManagerAdapter.SetAllowed = nil
	})
}

// --- StepResult defaults ---

func TestStepResultDefaults(t *testing.T) {
	r := StepResult{}
	if r.Result != "" {
		t.Errorf("expected empty result, got '%s'", r.Result)
	}
	if r.Next != nil {
		t.Errorf("expected nil next, got %v", r.Next)
	}
}

func TestStepResultWithNext(t *testing.T) {
	r := StepResult{Result: "done", Next: []string{"approve", "reject"}}
	if r.Result != "done" {
		t.Errorf("expected 'done', got '%s'", r.Result)
	}
	if len(r.Next) != 2 || r.Next[0] != "approve" || r.Next[1] != "reject" {
		t.Errorf("unexpected next: %v", r.Next)
	}
}

// --- Workflow registration ---

func TestWorkflowRegistration(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("deploy",
		WorkflowDescription("Deploy workflow"),
		Step("start",
			StepDescription("Start deploy"),
			StepInitial(),
			StepNext("confirm"),
			StepArgs(StrArg("env")),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				env := args["env"].(string)
				return StepResult{Result: "Deploying to " + env}, nil
			}),
		),
		Step("confirm",
			StepDescription("Confirm deploy"),
			StepTerminal(),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "Confirmed"}, nil
			}),
		),
	)

	wfs := GetRegisteredWorkflows()
	if len(wfs) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(wfs))
	}
	if wfs[0].Name != "deploy" {
		t.Errorf("expected name 'deploy', got '%s'", wfs[0].Name)
	}
	if len(wfs[0].Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(wfs[0].Steps))
	}
	stepNames := map[string]bool{}
	for _, s := range wfs[0].Steps {
		stepNames[s.Name] = true
	}
	if !stepNames["start"] || !stepNames["confirm"] {
		t.Errorf("expected steps 'start' and 'confirm', got %v", stepNames)
	}
}

func TestInitialStepDetection(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("w1",
		WorkflowDescription("test"),
		Step("init",
			StepDescription("Init"),
			StepInitial(),
			StepNext("done"),
		),
		Step("done",
			StepDescription("Done"),
			StepTerminal(),
		),
	)

	wfs := GetRegisteredWorkflows()
	var initial []StepDef
	for _, s := range wfs[0].Steps {
		if s.Initial {
			initial = append(initial, s)
		}
	}
	if len(initial) != 1 {
		t.Fatalf("expected 1 initial step, got %d", len(initial))
	}
	if initial[0].Name != "init" {
		t.Errorf("expected initial step 'init', got '%s'", initial[0].Name)
	}
}

func TestOnCancelCapture(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("w3",
		WorkflowDescription("test"),
		Step("s", StepDescription("S"), StepInitial(), StepNext("e")),
		Step("e", StepDescription("E"), StepTerminal()),
		OnCancel(func(currentStep string, history []StepHistoryEntry) string {
			return "cancelled"
		}),
	)

	wfs := GetRegisteredWorkflows()
	if wfs[0].OnCancelFn == nil {
		t.Error("expected on_cancel to be set")
	}
}

func TestOnCompleteCapture(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("w4",
		WorkflowDescription("test"),
		Step("s", StepDescription("S"), StepInitial(), StepNext("e")),
		Step("e", StepDescription("E"), StepTerminal()),
		OnComplete(func(history []StepHistoryEntry) {}),
	)

	wfs := GetRegisteredWorkflows()
	if wfs[0].OnCompleteFn == nil {
		t.Error("expected on_complete to be set")
	}
}

// --- Graph validation errors ---

func TestNoInitialStep(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for no initial step")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "no initial step") {
			t.Errorf("expected 'no initial step' in panic, got '%s'", msg)
		}
	}()

	Workflow("bad1",
		Step("a", StepDescription("A"), StepNext("b")),
		Step("b", StepDescription("B"), StepTerminal()),
	)
}

func TestMultipleInitialSteps(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for multiple initial steps")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "multiple initial steps") {
			t.Errorf("expected 'multiple initial steps' in panic, got '%s'", msg)
		}
	}()

	Workflow("bad2",
		Step("a", StepDescription("A"), StepInitial(), StepNext("c")),
		Step("b", StepDescription("B"), StepInitial(), StepNext("c")),
		Step("c", StepDescription("C"), StepTerminal()),
	)
}

func TestMissingNextReference(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for missing next reference")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "nonexistent step") {
			t.Errorf("expected 'nonexistent step' in panic, got '%s'", msg)
		}
	}()

	Workflow("bad3",
		Step("a", StepDescription("A"), StepInitial(), StepNext("ghost")),
	)
}

func TestDeadEndStep(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for dead end step")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "no next") && !strings.Contains(msg, "dead end") {
			t.Errorf("expected 'no next' or 'dead end' in panic, got '%s'", msg)
		}
	}()

	Workflow("bad4",
		Step("a", StepDescription("A"), StepInitial(), StepNext("b")),
		Step("b", StepDescription("B")), // no next, not terminal
	)
}

func TestTerminalWithNext(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for terminal with next")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "terminal step") || !strings.Contains(msg, "has next") {
			t.Errorf("expected 'terminal step...has next' in panic, got '%s'", msg)
		}
	}()

	Workflow("bad5",
		Step("a", StepDescription("A"), StepInitial(), StepTerminal(), StepNext("b")),
		Step("b", StepDescription("B"), StepTerminal()),
	)
}

func TestOnErrorNonexistentTarget(t *testing.T) {
	setupWorkflowTest(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for on_error nonexistent target")
		}
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "on_error") && !strings.Contains(msg, "nonexistent") {
			t.Errorf("expected on_error nonexistent reference in panic, got '%s'", msg)
		}
	}()

	Workflow("bad6",
		Step("a", StepDescription("A"), StepInitial(), StepNext("b"),
			StepOnError(map[string]string{"bad value": "ghost"})),
		Step("b", StepDescription("B"), StepTerminal()),
	)
}

func TestValidGraphPasses(t *testing.T) {
	setupWorkflowTest(t)

	// Should not panic
	Workflow("good",
		WorkflowDescription("test"),
		Step("a", StepDescription("A"), StepInitial(), StepNext("b", "c")),
		Step("b", StepDescription("B"), StepNext("c")),
		Step("c", StepDescription("C"), StepTerminal()),
	)

	if len(GetRegisteredWorkflows()) != 1 {
		t.Error("expected 1 registered workflow")
	}
}

// --- Tool def generation ---

func TestToolDefGeneration(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("deploy",
		WorkflowDescription("Deploy"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("approve"),
			StepArgs(StrArg("env")),
		),
		Step("approve",
			StepDescription("Approve"),
			StepTerminal(),
		),
	)

	defs := WorkflowsToToolDefs()
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["deploy.start"] {
		t.Error("expected 'deploy.start' tool def")
	}
	if !names["deploy.approve"] {
		t.Error("expected 'deploy.approve' tool def")
	}
	if !names["deploy.cancel"] {
		t.Error("expected 'deploy.cancel' tool def")
	}

	// Initial step is not hidden
	for _, d := range defs {
		if d.Name == "deploy.start" && d.Hidden {
			t.Error("initial step should not be hidden")
		}
		if d.Name == "deploy.approve" && !d.Hidden {
			t.Error("non-initial step should be hidden")
		}
		if d.Name == "deploy.cancel" && !d.Hidden {
			t.Error("cancel should be hidden")
		}
	}
}

func TestNoCancelToolWhenAllNoCancel(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("strict",
		WorkflowDescription("Strict"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepNoCancel(),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
			StepNoCancel(),
		),
	)

	defs := WorkflowsToToolDefs()
	for _, d := range defs {
		if d.Name == "strict.cancel" {
			t.Error("should not have cancel tool when all steps have no_cancel")
		}
	}
}

// --- Step dispatch ---

func TestInitialStepDispatch(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{"existing_tool"} }
	var lastAllowed []string
	ToolManagerAdapter.SetAllowed = func(tools []string) { lastAllowed = tools }

	Workflow("d1",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepArgs(StrArg("msg")),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "Started: " + args["msg"].(string)}, nil
			}),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
	)

	ctx := workflowTestCtx()
	result := HandleStepCall("d1", "start", ctx, map[string]interface{}{"msg": "hello"})
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.ResultText)
	}
	if !strings.Contains(result.ResultText, "Started: hello") {
		t.Errorf("expected 'Started: hello' in result, got '%s'", result.ResultText)
	}

	stack := GetActiveWorkflowStack()
	if len(stack) != 1 {
		t.Fatalf("expected 1 active workflow, got %d", len(stack))
	}
	if stack[0].CurrentStep != "start" {
		t.Errorf("expected current step 'start', got '%s'", stack[0].CurrentStep)
	}
	_ = lastAllowed
}

func TestStatePersistenceAcrossSteps(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{"existing_tool"} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	var storedVal string

	Workflow("stateful",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepArgs(StrArg("val")),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				storedVal = args["val"].(string)
				return StepResult{Result: "stored"}, nil
			}),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "data=" + storedVal}, nil
			}),
		),
	)

	ctx := workflowTestCtx()
	HandleStepCall("stateful", "start", ctx, map[string]interface{}{"val": "foo"})
	result := HandleStepCall("stateful", "end", ctx, map[string]interface{}{})
	if !strings.Contains(result.ResultText, "data=foo") {
		t.Errorf("expected 'data=foo' in result, got '%s'", result.ResultText)
	}
}

func TestDynamicNextNarrowing(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	var lastAllowed []string
	ToolManagerAdapter.SetAllowed = func(tools []string) { lastAllowed = tools }

	Workflow("dyn",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("a", "b"),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "pick a", Next: []string{"a"}}, nil
			}),
		),
		Step("a", StepDescription("A"), StepTerminal()),
		Step("b", StepDescription("B"), StepTerminal()),
	)

	ctx := workflowTestCtx()
	HandleStepCall("dyn", "start", ctx, map[string]interface{}{})

	// Check that set_allowed was called with only "dyn.a" (not "dyn.b")
	hasA := false
	hasB := false
	for _, tool := range lastAllowed {
		if tool == "dyn.a" {
			hasA = true
		}
		if tool == "dyn.b" {
			hasB = true
		}
	}
	if !hasA {
		t.Error("expected 'dyn.a' in allowed tools")
	}
	if hasB {
		t.Error("did not expect 'dyn.b' in allowed tools")
	}
}

func TestDynamicNextRejectsInvalid(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	Workflow("dyn2",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("a"),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "bad", Next: []string{"a", "ghost"}}, nil
			}),
		),
		Step("a", StepDescription("A"), StepTerminal()),
	)

	ctx := workflowTestCtx()
	result := HandleStepCall("dyn2", "start", ctx, map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for invalid dynamic next")
	}
	if !strings.Contains(result.ResultText, "invalid next") {
		t.Errorf("expected 'invalid next' in result, got '%s'", result.ResultText)
	}
}

func TestCancelCallsOnCancel(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{"t1", "t2"} }
	var lastAllowed []string
	ToolManagerAdapter.SetAllowed = func(tools []string) { lastAllowed = tools }

	cancelCalled := false

	Workflow("canc",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
		OnCancel(func(currentStep string, history []StepHistoryEntry) string {
			cancelCalled = true
			return "cancelled"
		}),
	)

	ctx := workflowTestCtx()
	HandleStepCall("canc", "start", ctx, map[string]interface{}{})
	result := HandleCancel("canc")

	if !strings.Contains(result.ResultText, "cancelled") {
		t.Errorf("expected 'cancelled' in result, got '%s'", result.ResultText)
	}
	if !cancelCalled {
		t.Error("expected on_cancel to be called")
	}
	// Should restore pre-workflow tools
	if len(lastAllowed) != 2 || lastAllowed[0] != "t1" || lastAllowed[1] != "t2" {
		t.Errorf("expected pre-workflow tools restored, got %v", lastAllowed)
	}
}

func TestOnCompleteCalledOnTerminal(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	completeCalled := false

	Workflow("comp",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "finished"}, nil
			}),
		),
		OnComplete(func(history []StepHistoryEntry) {
			completeCalled = true
		}),
	)

	ctx := workflowTestCtx()
	HandleStepCall("comp", "start", ctx, map[string]interface{}{})
	result := HandleStepCall("comp", "end", ctx, map[string]interface{}{})

	if !completeCalled {
		t.Error("expected on_complete to be called")
	}
	if !strings.Contains(result.ResultText, "finished") {
		t.Errorf("expected 'finished' in result, got '%s'", result.ResultText)
	}
}

func TestHistoryTracking(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	Workflow("hist",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("mid"),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "s1"}, nil
			}),
		),
		Step("mid",
			StepDescription("Mid"),
			StepNext("end"),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "s2"}, nil
			}),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
	)

	ctx := workflowTestCtx()
	HandleStepCall("hist", "start", ctx, map[string]interface{}{})
	HandleStepCall("hist", "mid", ctx, map[string]interface{}{})

	stack := GetActiveWorkflowStack()
	if len(stack) != 1 {
		t.Fatalf("expected 1 active workflow, got %d", len(stack))
	}
	history := stack[0].History
	if len(history) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(history))
	}
	if history[0].StepName != "start" {
		t.Errorf("expected first history entry 'start', got '%s'", history[0].StepName)
	}
	if history[1].StepName != "mid" {
		t.Errorf("expected second history entry 'mid', got '%s'", history[1].StepName)
	}
	if history[0].Result.Result != "s1" {
		t.Errorf("expected first result 's1', got '%s'", history[0].Result.Result)
	}
}

func TestErrorStaysInState(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	callCount := 0

	Workflow("err1",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				callCount++
				if callCount == 1 {
					return StepResult{}, errors.New("transient")
				}
				return StepResult{Result: "ok"}, nil
			}),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
	)

	ctx := workflowTestCtx()
	result1 := HandleStepCall("err1", "start", ctx, map[string]interface{}{})
	if !result1.IsError {
		t.Error("expected error on first call")
	}
	if !strings.Contains(result1.ResultText, "transient") {
		t.Errorf("expected 'transient' in error, got '%s'", result1.ResultText)
	}

	// Clear stack and retry as fresh initial call
	ClearActiveWorkflowStack()
	result2 := HandleStepCall("err1", "start", ctx, map[string]interface{}{})
	if result2.IsError {
		t.Errorf("expected success on retry, got error: %s", result2.ResultText)
	}
	if !strings.Contains(result2.ResultText, "ok") {
		t.Errorf("expected 'ok' in result, got '%s'", result2.ResultText)
	}
}

func TestOnErrorTransitions(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	Workflow("err2",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepOnError(map[string]string{"bad value": "fix"}),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{}, errors.New("bad value")
			}),
		),
		Step("fix",
			StepDescription("Fix"),
			StepNext("end"),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
	)

	ctx := workflowTestCtx()
	result := HandleStepCall("err2", "start", ctx, map[string]interface{}{})
	if result.IsError {
		t.Errorf("expected non-error (transition), got error: %s", result.ResultText)
	}
	if !strings.Contains(result.ResultText, "transitioning to 'fix'") {
		t.Errorf("expected 'transitioning to fix' in result, got '%s'", result.ResultText)
	}

	stack := GetActiveWorkflowStack()
	if len(stack) != 1 {
		t.Fatalf("expected 1 active workflow, got %d", len(stack))
	}
	if stack[0].CurrentStep != "fix" {
		t.Errorf("expected current step 'fix', got '%s'", stack[0].CurrentStep)
	}
}

func TestUnmatchedErrorStaysInState(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	Workflow("err4",
		WorkflowDescription("test"),
		Step("start",
			StepDescription("Start"),
			StepInitial(),
			StepNext("end"),
			StepOnError(map[string]string{"bad value": "end"}),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{}, errors.New("not a value error")
			}),
		),
		Step("end",
			StepDescription("End"),
			StepTerminal(),
		),
	)

	ctx := workflowTestCtx()
	result := HandleStepCall("err4", "start", ctx, map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for unmatched error")
	}
	if !strings.Contains(strings.ToLower(result.ResultText), "retry") && !strings.Contains(strings.ToLower(result.ResultText), "failed") {
		t.Errorf("expected 'retry' or 'failed' in result, got '%s'", result.ResultText)
	}
}

// --- Visibility matching ---

func TestMatchesVisibilityBothNil(t *testing.T) {
	if matchesVisibility("anything", nil, nil) {
		t.Error("expected false when both allow and block are nil")
	}
}

func TestMatchesVisibilityAllowOnly(t *testing.T) {
	if !matchesVisibility("foo.bar", []string{"foo.*"}, nil) {
		t.Error("expected true for matching allow pattern")
	}
	if matchesVisibility("baz.qux", []string{"foo.*"}, nil) {
		t.Error("expected false for non-matching allow pattern")
	}
}

func TestMatchesVisibilityBlockOnly(t *testing.T) {
	if matchesVisibility("foo.bar", nil, []string{"foo.*"}) {
		t.Error("expected false for matching block pattern")
	}
	if !matchesVisibility("baz.qux", nil, []string{"foo.*"}) {
		t.Error("expected true for non-matching block pattern")
	}
}

func TestMatchesVisibilityAllowAndBlock(t *testing.T) {
	// Allow all, block specific
	if matchesVisibility("foo.secret", []string{"foo.*"}, []string{"*.secret"}) {
		t.Error("expected false: allowed by allow but blocked by block")
	}
	if !matchesVisibility("foo.bar", []string{"foo.*"}, []string{"*.secret"}) {
		t.Error("expected true: allowed and not blocked")
	}
}

func TestStepVisibilityOverridesWorkflow(t *testing.T) {
	wf := &WorkflowDef{
		AllowDuring: []string{"wf.*"},
		BlockDuring: []string{"wf.secret"},
	}

	// Step with its own visibility
	stepWithOwn := &StepDef{
		AllowDuring: []string{"step.*"},
	}
	allow, block := getStepVisibility(stepWithOwn, wf)
	if len(allow) != 1 || allow[0] != "step.*" {
		t.Errorf("expected step allow override, got %v", allow)
	}
	if block != nil {
		t.Errorf("expected nil block from step override, got %v", block)
	}

	// Step without its own visibility falls back to workflow
	stepWithout := &StepDef{}
	allow, block = getStepVisibility(stepWithout, wf)
	if len(allow) != 1 || allow[0] != "wf.*" {
		t.Errorf("expected workflow allow, got %v", allow)
	}
	if len(block) != 1 || block[0] != "wf.secret" {
		t.Errorf("expected workflow block, got %v", block)
	}
}

// --- Workflows in GetRegisteredTools ---

func TestWorkflowsInGetRegisteredTools(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("wf_tools_test",
		WorkflowDescription("Test workflow"),
		Step("begin",
			StepDescription("Begin"),
			StepInitial(),
			StepNext("finish"),
		),
		Step("finish",
			StepDescription("Finish"),
			StepTerminal(),
		),
	)

	tools := GetRegisteredTools()
	found := map[string]bool{}
	for _, td := range tools {
		found[td.Name] = true
	}
	if !found["wf_tools_test.begin"] {
		t.Error("expected 'wf_tools_test.begin' in registered tools")
	}
	if !found["wf_tools_test.finish"] {
		t.Error("expected 'wf_tools_test.finish' in registered tools")
	}
	if !found["wf_tools_test.cancel"] {
		t.Error("expected 'wf_tools_test.cancel' in registered tools")
	}
}

// --- Cancel with no active workflow ---

func TestCancelNoActiveWorkflow(t *testing.T) {
	setupWorkflowTest(t)

	result := HandleCancel("nonexistent")
	if !result.IsError {
		t.Error("expected error when cancelling nonexistent workflow")
	}
	if !strings.Contains(result.ResultText, "No active workflow") {
		t.Errorf("expected 'No active workflow' in result, got '%s'", result.ResultText)
	}
}

// --- Non-initial step without active workflow ---

func TestNonInitialStepWithoutActiveWorkflow(t *testing.T) {
	setupWorkflowTest(t)

	Workflow("noactive",
		WorkflowDescription("test"),
		Step("start", StepDescription("S"), StepInitial(), StepNext("end")),
		Step("end", StepDescription("E"), StepTerminal()),
	)

	ctx := workflowTestCtx()
	result := HandleStepCall("noactive", "end", ctx, map[string]interface{}{})
	if !result.IsError {
		t.Error("expected error for non-initial step without active workflow")
	}
	if !strings.Contains(result.ResultText, "No active workflow") {
		t.Errorf("expected 'No active workflow' in result, got '%s'", result.ResultText)
	}
}

// --- Tool handler dispatch via generated ToolDef ---

func TestToolDefHandlerDispatch(t *testing.T) {
	setupWorkflowTest(t)

	ToolManagerAdapter.GetActiveTools = func() []string { return []string{} }
	ToolManagerAdapter.SetAllowed = func(tools []string) {}

	Workflow("handler_wf",
		WorkflowDescription("test"),
		Step("go",
			StepDescription("Go"),
			StepInitial(),
			StepNext("done"),
			StepArgs(StrArg("name")),
			StepHandler(func(ctx ToolContext, args map[string]interface{}) (StepResult, error) {
				return StepResult{Result: "Hello " + args["name"].(string)}, nil
			}),
		),
		Step("done", StepDescription("Done"), StepTerminal()),
	)

	defs := WorkflowsToToolDefs()
	var goDef *ToolDef
	for i := range defs {
		if defs[i].Name == "handler_wf.go" {
			goDef = &defs[i]
			break
		}
	}
	if goDef == nil {
		t.Fatal("expected 'handler_wf.go' tool def")
	}

	ctx := workflowTestCtx()
	result := goDef.HandlerFn(ctx, map[string]interface{}{"name": "World"})
	if result.IsError {
		t.Errorf("unexpected error: %s", result.ResultText)
	}
	if !strings.Contains(result.ResultText, "Hello World") {
		t.Errorf("expected 'Hello World' in result, got '%s'", result.ResultText)
	}
}
