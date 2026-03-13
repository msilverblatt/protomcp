# protomcp

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)

**Write MCP tools in any language. One file, one command, hot-reload.**

Building MCP tools today means wrestling with protocol boilerplate, restarting your AI host every time you change a line, and being locked into one language per SDK. protomcp eliminates all of that — you write a handler function, run `pmcp dev tools.py`, and your tools are live in Claude Desktop, Cursor, or any MCP client. Change your code, save the file, and it reloads instantly.

## How It Works

```
┌─────────────┐         ┌──────────────┐         ┌──────────────┐
│             │  MCP     │              │ protobuf │              │
│  MCP Host   │◄───────►│    pmcp      │◄────────►│  Your Code   │
│  (Claude,   │ JSON-RPC │   (Go)      │  unix    │  (any lang)  │
│   Cursor…)  │  stdio   │             │  socket  │              │
└─────────────┘         └──────────────┘         └──────────────┘
```

pmcp sits between your MCP host and your tool process. It speaks MCP (JSON-RPC over stdio) on one side and a simple protobuf protocol over a unix socket on the other. Your tool process registers handlers, and pmcp handles everything else: listing tools, routing calls, hot reload, and dynamic tool management.

## Quick Start

### Install

```sh
brew install protomcp/tap/protomcp
```

### Python

```python
# tools.py
from protomcp import tool, ToolResult

@tool("Add two numbers")
def add(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a + b))
```

```sh
pmcp dev tools.py
```

### TypeScript

```typescript
// tools.ts
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
```

```sh
pmcp dev tools.ts
```

Then add either `pmcp dev` command to your MCP client config. That's it.

**[See it in action →](https://msilverblatt.github.io/protomcp/demo/)**

## Features

- **Any Language** — write tools in Python, TypeScript, Go, Rust, or any language that speaks protobuf over a unix socket
- **Hot Reload** — save your file and tools reload instantly, no restart needed
- **Dynamic Tool Lists** — tools can enable/disable themselves at runtime based on context
- **5 Transports** — stdio, SSE, streamable HTTP, WebSocket, gRPC
- **Structured Output** — define output schemas for typed tool results
- **Async Tasks** — long-running operations with background task tracking
- **Progress & Cancellation** — report progress and respond to cancellation requests
- **Server Logging** — 8 RFC 5424 log levels forwarded to the MCP host
- **Tool Metadata** — annotations for destructive, read-only, idempotent, and open-world hints

## Comparison

| Feature | pmcp | FastMCP (Python) | MCP SDKs |
|---------|------|------------------|----------|
| Language support | Any (protobuf) | Python only | One SDK per language |
| Hot reload | Built-in | No | No |
| Dynamic tool lists | Built-in | No | Manual |
| Transports | stdio, SSE, HTTP, WS, gRPC | stdio, SSE | Varies by SDK |
| Structured output | Yes | No | Varies |
| Async tasks | Yes | No | No |
| Single binary | Yes (Go) | No (Python runtime) | No (per-language) |

## Examples

See [`examples/`](examples/) for runnable demos at three levels:

- **Basic** — minimal single-tool examples in Python and TypeScript
- **Real-world** — file search tool with progress reporting, cancellation, and logging
- **Full showcase** — multi-tool server with structured output, dynamic tool lists, error handling, and metadata

Run all demos: `./examples/run-demo.sh`

## Documentation

Full documentation at [msilverblatt.github.io/protomcp](https://msilverblatt.github.io/protomcp/):

- [Quick Start](https://msilverblatt.github.io/protomcp/getting-started/quick-start/)
- [Python Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-python/)
- [TypeScript Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-typescript/)
- [CLI Reference](https://msilverblatt.github.io/protomcp/reference/cli/)
- [Protobuf Spec](https://msilverblatt.github.io/protomcp/reference/protobuf-spec/)

## License

MIT
