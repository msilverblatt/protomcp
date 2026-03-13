# protomcp

[![CI](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![PyPI](https://img.shields.io/pypi/v/protomcp)](https://pypi.org/project/protomcp/)
[![npm](https://img.shields.io/npm/v/protomcp)](https://www.npmjs.com/package/protomcp)
[![crates.io](https://img.shields.io/crates/v/protomcp)](https://crates.io/crates/protomcp)

**Write MCP tools in any language. One file, one command, hot-reload.**

Build MCP tools without protocol boilerplate, restarting your server with very change, or needing a different SDK for every language. 

protomcp lets you write a handler function, run `pmcp dev tools.py`, and your tools just work with any MCP client. Change your code, save the file, and it reloads instantly.

## How It Works

```
┌─────────────┐          ┌─────────────┐           ┌──────────────┐
│             │   MCP    │             │  protobuf │              │
│  MCP Host   │ ◄──────► │    pmcp     │ ◄────────►│  Your Code   │
│  (Claude,   │ JSON-RPC │   (Go)      │   unix    │  (any lang)  │
│   Cursor…)  │  stdio   │             │   socket  │              │
└─────────────┘          └─────────────┘           └──────────────┘
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

### Go

```go
// tools.go
package main

import (
	"fmt"
	"github.com/msilverblatt/protomcp/sdk/go/protomcp"
)

func main() {
	protomcp.Tool("add",
		protomcp.Description("Add two numbers"),
		protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
		protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
			a := int(args["a"].(float64))
			b := int(args["b"].(float64))
			return protomcp.Result(fmt.Sprintf("%d", a+b))
		}),
	)
	protomcp.Run()
}
```

```sh
pmcp dev tools.go
```

### Rust

```rust
// src/main.rs
use protomcp::{tool, ToolResult, ArgDef};

#[tokio::main]
async fn main() {
    tool("add")
        .description("Add two numbers")
        .arg(ArgDef::int("a"))
        .arg(ArgDef::int("b"))
        .handler(|_ctx, args| {
            let a = args["a"].as_i64().unwrap_or(0);
            let b = args["b"].as_i64().unwrap_or(0);
            ToolResult::new(format!("{}", a + b))
        })
        .register();
    protomcp::run().await;
}
```

```sh
pmcp dev src/main.rs
```

Then add the `pmcp dev` command to your MCP client config. That's it.

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
- **Custom Middleware** — intercept tool calls with before/after hooks registered from your tool process
- **Authentication** — built-in token and API key auth for network transports
- **Validation** — `pmcp validate` checks tool definitions before deployment

## Comparison

| Feature | pmcp | FastMCP (Python) | MCP SDKs |
|---------|------|------------------|----------|
| Language support | Any (Python, TS, Go, Rust, ...) | Python only | One SDK per language |
| Hot reload | Built-in | No | No |
| Dynamic tool lists | Built-in | No | Manual |
| Custom middleware | Built-in | No | No |
| Authentication | Built-in (token, apikey) | Manual | Manual |
| Validation | `pmcp validate` | No | No |
| Transports | stdio, SSE, HTTP, WS, gRPC | stdio, SSE | Varies by SDK |
| Structured output | Yes | No | Varies |
| Async tasks | Yes | No | No |
| Single binary | Yes (Go) | No (Python runtime) | No (per-language) |

## Examples

See [`examples/`](examples/) for runnable demos at three levels:

- **Basic** — minimal single-tool examples in Python, TypeScript, Go, and Rust
- **Real-world** — file search tool with progress reporting, cancellation, and logging
- **Full showcase** — multi-tool server with structured output, dynamic tool lists, error handling, and metadata

Examples are available in all four languages: [`examples/python/`](examples/python/), [`examples/typescript/`](examples/typescript/), [`examples/go/`](examples/go/), [`examples/rust/`](examples/rust/).

Run all demos: `./examples/run-demo.sh`

## Documentation

Full documentation at [msilverblatt.github.io/protomcp](https://msilverblatt.github.io/protomcp/):

- [Quick Start](https://msilverblatt.github.io/protomcp/getting-started/quick-start/)
- [Python Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-python/)
- [TypeScript Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-typescript/)
- [Go Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-go/)
- [Rust Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-rust/)
- [Custom Middleware](https://msilverblatt.github.io/protomcp/guides/middleware/)
- [Authentication](https://msilverblatt.github.io/protomcp/guides/auth/)
- [CLI Reference](https://msilverblatt.github.io/protomcp/reference/cli/)
- [Protobuf Spec](https://msilverblatt.github.io/protomcp/reference/protobuf-spec/)

## License

MIT
