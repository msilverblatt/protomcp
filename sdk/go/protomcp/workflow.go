package protomcp

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// StepResult is returned by step handlers to indicate the result and optionally narrow next steps.
type StepResult struct {
	Result string
	Next   []string // nil means use declared next; non-nil narrows declared next (must be subset)
}

// StepHistoryEntry records a step execution in the workflow history.
type StepHistoryEntry struct {
	StepName string
	Result   StepResult
}

// StepDef defines a single step within a workflow.
type StepDef struct {
	Name        string
	Description string
	HandlerFn   func(ToolContext, map[string]interface{}) (StepResult, error)
	InputSchema map[string]interface{}
	Initial     bool
	Next        []string
	Terminal    bool
	NoCancel    bool
	AllowDuring []string
	BlockDuring []string
	OnError     map[string]string // error substring -> target step
	Requires    []string
	EnumFields  map[string][]string
}

// WorkflowDef defines a complete workflow with its steps and callbacks.
type WorkflowDef struct {
	Name        string
	Description string
	Steps       []StepDef
	AllowDuring []string
	BlockDuring []string
	OnCancelFn  func(currentStep string, history []StepHistoryEntry) string
	OnCompleteFn func(history []StepHistoryEntry)
}

// WorkflowState tracks the runtime state of an active workflow instance.
type WorkflowState struct {
	WorkflowName    string
	CurrentStep     string
	History         []StepHistoryEntry
	PreWorkflowTools []string
}

// --- Registry ---

var workflowRegistry []WorkflowDef
var activeWorkflowStack []WorkflowState

// --- Functional options ---

type WorkflowOption func(*WorkflowDef)
type StepOption func(*StepDef)

// Workflow registers a workflow definition with the given name and options.
// Panics if graph validation fails (Go convention for registration-time errors).
func Workflow(name string, opts ...WorkflowOption) {
	wf := WorkflowDef{
		Name: name,
	}
	for _, opt := range opts {
		opt(&wf)
	}
	validateWorkflowGraph(&wf)
	workflowRegistry = append(workflowRegistry, wf)
}

// WorkflowDescription sets the workflow description.
func WorkflowDescription(desc string) WorkflowOption {
	return func(wf *WorkflowDef) { wf.Description = desc }
}

// AllowDuring sets workflow-level allow patterns for tool visibility during the workflow.
func AllowDuring(patterns ...string) WorkflowOption {
	return func(wf *WorkflowDef) { wf.AllowDuring = patterns }
}

// BlockDuring sets workflow-level block patterns for tool visibility during the workflow.
func BlockDuring(patterns ...string) WorkflowOption {
	return func(wf *WorkflowDef) { wf.BlockDuring = patterns }
}

// OnCancel sets the cancel callback for the workflow.
func OnCancel(fn func(currentStep string, history []StepHistoryEntry) string) WorkflowOption {
	return func(wf *WorkflowDef) { wf.OnCancelFn = fn }
}

// OnComplete sets the completion callback for the workflow.
func OnComplete(fn func(history []StepHistoryEntry)) WorkflowOption {
	return func(wf *WorkflowDef) { wf.OnCompleteFn = fn }
}

// Step adds a step to the workflow.
func Step(name string, opts ...StepOption) WorkflowOption {
	return func(wf *WorkflowDef) {
		sd := StepDef{
			Name:       name,
			EnumFields: map[string][]string{},
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		}
		for _, opt := range opts {
			opt(&sd)
		}
		wf.Steps = append(wf.Steps, sd)
	}
}

// --- Step options ---

func StepDescription(desc string) StepOption {
	return func(sd *StepDef) { sd.Description = desc }
}

func StepInitial() StepOption {
	return func(sd *StepDef) { sd.Initial = true }
}

func StepTerminal() StepOption {
	return func(sd *StepDef) { sd.Terminal = true }
}

func StepNoCancel() StepOption {
	return func(sd *StepDef) { sd.NoCancel = true }
}

func StepNext(names ...string) StepOption {
	return func(sd *StepDef) { sd.Next = names }
}

func StepHandler(fn func(ToolContext, map[string]interface{}) (StepResult, error)) StepOption {
	return func(sd *StepDef) { sd.HandlerFn = fn }
}

func StepAllowDuring(patterns ...string) StepOption {
	return func(sd *StepDef) { sd.AllowDuring = patterns }
}

func StepBlockDuring(patterns ...string) StepOption {
	return func(sd *StepDef) { sd.BlockDuring = patterns }
}

func StepOnError(mapping map[string]string) StepOption {
	return func(sd *StepDef) { sd.OnError = mapping }
}

