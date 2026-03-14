import { z } from 'zod';
import { zodToJsonSchema } from 'zod-to-json-schema';
import type { ToolContext } from './context.js';
import { type ToolDef, _setWorkflowsToToolDefs } from './tool.js';
import { ToolResult } from './result.js';
import { toolManager } from './manager.js';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export class StepResult {
  result: string;
  next?: string[];

  constructor(result: string = '', next?: string[]) {
    this.result = result;
    this.next = next;
  }
}

export interface StepOptions<T extends z.ZodObject<any>> {
  description?: string;
  args?: T;
  handler: (args: z.infer<T>, ctx: ToolContext) => StepResult | string;
  initial?: boolean;
  next?: string[];
  terminal?: boolean;
  noCancel?: boolean;
  allowDuring?: string[];
  blockDuring?: string[];
  onError?: Record<string, string>; // error message substring -> step name
  requires?: string[];
  enumFields?: Record<string, string[]>;
}

interface StepDef {
  name: string;
  description: string;
  args: z.ZodObject<any> | undefined;
  handler: (args: any, ctx: ToolContext) => StepResult | string;
  initial: boolean;
  next: string[] | undefined;
  terminal: boolean;
  noCancel: boolean;
  allowDuring: string[] | undefined;
  blockDuring: string[] | undefined;
  onError: Record<string, string> | undefined;
  requires: string[] | undefined;
  enumFields: Record<string, string[]> | undefined;
}

export interface WorkflowDef {
  name: string;
  description: string;
  steps: StepDef[];
  allowDuring: string[] | undefined;
  blockDuring: string[] | undefined;
  onCancel: ((currentStep: string, history: StepHistoryEntry[]) => string) | undefined;
  onComplete: ((history: StepHistoryEntry[]) => void) | undefined;
}

export interface StepHistoryEntry {
  step: string;
  result: StepResult;
}

export interface WorkflowOptions {
  name: string;
  description?: string;
  steps: Record<string, StepOptions<any>>;
  allowDuring?: string[];
  blockDuring?: string[];
  onCancel?: (currentStep: string, history: StepHistoryEntry[]) => string;
  onComplete?: (history: StepHistoryEntry[]) => void;
}

interface WorkflowState {
  workflowName: string;
  currentStep: string;
  history: StepHistoryEntry[];
  preWorkflowTools: string[];
}

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

const workflowRegistry: WorkflowDef[] = [];
const activeWorkflowStack: WorkflowState[] = [];

// ---------------------------------------------------------------------------
// Simple glob matching (fnmatch-style)
// ---------------------------------------------------------------------------

function fnmatch(name: string, pattern: string): boolean {
  // Convert fnmatch pattern to regex
  let regex = '^';
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i];
    if (c === '*') {
      regex += '.*';
    } else if (c === '?') {
      regex += '.';
    } else if (c === '[') {
      // Find closing bracket
      const end = pattern.indexOf(']', i + 1);
      if (end === -1) {
        regex += '\\[';
      } else {
        regex += '[' + pattern.slice(i + 1, end) + ']';
        i = end;
      }
    } else {
      // Escape regex special chars
      regex += c.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    }
  }
  regex += '$';
  return new RegExp(regex).test(name);
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

function validateWorkflowGraph(wf: WorkflowDef): void {
  const stepNames = new Set(wf.steps.map(s => s.name));

  const initialSteps = wf.steps.filter(s => s.initial);
  if (initialSteps.length === 0) {
    throw new Error(`Workflow '${wf.name}': no initial step defined`);
  }
  if (initialSteps.length > 1) {
    const names = initialSteps.map(s => s.name);
    throw new Error(`Workflow '${wf.name}': multiple initial steps: ${JSON.stringify(names)}`);
  }

  for (const s of wf.steps) {
    if (s.terminal && s.next !== undefined) {
      throw new Error(`Workflow '${wf.name}': terminal step '${s.name}' has next`);
    }

    if (!s.terminal && s.next === undefined) {
      throw new Error(`Workflow '${wf.name}': non-terminal step '${s.name}' has no next (dead end)`);
    }

    if (s.next !== undefined) {
      for (const ref of s.next) {
        if (!stepNames.has(ref)) {
          throw new Error(`Workflow '${wf.name}': step '${s.name}' references nonexistent step '${ref}'`);
        }
      }
    }

    if (s.onError !== undefined) {
      for (const [, target] of Object.entries(s.onError)) {
        if (!stepNames.has(target)) {
          throw new Error(`Workflow '${wf.name}': step '${s.name}' onError references nonexistent step '${target}'`);
        }
      }
    }
  }
}

// ---------------------------------------------------------------------------
// Visibility helpers
// ---------------------------------------------------------------------------

function matchesVisibility(toolName: string, allowDuring: string[] | undefined, blockDuring: string[] | undefined): boolean {
  if (allowDuring === undefined && blockDuring === undefined) {
    return false;
  }

  if (allowDuring !== undefined) {
    const allowed = allowDuring.some(pat => fnmatch(toolName, pat));
    if (!allowed) return false;
  }

  if (blockDuring !== undefined) {
    const blocked = blockDuring.some(pat => fnmatch(toolName, pat));
    if (blocked) return false;
  }

  return true;
}

