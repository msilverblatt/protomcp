import { describe, it, expect, beforeEach } from 'vitest';
import { z } from 'zod';
import { toolGroup, getRegisteredGroups, clearGroupRegistry, groupsToToolDefs } from '../src/group.js';
import { getRegisteredTools, clearRegistry } from '../src/tool.js';
import { ToolContext } from '../src/context.js';

function dummyCtx(): ToolContext {
  return new ToolContext('', () => {});
}

beforeEach(() => {
  clearGroupRegistry();
  clearRegistry();
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