func StepRequires(fields ...string) StepOption {
	return func(sd *StepDef) { sd.Requires = fields }
}

func StepEnumField(name string, values ...string) StepOption {
	return func(sd *StepDef) { sd.EnumFields[name] = values }
}

func StepArgs(args ...ArgDef) StepOption {
	return func(sd *StepDef) {
		props := map[string]interface{}{}
		required := []string{}
		for _, a := range args {
			props[a.Name] = argToSchema(a)
			required = append(required, a.Name)
		}
		sd.InputSchema = map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}
}

// --- Graph validation ---

func validateWorkflowGraph(wf *WorkflowDef) {
	stepNames := map[string]bool{}
	for _, s := range wf.Steps {
		stepNames[s.Name] = true
	}

	// Check initial steps
	var initialSteps []string
	for _, s := range wf.Steps {
		if s.Initial {
			initialSteps = append(initialSteps, s.Name)
		}
	}
	if len(initialSteps) == 0 {
		panic(fmt.Sprintf("workflow '%s': no initial step defined", wf.Name))
	}
	if len(initialSteps) > 1 {
		panic(fmt.Sprintf("workflow '%s': multiple initial steps: %v", wf.Name, initialSteps))
	}

	for _, s := range wf.Steps {
		// Terminal step must not have next
		if s.Terminal && s.Next != nil {
			panic(fmt.Sprintf("workflow '%s': terminal step '%s' has next", wf.Name, s.Name))
		}

		// Non-terminal step must have next
		if !s.Terminal && s.Next == nil {
			panic(fmt.Sprintf("workflow '%s': non-terminal step '%s' has no next (dead end)", wf.Name, s.Name))
		}

		// next references must exist
		if s.Next != nil {
			for _, ref := range s.Next {
				if !stepNames[ref] {
					panic(fmt.Sprintf("workflow '%s': step '%s' references nonexistent step '%s'", wf.Name, s.Name, ref))
				}
			}
		}

		// on_error targets must exist
		if s.OnError != nil {
			for errKey, target := range s.OnError {
				if !stepNames[target] {
					panic(fmt.Sprintf("workflow '%s': step '%s' on_error '%s' references nonexistent step '%s'", wf.Name, s.Name, errKey, target))
				}
			}
		}
	}
}

// --- Visibility matching ---

