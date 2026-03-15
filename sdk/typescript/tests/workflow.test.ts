import { describe, it, expect, beforeEach, vi } from 'vitest';
import { z } from 'zod';
import { workflow, StepResult, getRegisteredWorkflows, clearWorkflowRegistry } from '../src/workflow.js';
import { getRegisteredTools, clearRegistry } from '../src/tool.js';
import { ToolContext } from '../src/context.js';
import { ToolResult } from '../src/result.js';
import { toolManager } from '../src/manager.js';

function dummyCtx(): ToolContext {
  return new ToolContext('', () => {});
}

beforeEach(() => {
  clearWorkflowRegistry();
  clearRegistry();
});

// ---------------------------------------------------------------------------
// Registration & graph validation
// ---------------------------------------------------------------------------

describe('workflow registration', () => {
  it('registers a simple workflow', () => {
    workflow({
      name: 'deploy',
      description: 'Deploy workflow',
      steps: {
        start: {
          description: 'Start deploy',
          initial: true,
          next: ['finish'],
          handler: () => 'started',
        },
        finish: {
          description: 'Finish deploy',
          terminal: true,
          handler: () => 'done',
        },
      },
    });

    const wfs = getRegisteredWorkflows();
    expect(wfs).toHaveLength(1);
    expect(wfs[0].name).toBe('deploy');
    expect(wfs[0].steps).toHaveLength(2);
  });

  it('throws if no initial step', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: { terminal: true, handler: () => 'x' },
        },
      });
    }).toThrow('no initial step');
  });

  it('throws if multiple initial steps', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: { initial: true, next: ['b'], handler: () => 'x' },
          b: { initial: true, terminal: true, handler: () => 'x' },
        },
      });
    }).toThrow('multiple initial steps');
  });

  it('throws if terminal step has next', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: { initial: true, terminal: true, next: ['a'], handler: () => 'x' },
        },
      });
    }).toThrow("terminal step 'a' has next");
  });

  it('throws if non-terminal step has no next (dead end)', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: { initial: true, handler: () => 'x' },
        },
      });
    }).toThrow('dead end');
  });

  it('throws if next references nonexistent step', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: { initial: true, next: ['ghost'], handler: () => 'x' },
        },
      });
    }).toThrow("nonexistent step 'ghost'");
  });

  it('throws if onError references nonexistent step', () => {
    expect(() => {
      workflow({
        name: 'bad',
        steps: {
          a: {
            initial: true,
            next: ['b'],
            onError: { 'fail': 'ghost' },
            handler: () => 'x',
          },
          b: { terminal: true, handler: () => 'x' },
        },
      });
    }).toThrow("onError references nonexistent step 'ghost'");
  });
});

// ---------------------------------------------------------------------------
// Tool generation
// ---------------------------------------------------------------------------

describe('tool generation', () => {
  it('generates step tools and cancel tool', () => {
    workflow({
      name: 'wf',
      steps: {
        begin: {
          description: 'Begin',
          initial: true,
          next: ['end'],
          handler: () => 'ok',
        },
        end: {
          description: 'End',
          terminal: true,
          handler: () => 'done',
        },
      },
    });

    const tools = getRegisteredTools();
    const names = tools.map(t => t.name);
    expect(names).toContain('wf.begin');
    expect(names).toContain('wf.end');
    expect(names).toContain('wf.cancel');
  });

  it('initial step is not hidden, others are hidden', () => {
    workflow({
      name: 'wf2',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['done'],
          handler: () => 'ok',
        },
        done: {
          description: 'Done',
          terminal: true,
          handler: () => 'ok',
        },
      },
    });

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'wf2.start')!;
    const done = tools.find(t => t.name === 'wf2.done')!;
    const cancel = tools.find(t => t.name === 'wf2.cancel')!;

    expect(start.hidden).toBe(false);
    expect(done.hidden).toBe(true);
    expect(cancel.hidden).toBe(true);
  });

  it('no cancel tool when all steps have noCancel', () => {
    workflow({
      name: 'nocancel',
      steps: {
        begin: {
          description: 'Begin',
          initial: true,
          noCancel: true,
          next: ['end'],
          handler: () => 'ok',
        },
        end: {
          description: 'End',
          terminal: true,
          noCancel: true,
          handler: () => 'done',
        },
      },
    });

    const tools = getRegisteredTools();
    const names = tools.map(t => t.name);
    expect(names).not.toContain('nocancel.cancel');
  });

  it('generates schema from zod args', () => {
    workflow({
      name: 'argwf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          terminal: true,
          args: z.object({ message: z.string() }),
          handler: (args) => args.message,
        },
      },
    });

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'argwf.start')!;
    const schema = JSON.parse(start.inputSchemaJson);
    expect(schema.properties.message).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// Step dispatch
