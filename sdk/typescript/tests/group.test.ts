import { describe, it, expect, beforeEach } from 'vitest';
import { z } from 'zod';
import { toolGroup, getRegisteredGroups, clearGroupRegistry, groupsToToolDefs } from '../src/group.js';
import { getRegisteredTools, clearRegistry } from '../src/tool.js';
import { ToolContext } from '../src/context.js';
import { ToolResult } from '../src/result.js';
import { clearContextRegistry } from '../src/serverContext.js';

function dummyCtx(): ToolContext {
  return new ToolContext('', () => {});
}

beforeEach(() => {
  clearGroupRegistry();
  clearRegistry();
  clearContextRegistry();
});

describe('toolGroup', () => {
  it('registers a group', () => {
    toolGroup({
      name: 'math',
      description: 'Math operations',
      actions: {
        add: {
          description: 'Add two numbers',
          args: z.object({ a: z.number(), b: z.number() }),
          handler: (args) => args.a + args.b,
        },
        multiply: {
          description: 'Multiply two numbers',
          args: z.object({ x: z.number(), y: z.number() }),
          handler: (args) => args.x * args.y,
        },
      },
    });

    const groups = getRegisteredGroups();
    expect(groups).toHaveLength(1);
    expect(groups[0].name).toBe('math');
    expect(groups[0].description).toBe('Math operations');
    expect(Object.keys(groups[0].actions)).toHaveLength(2);
  });

  it('generates union strategy schema', () => {
    toolGroup({
      name: 'db',
      description: 'DB ops',
      actions: {
        query: {
          description: 'Run query',
          args: z.object({ sql: z.string() }),
          handler: (args) => args.sql,
        },
        insert: {
          description: 'Insert record',
          args: z.object({ table: z.string(), data: z.object({}) }),
          handler: () => 'ok',
        },
      },
    });

    const defs = groupsToToolDefs();
    expect(defs).toHaveLength(1);
    const td = defs[0];
    expect(td.name).toBe('db');
    const schema = JSON.parse(td.inputSchemaJson);
    expect(new Set(schema.properties.action.enum)).toEqual(new Set(['query', 'insert']));
    expect(schema.oneOf).toHaveLength(2);

    const entries: Record<string, any> = {};
    for (const e of schema.oneOf) {
      entries[e.properties.action.const] = e;
    }
    expect(entries.query.properties.sql).toBeDefined();
    expect(entries.insert.properties.table).toBeDefined();
    expect(entries.insert.properties.data).toBeDefined();
  });

  it('generates separate strategy schema', () => {
    toolGroup({
      name: 'files',
      description: 'File ops',
      strategy: 'separate',
      actions: {
        read: {
          description: 'Read a file',
          args: z.object({ path: z.string() }),
          handler: (args) => args.path,
        },
        write: {
          description: 'Write a file',
          args: z.object({ path: z.string(), content: z.string() }),
          handler: () => 'ok',
        },
      },
    });

    const defs = groupsToToolDefs();
    expect(defs).toHaveLength(2);
    const names = defs.map((d) => d.name);
    expect(names).toContain('files.read');
    expect(names).toContain('files.write');

    const readDef = defs.find((d) => d.name === 'files.read')!;
    const readSchema = JSON.parse(readDef.inputSchemaJson);
    expect(readSchema.properties.path).toBeDefined();
    expect(readSchema.properties.action).toBeUndefined();
  });

  it('dispatches correct action via union handler', () => {
    toolGroup({
      name: 'calc',
      description: 'Calculator',
      actions: {
        add: {
          description: 'Add',
          args: z.object({ a: z.number(), b: z.number() }),
          handler: (args) => args.a + args.b,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'add', a: 3, b: 4 }, dummyCtx());
    expect(result).toBe(7);
  });

  it('returns error for unknown action', () => {
    toolGroup({
      name: 'calc2',
      description: 'Calculator',
      actions: {
        add: {
          description: 'Add',
          args: z.object({ a: z.number(), b: z.number() }),
          handler: (args) => args.a + args.b,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'ad' }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('Unknown action');
    expect(result.result).toContain('add');
  });

  it('returns error for missing action', () => {
    toolGroup({
      name: 'calc3',
      description: 'Calculator',
      actions: {
        add: {
          description: 'Add',
          args: z.object({ a: z.number() }),
          handler: (args) => args.a,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({}, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.result).toContain('Missing');
  });

  it('groups appear in getRegisteredTools', () => {
    toolGroup({
      name: 'tools_test',
      description: 'Test group',
      actions: {
        ping: {
          description: 'Ping',
          args: z.object({}),
          handler: () => 'pong',
        },
      },
    });

    const tools = getRegisteredTools();
    const names = tools.map((t) => t.name);
    expect(names).toContain('tools_test');
  });

  it('dispatches separate strategy handlers', () => {
    toolGroup({
      name: 'sep_test',
      description: 'Sep test',
      strategy: 'separate',
      actions: {
        echo: {
          description: 'Echo',
          args: z.object({ msg: z.string() }),
          handler: (args) => args.msg,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ msg: 'hi' }, dummyCtx());
    expect(result).toBe('hi');
  });
});

describe('declarative validation', () => {
  it('validates requires fields', () => {
    toolGroup({
      name: 'val_req',
      description: 'Validation test',
      actions: {
        doIt: {
          description: 'Do something',
          args: z.object({ name: z.string().optional() }),
          requires: ['name'],
          handler: (args) => args.name,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'doIt' }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.errorCode).toBe('MISSING_REQUIRED');
    expect(result.result).toContain('name');
  });

  it('validates enum fields with fuzzy suggestion', () => {
    toolGroup({
      name: 'val_enum',
      description: 'Enum test',
      actions: {
        setColor: {
          description: 'Set color',
          args: z.object({ color: z.string() }),
          enumFields: { color: ['red', 'green', 'blue'] },
          handler: (args) => args.color,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'setColor', color: 'rad' }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.errorCode).toBe('INVALID_ENUM');
    expect(result.result).toContain('rad');
    expect(result.suggestion).toContain('red');
  });

  it('allows valid enum values', () => {
    toolGroup({
      name: 'val_enum_ok',
      description: 'Enum ok test',
      actions: {
        setColor: {
          description: 'Set color',
          args: z.object({ color: z.string() }),
          enumFields: { color: ['red', 'green', 'blue'] },
          handler: (args) => args.color,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'setColor', color: 'red' }, dummyCtx());
    expect(result).toBe('red');
  });

  it('validates cross rules', () => {
    toolGroup({
      name: 'val_cross',
      description: 'Cross rules test',
      actions: {
        range: {
          description: 'Set range',
          args: z.object({ min: z.number(), max: z.number() }),
          crossRules: [
            { condition: (args) => args.min > args.max, message: 'min must be <= max' },
          ],
          handler: (args) => `${args.min}-${args.max}`,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'range', min: 10, max: 5 }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.isError).toBe(true);
    expect(result.errorCode).toBe('CROSS_PARAM_VIOLATION');
    expect(result.result).toContain('min must be <= max');
  });

  it('passes cross rules when valid', () => {
    toolGroup({
      name: 'val_cross_ok',
      description: 'Cross ok',
      actions: {
        range: {
          description: 'Set range',
          args: z.object({ min: z.number(), max: z.number() }),
          crossRules: [
            { condition: (args) => args.min > args.max, message: 'min must be <= max' },
          ],
          handler: (args) => `${args.min}-${args.max}`,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'range', min: 1, max: 10 }, dummyCtx());
    expect(result).toBe('1-10');
  });

  it('collects hints and appends to result', () => {
    toolGroup({
      name: 'val_hints',
      description: 'Hints test',
      actions: {
        deploy: {
          description: 'Deploy',
          args: z.object({ env: z.string() }),
          hints: {
            prodWarning: {
              condition: (args) => args.env === 'production',
              message: 'Deploying to production - be careful!',
            },
          },
          handler: (args) => `deployed to ${args.env}`,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'deploy', env: 'production' }, dummyCtx());
    expect(result).toBeInstanceOf(ToolResult);
    expect(result.result).toContain('deployed to production');
    expect(result.result).toContain('Deploying to production');
  });

  it('does not append hints when no conditions met', () => {
    toolGroup({
      name: 'val_hints_none',
      description: 'No hints',
      actions: {
        deploy: {
          description: 'Deploy',
          args: z.object({ env: z.string() }),
          hints: {
            prodWarning: {
              condition: (args) => args.env === 'production',
              message: 'Deploying to production',
            },
          },
          handler: (args) => `deployed to ${args.env}`,
        },
      },
    });

    const defs = groupsToToolDefs();
    const result = defs[0].handler({ action: 'deploy', env: 'staging' }, dummyCtx());
    expect(result).toBe('deployed to staging');
  });
});
