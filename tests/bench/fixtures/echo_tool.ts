// Benchmark tool fixture for protomcp — TypeScript implementation.
// Provides echo, add, compute, generate, parse_json tools.

import { tool, ToolResult, run } from 'protomcp';
import { z } from 'zod';
import { createHash } from 'crypto';

tool({
  name: 'echo',
  description: 'Echo the input back',
  args: z.object({ message: z.string() }),
  handler({ message }) {
    return new ToolResult({ result: message });
  },
});

tool({
  name: 'add',
  description: 'Add two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler({ a, b }) {
    return new ToolResult({ result: String(a + b) });
  },
});

tool({
  name: 'compute',
  description: 'CPU-bound work: hash a string N times',
  args: z.object({ iterations: z.number() }),
  handler({ iterations }) {
    let result = 'seed';
    for (let i = 0; i < iterations; i++) {
      result = createHash('sha256').update(result).digest('hex');
    }
    return new ToolResult({ result });
  },
});

tool({
  name: 'generate',
  description: 'Return a string of the requested size in bytes',
  args: z.object({ size: z.number() }),
  handler({ size }) {
    return new ToolResult({ result: 'X'.repeat(size) });
  },
});

tool({
  name: 'parse_json',
  description: 'Parse JSON and return it serialized back',
  args: z.object({ data: z.string() }),
  handler({ data }) {
    const parsed = JSON.parse(data);
    return new ToolResult({ result: JSON.stringify(parsed) });
  },
});

run();
