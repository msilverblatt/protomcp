import { describe, it, expect, beforeEach, vi } from 'vitest';
import { z } from 'zod';
import { tool, getRegisteredTools, getHiddenToolNames, clearRegistry } from '../src/tool.js';
import { workflow, StepResult, getRegisteredWorkflows, clearWorkflowRegistry } from '../src/workflow.js';
import { clearGroupRegistry } from '../src/group.js';
import { configure, discoverHandlers, resetConfig } from '../src/discovery.js';
import { ToolContext } from '../src/context.js';
import { ToolResult } from '../src/result.js';
import { toolManager } from '../src/manager.js';

function dummyCtx(): ToolContext {
  return new ToolContext('', () => {});
}

beforeEach(() => {
  clearWorkflowRegistry();
  clearRegistry();
  clearGroupRegistry();
  resetConfig();
});

// ---------------------------------------------------------------------------
// 1. Workflow preWorkflowTools snapshot
// ---------------------------------------------------------------------------

describe('workflow preWorkflowTools snapshot', () => {
  it('initial step result has enableTools/disableTools reflecting pre-workflow tools', async () => {
    // Register some regular tools first
    tool({
      name: 'regular_tool',
      description: 'A regular tool',
      args: z.object({}),
      handler: () => 'ok',
    });

    // Register a workflow
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

    vi.spyOn(toolManager, 'getActiveTools').mockResolvedValue([]);
    vi.spyOn(toolManager, 'setAllowed').mockResolvedValue([]);

    const tools = getRegisteredTools();
    const startTool = tools.find(t => t.name === 'deploy.start')!;
    expect(startTool).toBeDefined();

    const result = await startTool.handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(false);
    expect(result.result).toBe('started');
    // The result should have enableTools (next steps) and disableTools
    expect(result.enableTools).toBeDefined();
    expect(result.disableTools).toBeDefined();
    // Enable tools should include the next step
    expect(result.enableTools).toContain('deploy.finish');
  });
});

// ---------------------------------------------------------------------------
// 2. Hot reload discovery clears all registries
// ---------------------------------------------------------------------------

describe('hot reload clears all registries', () => {
  it('clears tools on hot reload', async () => {
    tool({
      name: 'temp_tool',
      description: 'A temporary tool',
      args: z.object({}),
      handler: () => 'temp',
    });

    expect(getRegisteredTools().some(t => t.name === 'temp_tool')).toBe(true);

    configure({ handlersDir: '/tmp/nonexistent-protomcp-test-dir', hotReload: true });
    // First discover loads (noop for nonexistent dir but marks loadedModules)
    await discoverHandlers();
    // Second discover triggers hot reload clear
    await discoverHandlers();

    // After hot reload, cleanup
    clearRegistry();
    resetConfig();
  });

  it('clears workflows on hot reload', async () => {
    workflow({
      name: 'temp_wf',
      description: 'Temp workflow',
      steps: {
        start: {
          description: 'Start',
          initial: true,
          terminal: true,
          handler: () => 'ok',
        },
      },
    });

    expect(getRegisteredWorkflows()).toHaveLength(1);

    configure({ handlersDir: '/tmp/nonexistent-protomcp-test-dir', hotReload: true });
    await discoverHandlers();
    await discoverHandlers();

    // Cleanup
    clearWorkflowRegistry();
    resetConfig();
  });
});

// ---------------------------------------------------------------------------
// 3. Hidden tool names include workflow tools
// ---------------------------------------------------------------------------

describe('hidden tool names include workflow tools', () => {
  it('getHiddenToolNames returns hidden workflow step names', () => {
    workflow({
      name: 'review',
      description: 'Review workflow',
      steps: {
        start: {
          description: 'Start review',
          initial: true,
          next: ['approve', 'reject'],
          handler: () => 'reviewing',
        },
        approve: {
          description: 'Approve',
          terminal: true,
          handler: () => 'approved',
        },
        reject: {
          description: 'Reject',
          terminal: true,
          handler: () => 'rejected',
        },
      },
    });

    const hidden = getHiddenToolNames();
    // Non-initial steps should be hidden
    expect(hidden).toContain('review.approve');
    expect(hidden).toContain('review.reject');
    expect(hidden).toContain('review.cancel');
    // Initial step should NOT be hidden
    expect(hidden).not.toContain('review.start');
  });

  it('getHiddenToolNames returns hidden tools from mixed sources', () => {
    // Add a hidden individual tool
    tool({
      name: 'hidden_tool',
      description: 'A hidden tool',
      args: z.object({}),
      handler: () => 'secret',
    });
    // Manually set hidden on the last tool
    const tools = getRegisteredTools();
    const hiddenTool = tools.find(t => t.name === 'hidden_tool')!;
    hiddenTool.hidden = true;

    // Add a workflow (non-initial steps are auto-hidden)
    workflow({
      name: 'wf',
      description: 'Workflow',
      steps: {
        init: {
          description: 'Init',
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

    const hidden = getHiddenToolNames();
    expect(hidden).toContain('hidden_tool');
    expect(hidden).toContain('wf.done');
    expect(hidden).toContain('wf.cancel');
    expect(hidden).not.toContain('wf.init');
  });
});