// ---------------------------------------------------------------------------

describe('step dispatch', () => {
  it('calls initial step handler and returns result', async () => {
    workflow({
      name: 'test',
      steps: {
        init: {
          description: 'Init',
          initial: true,
          terminal: true,
          handler: () => 'hello from init',
        },
      },
    });

    // Mock toolManager since it requires transport
    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const initTool = tools.find(t => t.name === 'test.init')!;
    const result = await initTool.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.result).toBe('hello from init');
    expect(result.isError).toBe(false);
  });

  it('passes args to step handler', async () => {
    workflow({
      name: 'argtest',
      steps: {
        step1: {
          description: 'Step 1',
          initial: true,
          terminal: true,
          args: z.object({ name: z.string() }),
          handler: (args) => `Hello ${args.name}`,
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const step1 = tools.find(t => t.name === 'argtest.step1')!;
    const result = await step1.handler({ name: 'World' }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.result).toBe('Hello World');
  });

  it('returns error for non-initial step without active workflow', async () => {
    workflow({
      name: 'wf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['middle'],
          handler: () => 'ok',
        },
        middle: {
          description: 'Middle',
          next: ['end'],
          handler: () => 'ok',
        },
        end: {
          description: 'End',
          terminal: true,
          handler: () => 'done',
        },
      },
    });

    const tools = getRegisteredTools();
    const middle = tools.find(t => t.name === 'wf.middle')!;
    const result = await middle.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('No active workflow');
  });

  it('handles StepResult with dynamic next narrowing', async () => {
    workflow({
      name: 'dynwf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['pathA', 'pathB'],
          handler: () => new StepResult('choosing A', ['pathA']),
        },
        pathA: {
          description: 'Path A',
          terminal: true,
          handler: () => 'A done',
        },
        pathB: {
          description: 'Path B',
          terminal: true,
          handler: () => 'B done',
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'dynwf.start')!;
    const result = await start.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.result).toBe('choosing A');
  });

  it('rejects dynamic next with steps not in declared next', async () => {
    workflow({
      name: 'badnext',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['end'],
          handler: () => new StepResult('bad', ['ghost']),
        },
        end: {
          description: 'End',
          terminal: true,
          handler: () => 'done',
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'badnext.start')!;
    const result = await start.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('invalid next steps');
  });

  it('rejects dynamic next on a step with no declared next', async () => {
    workflow({
      name: 'nodeclared',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          terminal: true,
          handler: () => new StepResult('bad', ['something']),
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'nodeclared.start')!;
    const result = await start.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('has no declared next');
  });
});

// ---------------------------------------------------------------------------
// Error handling
// ---------------------------------------------------------------------------

describe('error handling', () => {
  it('stays in state on unmatched error for retry', async () => {
    workflow({
      name: 'errwf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          terminal: true,
          handler: () => { throw new Error('boom'); },
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'errwf.start')!;
    const result = await start.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('boom');
    expect(result.result).toContain('retry');
  });

  it('transitions to error target step on matched error substring', async () => {
    workflow({
      name: 'errmap',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['recovery'],
          onError: { 'not found': 'recovery' },
          handler: () => { throw new Error('item not found'); },
        },
        recovery: {
          description: 'Recovery',
          terminal: true,
          handler: () => 'recovered',
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'errmap.start')!;
    const result = await start.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(false);
    expect(result.result).toContain('transitioning');
    expect(result.result).toContain('recovery');
  });
});

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

describe('cancel', () => {
  it('cancels an active workflow', async () => {
    const cancelSpy = vi.fn();

    workflow({
      name: 'cancelwf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['end'],
          handler: () => 'started',
        },
        end: {
          description: 'End',
          terminal: true,
          handler: () => 'done',
        },
      },
      onCancel: cancelSpy,
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const startTool = tools.find(t => t.name === 'cancelwf.start')!;
    const cancelTool = tools.find(t => t.name === 'cancelwf.cancel')!;

    // Start the workflow
    await startTool.handler({}, dummyCtx());

    // Cancel it
    const result = cancelTool.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.result).toContain('cancelled');
    expect(cancelSpy).toHaveBeenCalled();
  });

  it('returns error when cancelling without active workflow', () => {
    workflow({
      name: 'cancelwf2',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          next: ['end'],
          handler: () => 'ok',
        },
        end: {
          description: 'End',
          terminal: true,
          handler: () => 'done',
        },
      },
    });

    const tools = getRegisteredTools();
    const cancelTool = tools.find(t => t.name === 'cancelwf2.cancel')!;
    const result = cancelTool.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('No active workflow');
  });
});

// ---------------------------------------------------------------------------
// onComplete callback
// ---------------------------------------------------------------------------

describe('onComplete', () => {
  it('calls onComplete when terminal step completes', async () => {
    const completeSpy = vi.fn();

    workflow({
      name: 'completewf',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          terminal: true,
          handler: () => 'finished',
        },
      },
      onComplete: completeSpy,
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const start = tools.find(t => t.name === 'completewf.start')!;
    await start.handler({}, dummyCtx());
    expect(completeSpy).toHaveBeenCalledTimes(1);
    // Called with history array
    expect(completeSpy.mock.calls[0][0]).toHaveLength(1);
    expect(completeSpy.mock.calls[0][0][0].step).toBe('start');
  });
});

// ---------------------------------------------------------------------------
// clearWorkflowRegistry
// ---------------------------------------------------------------------------

describe('clearWorkflowRegistry', () => {
  it('clears all workflows', () => {
    workflow({
      name: 'tmp',
      steps: {
        a: { initial: true, terminal: true, handler: () => 'x' },
      },
    });
    expect(getRegisteredWorkflows()).toHaveLength(1);
    clearWorkflowRegistry();
    expect(getRegisteredWorkflows()).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Multi-step workflow
// ---------------------------------------------------------------------------

describe('multi-step workflow', () => {
  it('walks through multiple steps', async () => {
    const log: string[] = [];

    workflow({
      name: 'multi',
      steps: {
        step1: {
          description: 'Step 1',
          initial: true,
          next: ['step2'],
          handler: () => { log.push('step1'); return 'step1 done'; },
        },
        step2: {
          description: 'Step 2',
          next: ['step3'],
          handler: () => { log.push('step2'); return 'step2 done'; },
        },
        step3: {
          description: 'Step 3',
          terminal: true,
          handler: () => { log.push('step3'); return 'step3 done'; },
        },
      },
    });

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const step1 = tools.find(t => t.name === 'multi.step1')!;
    const step2 = tools.find(t => t.name === 'multi.step2')!;
    const step3 = tools.find(t => t.name === 'multi.step3')!;

    const r1 = await step1.handler({}, dummyCtx());
    expect(r1.result).toBe('step1 done');

    const r2 = await step2.handler({}, dummyCtx());
    expect(r2.result).toBe('step2 done');

    const r3 = await step3.handler({}, dummyCtx());
    expect(r3.result).toBe('step3 done');

    expect(log).toEqual(['step1', 'step2', 'step3']);
  });
});
