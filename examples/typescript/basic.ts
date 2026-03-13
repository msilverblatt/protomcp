// examples/typescript/basic.ts
// A minimal protomcp tool — adds two numbers.
// Run: pmcp dev examples/typescript/basic.ts

import { tool, ToolResult } from 'protomcp';
import { z } from 'zod';

tool({
  name: 'add',
  description: 'Add two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a + b) });
  },
});

tool({
  name: 'multiply',
  description: 'Multiply two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a * b) });
  },
});