function getStepVisibility(stepDef: StepDef, workflowDef: WorkflowDef): { allowDuring: string[] | undefined; blockDuring: string[] | undefined } {
  if (stepDef.allowDuring !== undefined || stepDef.blockDuring !== undefined) {
    return { allowDuring: stepDef.allowDuring, blockDuring: stepDef.blockDuring };
  }
  return { allowDuring: workflowDef.allowDuring, blockDuring: workflowDef.blockDuring };
}

function transitionToSteps(workflowDef: WorkflowDef, state: WorkflowState, nextStepNames: string[]): void {
  const stepMap = new Map(workflowDef.steps.map(s => [s.name, s]));

  const allowedTools: string[] = [];

  // Add next step tools
  for (const sn of nextStepNames) {
    allowedTools.push(`${workflowDef.name}.${sn}`);
  }

  // Add cancel tool if any next step allows cancel
  const anyCancelable = nextStepNames.some(sn => {
    const step = stepMap.get(sn);
    return step && !step.noCancel;
  });
  if (anyCancelable) {
    allowedTools.push(`${workflowDef.name}.cancel`);
  }

  // Add visibility-matched tools from preWorkflowTools
  if (nextStepNames.length > 0) {
    const firstStep = stepMap.get(nextStepNames[0]);
    if (firstStep) {
      const { allowDuring, blockDuring } = getStepVisibility(firstStep, workflowDef);
      for (const toolName of state.preWorkflowTools) {
        if (matchesVisibility(toolName, allowDuring, blockDuring)) {
          allowedTools.push(toolName);
        }
      }
    }
  }

  toolManager.setAllowed(allowedTools);
}

// ---------------------------------------------------------------------------
// Lookup helpers
// ---------------------------------------------------------------------------

function findWorkflow(name: string): WorkflowDef | undefined {
  return workflowRegistry.find(wf => wf.name === name);
}

function findStep(workflowDef: WorkflowDef, stepName: string): StepDef | undefined {
  return workflowDef.steps.find(s => s.name === stepName);
}

function getActiveState(): WorkflowState | undefined {
  return activeWorkflowStack.length > 0 ? activeWorkflowStack[activeWorkflowStack.length - 1] : undefined;
}

// ---------------------------------------------------------------------------
// Step dispatch
// ---------------------------------------------------------------------------

function handleStepCall(workflowName: string, stepName: string, kwargs: Record<string, any>, ctx: ToolContext): ToolResult {
  const wf = findWorkflow(workflowName);
  if (!wf) {
    return new ToolResult({ result: `Unknown workflow: ${workflowName}`, isError: true });
  }

  const stepDef = findStep(wf, stepName);
  if (!stepDef) {
    return new ToolResult({ result: `Unknown step: ${stepName}`, isError: true });
  }

  let state = getActiveState();

  if (stepDef.initial) {
    const preTools = toolManager.getActiveTools();
    state = {
      workflowName,
      currentStep: stepName,
      history: [],
      preWorkflowTools: [],
    };
    activeWorkflowStack.push(state);
    // getActiveTools is async; store a promise-resolved value or use sync fallback
    // In the workflow context, we handle this by storing tools asynchronously
    preTools.then((tools: string[]) => {
      state!.preWorkflowTools = tools;
    }).catch(() => {
      // If toolManager not connected, preWorkflowTools stays empty
    });
  } else {
    if (!state || state.workflowName !== workflowName) {
      return new ToolResult({
        result: `No active workflow '${workflowName}' to continue`,
        isError: true,
      });
    }
    state.currentStep = stepName;
  }

  // Run the handler
  let result: StepResult | string;
  try {
    result = stepDef.handler(kwargs, ctx);
  } catch (exc: any) {
    // Check onError mapping
    if (stepDef.onError) {
      const errMsg = String(exc?.message ?? exc);
      for (const [substring, targetStep] of Object.entries(stepDef.onError)) {
        if (errMsg.includes(substring)) {
          state!.currentStep = targetStep;
          transitionToSteps(wf, state!, [targetStep]);
          return new ToolResult({
            result: `Error caught (${errMsg}), transitioning to '${targetStep}'`,
          });
        }
      }
    }
    // No matching onError: stay in current state for retry
    return new ToolResult({
      result: `Step '${stepName}' failed: ${String(exc?.message ?? exc)}. You can retry.`,
      isError: true,
    });
  }

  // Normalize result
  if (!(result instanceof StepResult)) {
    result = new StepResult(result != null ? String(result) : '');
  }

  // Validate dynamic next
  if (result.next !== undefined) {
    if (stepDef.next === undefined) {
      return new ToolResult({
        result: `Step '${stepName}' returned next=${JSON.stringify(result.next)} but has no declared next`,
        isError: true,
      });
    }
    const declaredSet = new Set(stepDef.next);
    const invalid = result.next.filter(n => !declaredSet.has(n));
    if (invalid.length > 0) {
      return new ToolResult({
        result: `Step '${stepName}' returned invalid next steps: ${JSON.stringify(invalid)}. Declared: ${JSON.stringify(stepDef.next)}`,
        isError: true,
      });
    }
  }

  // Record in history
  state!.history.push({ step: stepName, result });

  // Determine effective next
  const effectiveNext = result.next !== undefined ? result.next : stepDef.next;

  if (stepDef.terminal) {
    // Workflow complete
    if (wf.onComplete) {
      wf.onComplete(state!.history);
    }
    // Restore pre-workflow tools
    toolManager.setAllowed(state!.preWorkflowTools);
    activeWorkflowStack.pop();
    return new ToolResult({ result: result.result || 'Workflow complete' });
  } else {
    // Transition to next steps
    transitionToSteps(wf, state!, effectiveNext || []);
    return new ToolResult({ result: result.result || `Proceed to: ${JSON.stringify(effectiveNext)}` });
  }
}

