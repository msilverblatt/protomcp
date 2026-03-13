import { describe, it, expect, beforeEach } from 'vitest';
import { z } from 'zod';
import { tool, getRegisteredTools, clearRegistry } from '../src/tool.js';

describe('tool()', () => {
  beforeEach(() => clearRegistry());

  it('registers a tool with Zod schema', () => {
    const add = tool({
      name: 'add',
      description: 'Add two numbers',
      args: z.object({
        a: z.number().describe('First number'),
        b: z.number().describe('Second number'),
      }),
      handler: (args) => args.a + args.b,
    });

    const tools = getRegisteredTools();
    expect(tools).toHaveLength(1);
    expect(tools[0].name).toBe('add');
    expect(tools[0].description).toBe('Add two numbers');
  });

  it('requires name for arrow function handlers', () => {
    const t = tool({
      description: 'No name',
      args: z.object({ x: z.number() }),
      handler: (args) => args.x,
    });
    // Arrow functions have empty .name, so falls back to tool_N
    expect(t.name).toMatch(/^tool_\d+$/);
  });

  it('generates JSON Schema from Zod', () => {
    tool({
      name: 'search',
      description: 'Search',
      args: z.object({
        query: z.string().describe('Search query'),
        limit: z.number().default(10).describe('Max results'),
      }),
      handler: (args) => [],
    });

    const tools = getRegisteredTools();
    const schema = JSON.parse(tools[0].inputSchemaJson);
    expect(schema.type).toBe('object');
    expect(schema.properties.query.type).toBe('string');
    expect(schema.properties.limit.default).toBe(10);
    expect(schema.required).toContain('query');
    expect(schema.required).not.toContain('limit');
  });

  it('handler is callable', () => {
    const add = tool({
      description: 'Add',
      args: z.object({ a: z.number(), b: z.number() }),
      handler: (args) => args.a + args.b,
    });

    expect(add.handler({ a: 2, b: 3 })).toBe(5);
  });

  it('generates output schema from Zod', () => {
    const t = tool({
      name: 'search',
      description: 'Search',
      args: z.object({ q: z.string() }),
      output: z.object({ title: z.string(), score: z.number() }),
      handler: (args) => ({ title: 'test', score: 0.9 }),
    });
    const tools = getRegisteredTools();
    const def = tools[tools.length - 1];
    expect(def.outputSchemaJson).toBeTruthy();
    const schema = JSON.parse(def.outputSchemaJson);
    expect(schema.properties.title).toBeTruthy();
  });

  it('includes tool metadata', () => {
    const t = tool({
      name: 'delete_doc',
      description: 'Delete',
      args: z.object({ id: z.string() }),
      title: 'Delete Document',
      destructiveHint: true,
      handler: (args) => 'deleted',
    });
    const tools = getRegisteredTools();
    const def = tools[tools.length - 1];
    expect(def.title).toBe('Delete Document');
    expect(def.destructiveHint).toBe(true);
  });
});
