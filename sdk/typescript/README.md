# protomcp

[![CI](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![npm](https://img.shields.io/npm/v/protomcp)](https://www.npmjs.com/package/protomcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/msilverblatt/protomcp/blob/main/LICENSE)

TypeScript SDK for [protomcp](https://github.com/msilverblatt/protomcp) -- build MCP servers with tools, resources, and prompts in one file, one command.

## Install

```sh
npm install protomcp
```

You also need the `pmcp` CLI:

```sh
brew install msilverblatt/tap/protomcp
```

## Quick Start

```typescript
// server.ts
import { tool, resource, prompt, ToolResult } from 'protomcp';
import { z } from 'zod';

tool({
  name: 'add',
  description: 'Add two numbers',
  args: z.object({ a: z.number(), b: z.number() }),
  handler: ({ a, b }) => new ToolResult({ result: String(a + b) }),
});

resource({
  uri: 'config://app',
  description: 'App configuration',
  handler: (uri) => ({ uri, text: '{"debug": false, "version": "2.1"}' }),
});

prompt({
  name: 'explain',
  description: 'Explain a concept',
  arguments: [{ name: 'topic', required: true }],
  handler: (args) => [{ role: 'user', content: `Explain ${args.topic} in simple terms` }],
});
```

```sh
pmcp dev server.ts
```

## Tool Groups

Group related actions under a single tool with per-action schemas:

```typescript
import { toolGroup, ToolResult } from 'protomcp';
import { z } from 'zod';

toolGroup({
  name: 'math',
  description: 'Math operations',
  actions: {
    add: {
      description: 'Add two numbers',
      args: z.object({ a: z.number(), b: z.number() }),
      handler: ({ a, b }) => new ToolResult({ result: String(a + b) }),
    },
    multiply: {
      description: 'Multiply two numbers',
      args: z.object({ a: z.number(), b: z.number() }),
      handler: ({ a, b }) => new ToolResult({ result: String(a * b) }),
    },
  },
});
```

## Documentation

- [Full documentation](https://msilverblatt.github.io/protomcp/)
- [TypeScript Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-typescript/)
- [CLI Reference](https://msilverblatt.github.io/protomcp/reference/cli/)
- [Examples](https://github.com/msilverblatt/protomcp/tree/main/examples/typescript)

## License

MIT