func matchesVisibility(toolName string, allowDuring []string, blockDuring []string) bool {
	if allowDuring == nil && blockDuring == nil {
		return false
	}

	if allowDuring != nil {
		allowed := false
		for _, pat := range allowDuring {
			if matched, _ := path.Match(pat, toolName); matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	if blockDuring != nil {
		for _, pat := range blockDuring {
			if matched, _ := path.Match(pat, toolName); matched {
				return false
			}
		}
	}

	return true
}

func getStepVisibility(stepDef *StepDef, workflowDef *WorkflowDef) ([]string, []string) {
	if stepDef.AllowDuring != nil || stepDef.BlockDuring != nil {
		return stepDef.AllowDuring, stepDef.BlockDuring
	}
	return workflowDef.AllowDuring, workflowDef.BlockDuring
}

// --- Lookup helpers ---

func findWorkflow(name string) *WorkflowDef {
	for i := range workflowRegistry {
		if workflowRegistry[i].Name == name {
			return &workflowRegistry[i]
		}
	}
	return nil
}

func findStep(wf *WorkflowDef, stepName string) *StepDef {
	for i := range wf.Steps {
		if wf.Steps[i].Name == stepName {
			return &wf.Steps[i]
		}
	}
	return nil
}

func getActiveState() *WorkflowState {
	if len(activeWorkflowStack) > 0 {
		return &activeWorkflowStack[len(activeWorkflowStack)-1]
	}
	return nil
}

// --- Transition ---

// ToolManagerAdapter allows the workflow system to interact with tool visibility.
// Set this to integrate with the actual tool manager.
var ToolManagerAdapter struct {
	GetActiveTools func() []string
	SetAllowed     func([]string)
}

func transitionToSteps(wf *WorkflowDef, state *WorkflowState, nextStepNames []string) []string {
	stepMap := map[string]*StepDef{}
	for i := range wf.Steps {
		stepMap[wf.Steps[i].Name] = &wf.Steps[i]
	}

	var allowedTools []string

	// Add the next step tools
	for _, sn := range nextStepNames {
		allowedTools = append(allowedTools, fmt.Sprintf("%s.%s", wf.Name, sn))
	}

	// Add cancel tool if any next step allows cancel
	for _, sn := range nextStepNames {
		if s, ok := stepMap[sn]; ok && !s.NoCancel {
			allowedTools = append(allowedTools, fmt.Sprintf("%s.cancel", wf.Name))
			break
		}
	}

	// Add visibility-matched tools from pre_workflow_tools
	if len(nextStepNames) > 0 {
		if firstStep, ok := stepMap[nextStepNames[0]]; ok {
			allowDuring, blockDuring := getStepVisibility(firstStep, wf)
			for _, toolName := range state.PreWorkflowTools {
				if matchesVisibility(toolName, allowDuring, blockDuring) {
					allowedTools = append(allowedTools, toolName)
				}
			}
		}
	}

	if ToolManagerAdapter.SetAllowed != nil {
		ToolManagerAdapter.SetAllowed(allowedTools)
	}

	return allowedTools
}

// --- Step dispatch ---

func HandleStepCall(workflowName, stepName string, ctx ToolContext, args map[string]interface{}) ToolResult {
	wf := findWorkflow(workflowName)
	if wf == nil {
		return ErrorResult(fmt.Sprintf("Unknown workflow: %s", workflowName), "", "", false)
	}

	stepDef := findStep(wf, stepName)
	if stepDef == nil {
		return ErrorResult(fmt.Sprintf("Unknown step: %s", stepName), "", "", false)
	}

	state := getActiveState()

	if stepDef.Initial {
		// Start a new workflow
		var preTools []string
		if ToolManagerAdapter.GetActiveTools != nil {
			preTools = ToolManagerAdapter.GetActiveTools()
		}
		newState := WorkflowState{
			WorkflowName:     workflowName,
			CurrentStep:      stepName,
			History:          nil,
			PreWorkflowTools: preTools,
		}
		activeWorkflowStack = append(activeWorkflowStack, newState)
		state = &activeWorkflowStack[len(activeWorkflowStack)-1]
	} else {
		if state == nil || state.WorkflowName != workflowName {
			return ErrorResult(
				fmt.Sprintf("No active workflow '%s' to continue", workflowName),
				"", "", false,
			)
		}
		state.CurrentStep = stepName
	}

	// Run the handler
	var result StepResult
	var handlerErr error
	if stepDef.HandlerFn != nil {
		result, handlerErr = stepDef.HandlerFn(ctx, args)
	}

	if handlerErr != nil {
		// Check on_error mapping
		if stepDef.OnError != nil {
			errStr := handlerErr.Error()
			for errKey, targetStep := range stepDef.OnError {
				if strings.Contains(errStr, errKey) {
					state.CurrentStep = targetStep
					allowedTools := transitionToSteps(wf, state, []string{targetStep})
					allTools := GetRegisteredTools()
					allowedSet := map[string]bool{}
					for _, t := range allowedTools {
						allowedSet[t] = true
					}
					var disableTools []string
					for _, t := range allTools {
						if !allowedSet[t.Name] {
							disableTools = append(disableTools, t.Name)
						}
					}
					r := Result(fmt.Sprintf("Error caught (%s), transitioning to '%s'", errStr, targetStep))
					r.EnableTools = allowedTools
					r.DisableTools = disableTools
					return r
				}
			}
		}
		// No matching on_error: stay in current state for retry
		return ErrorResult(
			fmt.Sprintf("Step '%s' failed: %s. You can retry.", stepName, handlerErr.Error()),
			"", "", true,
		)
	}

	// Validate dynamic next
	if result.Next != nil {
		if stepDef.Next == nil {
			return ErrorResult(
				fmt.Sprintf("Step '%s' returned next=%v but has no declared next", stepName, result.Next),
				"", "", false,
			)
		}
		declaredSet := map[string]bool{}
		for _, n := range stepDef.Next {
			declaredSet[n] = true
		}
		for _, n := range result.Next {
			if !declaredSet[n] {
				invalid := []string{}
				for _, rn := range result.Next {
					if !declaredSet[rn] {
						invalid = append(invalid, rn)
					}
				}
				return ErrorResult(
					fmt.Sprintf("Step '%s' returned invalid next steps: %v. Declared: %v", stepName, invalid, stepDef.Next),
					"", "", false,
				)
			}
		}
	}

	// Record in history
	state.History = append(state.History, StepHistoryEntry{
		StepName: stepName,
		Result:   result,
	})

	// Determine effective next
	effectiveNext := result.Next
	if effectiveNext == nil {
		effectiveNext = stepDef.Next
	}

	if stepDef.Terminal {
		// Workflow complete
		if wf.OnCompleteFn != nil {
			wf.OnCompleteFn(state.History)
		}
		// Restore pre-workflow tools
		if ToolManagerAdapter.SetAllowed != nil {
			ToolManagerAdapter.SetAllowed(state.PreWorkflowTools)
		}
		activeWorkflowStack = activeWorkflowStack[:len(activeWorkflowStack)-1]
		resultText := result.Result
		if resultText == "" {
			resultText = "Workflow complete"
		}
		r := Result(resultText)
		r.EnableTools = state.PreWorkflowTools
		r.DisableTools = []string{}
		return r
	}

	// Transition to next steps
	allowedTools := transitionToSteps(wf, state, effectiveNext)
	allTools := GetRegisteredTools()
	allowedSet := map[string]bool{}
	for _, t := range allowedTools {
		allowedSet[t] = true
	}
	var disableTools []string
	for _, t := range allTools {
		if !allowedSet[t.Name] {
			disableTools = append(disableTools, t.Name)
		}
	}
	resultText := result.Result
	if resultText == "" {
		resultText = fmt.Sprintf("Proceed to: %v", effectiveNext)
	}
	r := Result(resultText)
	r.EnableTools = allowedTools
	r.DisableTools = disableTools
	return r
}

// HandleCancel handles a cancel tool call for the given workflow.
func HandleCancel(workflowName string) ToolResult {
	state := getActiveState()
	if state == nil || state.WorkflowName != workflowName {
		return ErrorResult(
			fmt.Sprintf("No active workflow '%s' to cancel", workflowName),
			"", "", false,
		)
	}

	wf := findWorkflow(workflowName)
	if wf == nil {
		return ErrorResult(fmt.Sprintf("Unknown workflow: %s", workflowName), "", "", false)
	}

	if wf.OnCancelFn != nil {
		wf.OnCancelFn(state.CurrentStep, state.History)
	}

	// Restore pre-workflow tools
	if ToolManagerAdapter.SetAllowed != nil {
		ToolManagerAdapter.SetAllowed(state.PreWorkflowTools)
	}
	activeWorkflowStack = activeWorkflowStack[:len(activeWorkflowStack)-1]
	r := Result(fmt.Sprintf("Workflow '%s' cancelled", workflowName))
	r.EnableTools = state.PreWorkflowTools
	r.DisableTools = []string{}
	return r
}

// --- Tool generation ---

// WorkflowsToToolDefs converts registered workflows into ToolDef list.
func WorkflowsToToolDefs() []ToolDef {
	var defs []ToolDef
	for _, wf := range workflowRegistry {
		hasCancelable := false
		for _, s := range wf.Steps {
			if !s.Terminal && !s.NoCancel {
				hasCancelable = true
				break
			}
		}

		for _, s := range wf.Steps {
			toolName := fmt.Sprintf("%s.%s", wf.Name, s.Name)
			desc := s.Description
			if desc == "" {
				desc = fmt.Sprintf("%s %s", wf.Name, s.Name)
			}

			// Capture for closure
			wfName := wf.Name
			sName := s.Name
			schemaJSON, _ := json.Marshal(s.InputSchema)

			defs = append(defs, ToolDef{
				Name: toolName,
				Desc: desc,
				InputSchema: s.InputSchema,
				HandlerFn: func(ctx ToolContext, args map[string]interface{}) ToolResult {
					return HandleStepCall(wfName, sName, ctx, args)
				},
				Hidden: !s.Initial,
			})
			_ = schemaJSON // used for JSON representation if needed
		}

		if hasCancelable {
			cancelName := fmt.Sprintf("%s.cancel", wf.Name)
			wfName := wf.Name
			defs = append(defs, ToolDef{
				Name: cancelName,
				Desc: fmt.Sprintf("Cancel the %s workflow", wf.Name),
				InputSchema: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
				HandlerFn: func(ctx ToolContext, args map[string]interface{}) ToolResult {
					return HandleCancel(wfName)
				},
				Hidden: true,
			})
		}
	}
	return defs
}

// --- Registry access ---

func GetRegisteredWorkflows() []WorkflowDef {
	result := make([]WorkflowDef, len(workflowRegistry))
	copy(result, workflowRegistry)
	return result
}

func ClearWorkflowRegistry() {
	workflowRegistry = nil
	activeWorkflowStack = nil
}

// GetActiveWorkflowStack returns a copy of the active workflow stack (for testing).
func GetActiveWorkflowStack() []WorkflowState {
	result := make([]WorkflowState, len(activeWorkflowStack))
	copy(result, activeWorkflowStack)
	return result
}

// ClearActiveWorkflowStack clears the active workflow stack (for testing).
func ClearActiveWorkflowStack() {
	activeWorkflowStack = nil
}