// ---------------------------------------------------------------------------
// Cancel handler
// ---------------------------------------------------------------------------

function handleCancel(workflowName: string): ToolResult {
  const state = getActiveState();
  if (!state || state.workflowName !== workflowName) {
    return new ToolResult({
      result: `No active workflow '${workflowName}' to cancel`,
      isError: true,
    });
  }

  const wf = findWorkflow(workflowName);
  if (!wf) {
    return new ToolResult({ result: `Unknown workflow: ${workflowName}`, isError: true });
  }

  if (wf.onCancel) {
    wf.onCancel(state.currentStep, state.history);
  }

  // Restore pre-workflow tools
  toolManager.setAllowed(state.preWorkflowTools);
  activeWorkflowStack.pop();
  return new ToolResult({ result: `Workflow '${workflowName}' cancelled` });
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

export function workflow(options: WorkflowOptions): void {
  const steps: StepDef[] = [];

  for (const [stepName, stepOpts] of Object.entries(options.steps)) {
    steps.push({
      name: stepName,
      description: stepOpts.description ?? '',
      args: stepOpts.args,
      handler: stepOpts.handler,
      initial: stepOpts.initial ?? false,
      next: stepOpts.next,
      terminal: stepOpts.terminal ?? false,
      noCancel: stepOpts.noCancel ?? false,
      allowDuring: stepOpts.allowDuring,
      blockDuring: stepOpts.blockDuring,
      onError: stepOpts.onError,
      requires: stepOpts.requires,
      enumFields: stepOpts.enumFields,
    });
  }

  const wf: WorkflowDef = {
    name: options.name,
    description: options.description ?? '',
    steps,
    allowDuring: options.allowDuring,
    blockDuring: options.blockDuring,
    onCancel: options.onCancel,
    onComplete: options.onComplete,
  };

  validateWorkflowGraph(wf);
  workflowRegistry.push(wf);
}

// ---------------------------------------------------------------------------
// Tool generation
// ---------------------------------------------------------------------------

export function workflowsToToolDefs(): ToolDef<any>[] {
  const defs: ToolDef<any>[] = [];

  for (const wf of workflowRegistry) {
    const hasCancelable = wf.steps.some(s => !s.noCancel && !s.terminal);

    for (const s of wf.steps) {
      const toolName = `${wf.name}.${s.name}`;

      const inputSchema: any = s.args
        ? zodToJsonSchema(s.args, { target: 'openApi3' })
        : { type: 'object', properties: {} };

      // Capture for closure
      const wfName = wf.name;
      const sName = s.name;

      defs.push({
        name: toolName,
        description: s.description || `${wf.name} ${s.name}`,
        inputSchemaJson: JSON.stringify(inputSchema),
        outputSchemaJson: '',
        title: '',
        destructiveHint: false,
        idempotentHint: false,
        readOnlyHint: false,
        openWorldHint: false,
        taskSupport: false,
        hidden: !s.initial,
        handler: (args: any, ctx: ToolContext) => handleStepCall(wfName, sName, args, ctx),
      });
    }

    if (hasCancelable) {
      const cancelName = `${wf.name}.cancel`;
      const wfName = wf.name;

      defs.push({
        name: cancelName,
        description: `Cancel the ${wf.name} workflow`,
        inputSchemaJson: JSON.stringify({ type: 'object', properties: {} }),
        outputSchemaJson: '',
        title: '',
        destructiveHint: false,
        idempotentHint: false,
        readOnlyHint: false,
        openWorldHint: false,
        taskSupport: false,
        hidden: true,
        handler: (_args: any, _ctx: ToolContext) => handleCancel(wfName),
      });
    }
  }

  return defs;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export function getRegisteredWorkflows(): WorkflowDef[] {
  return [...workflowRegistry];
}

export function clearWorkflowRegistry(): void {
  workflowRegistry.length = 0;
  activeWorkflowStack.length = 0;
}

// Register with tool.ts so getRegisteredTools() includes workflow defs.
_setWorkflowsToToolDefs(workflowsToToolDefs);
