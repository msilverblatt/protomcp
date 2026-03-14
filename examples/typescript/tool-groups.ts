// examples/typescript/tool-groups.ts
// Demonstrates tool groups with per-action Zod schemas, validation, and strategies.
// Run: pmcp dev examples/typescript/tool-groups.ts

import { toolGroup, ToolResult } from 'protomcp';
import { z } from 'zod';

// Union strategy (default): single tool "db" with action discriminator
toolGroup({
  name: 'db',
  description: 'Database operations',
  actions: {
    query: {
      description: 'Run a read-only SQL query',
      args: z.object({ sql: z.string(), limit: z.number().default(100) }),
      requires: ['sql'],
      handler({ sql, limit }) {
        return new ToolResult({ result: `Executed: ${sql} (limit ${limit})` });
      },
    },
    insert: {
      description: 'Insert a record into a table',
      args: z.object({ table: z.string(), data: z.string() }),
      requires: ['table', 'data'],
      enumFields: { table: ['users', 'events', 'logs'] },
      handler({ table, data }) {
        return new ToolResult({ result: `Inserted into ${table}: ${data}` });
      },
    },
    migrate: {
      description: 'Run a schema migration',
      args: z.object({ version: z.string(), dry_run: z.boolean().default(false) }),
      requires: ['version'],
      handler({ version, dry_run }) {
        const mode = dry_run ? 'dry run' : 'applied';
        return new ToolResult({ result: `Migration ${version} ${mode}` });
      },
    },
  },
});

// Separate strategy: each action becomes its own tool (files.read, files.write, etc.)
toolGroup({
  name: 'files',
  description: 'File operations',
  strategy: 'separate',
  actions: {
    read: {
      description: 'Read a file by path',
      args: z.object({ path: z.string() }),
      requires: ['path'],
      handler({ path }) {
        return new ToolResult({ result: `Contents of ${path}` });
      },
    },
    write: {
      description: 'Write content to a file',
      args: z.object({ path: z.string(), content: z.string() }),
      requires: ['path', 'content'],
      handler({ path, content }) {
        return new ToolResult({ result: `Wrote ${content.length} bytes to ${path}` });
      },
    },
    search: {
      description: 'Search files by pattern',
      args: z.object({
        pattern: z.string(),
        scope: z.enum(['workspace', 'project', 'global']).default('workspace'),
      }),
      requires: ['pattern'],
      enumFields: { scope: ['workspace', 'project', 'global'] },
      handler({ pattern, scope }) {
        return new ToolResult({ result: `Searching '${pattern}' in ${scope}` });
      },
    },
  },
});
