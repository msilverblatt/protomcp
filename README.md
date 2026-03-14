# protomcp

[![CI](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![PyPI](https://img.shields.io/pypi/v/protomcp)](https://pypi.org/project/protomcp/)
[![npm](https://img.shields.io/npm/v/protomcp)](https://www.npmjs.com/package/protomcp)
[![crates.io](https://img.shields.io/crates/v/protomcp)](https://crates.io/crates/protomcp)

**Build MCP servers in any language. Tools, resources, prompts — one file, one command.**

protomcp is a language-agnostic MCP runtime. Write your server logic in Python, TypeScript, Go, or Rust. Run `pmcp dev server.py` and you get a spec-compliant MCP server with hot reload, no protocol boilerplate, and no framework lock-in.

## How It Works

```
┌─────────────┐          ┌─────────────┐           ┌──────────────┐
│             │   MCP    │             │  protobuf │              │
│  MCP Host   │ ◄──────► │    pmcp     │ ◄────────►│  Your Code   │
│  (Claude,   │ JSON-RPC │  (official  │   unix    │  (any lang)  │
│   Cursor…)  │          │  Go SDK)    │   socket  │              │
└─────────────┘          └─────────────┘           └──────────────┘
```

pmcp uses the [official MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk) for full spec compliance. Your code registers tools, resources, and prompts through a simple protobuf protocol over a unix socket. pmcp handles everything else — protocol negotiation, transport, pagination, session management, and hot reload.

## Quick Start

### Install

```sh
brew install protomcp/tap/protomcp
```

### Python

```python
# server.py
from protomcp import tool, resource, prompt, ToolResult, ResourceContent, PromptMessage, PromptArg

@tool("Add two numbers")
def add(a: int, b: int) -> ToolResult:
    return ToolResult(result=str(a + b))

@resource(uri="config://app", description="App configuration")
def app_config(uri: str) -> ResourceContent:
    return ResourceContent(uri=uri, text='{"debug": false, "version": "2.1"}')

@prompt(description="Explain a concept", arguments=[PromptArg(name="topic", required=True)])
def explain(topic: str) -> list[PromptMessage]:
    return [PromptMessage(role="user", content=f"Explain {topic} in simple terms")]
```

```sh
pmcp dev server.py
```

### TypeScript

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

### Go

```go
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
pmcp dev server.go
```

### Rust

```rust
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

## MCP Spec Coverage

protomcp implements the full MCP specification (2025-03-26) via the official Go SDK:

| Feature | Status | Description |
|---------|--------|-------------|
| Tools | list, call, annotations, structured output | Define tools with typed arguments and results |
| Resources | list, read, templates, subscribe | Expose data for MCP clients to read |
| Prompts | list, get, arguments | Reusable message templates with parameters |
| Completions | complete | Autocomplete suggestions for prompt/resource arguments |
| Sampling | createMessage | SDK processes can request LLM calls from the MCP client |
| Roots | list | Access client filesystem roots |
| Transports | stdio, Streamable HTTP | Official SDK transports with session management |
| Logging | setLevel, 8 log levels | Structured logging forwarded to MCP host |
| Pagination | cursor-based | Automatic pagination for list operations |
| Progress | notifications | Report progress for long-running operations |
| Cancellation | notifications/cancelled | Respond to client cancellation requests |
| Ping | ping/pong | Built-in keepalive |
| Content Types | text, image, audio, embedded resource, resource link | Full content type support |

## Features

- **Any Language** — write servers in Python, TypeScript, Go, Rust, or any language that speaks protobuf
- **Full MCP Spec** — tools, resources, prompts, completions, sampling, roots, logging, pagination
- **Official SDK** — built on [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) for guaranteed spec compliance
- **Hot Reload** — save your file and everything reloads instantly, no restart
- **Dynamic Tool Lists** — enable/disable tools at runtime based on context
- **Sampling** — your tool code can request LLM calls from the MCP client
- **Structured Output** — define output schemas for typed tool results
- **Progress & Cancellation** — report progress and respond to cancellation
- **Server Logging** — 8 RFC 5424 log levels forwarded to the MCP host
- **Custom Middleware** — intercept tool calls with before/after hooks
- **Validation** — `pmcp validate` checks definitions before deployment

## When to Use protomcp

protomcp is not a replacement for the official MCP SDKs — it's built on top of the [official Go SDK](https://github.com/modelcontextprotocol/go-sdk). Use protomcp when:

- **You want one server in multiple languages** — write tools in Python, prompts in TypeScript, resources in Go, all served by a single MCP server
- **You want zero-config hot reload** — save a file, everything reloads instantly
- **You don't want to learn MCP internals** — no JSON-RPC, no transport wiring, no session management
- **You want a single binary** — `pmcp` is a single Go binary, no runtime dependencies for the server itself

Use the official SDKs directly when:

- **You only need one language** — the official Python/TS/Go SDKs are excellent for single-language servers
- **You need maximum control** — middleware, custom transports, OAuth flows, fine-grained session management
- **You're embedding MCP in an existing app** — the official SDKs integrate as libraries, protomcp runs as a separate process

## Testing & Playground

protomcp includes built-in testing tools — no agent required.

### `pmcp test` — CLI testing

```sh
# List all tools, resources, and prompts with their schemas
pmcp test server.py list

# Call a tool and see the result + full protocol trace
pmcp test server.py call get_weather --args '{"city": "SF"}'

# JSON output for scripting
pmcp test server.py list --format json
```

### `pmcp playground` — Interactive web UI

```sh
pmcp playground server.py --port 3000
# Open http://localhost:3000
```

Two-panel layout: interact with tools/resources/prompts on the left, watch every JSON-RPC message in real-time on the right. Auto-generated forms from your tool schemas, live protocol tracing, and hot reload visibility.

## Examples

See [`examples/`](examples/) for runnable demos:

- **Basic** — minimal tool examples in all four languages
- **Resources & Prompts** — resources, prompts, completions, and tools together
- **Full showcase** — structured output, progress, cancellation, dynamic tool lists, error handling

Examples are available in all four languages: [`examples/python/`](examples/python/), [`examples/typescript/`](examples/typescript/), [`examples/go/`](examples/go/), [`examples/rust/`](examples/rust/).

## Documentation

Full documentation at [msilverblatt.github.io/protomcp](https://msilverblatt.github.io/protomcp/):

- [Quick Start](https://msilverblatt.github.io/protomcp/getting-started/quick-start/)
- [Python Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-python/)
- [TypeScript Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-typescript/)
- [Go Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-go/)
- [Rust Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-rust/)
- [Resources Guide](https://msilverblatt.github.io/protomcp/guides/resources/)
- [Prompts Guide](https://msilverblatt.github.io/protomcp/guides/prompts/)
- [Sampling Guide](https://msilverblatt.github.io/protomcp/guides/sampling/)
- [MCP Compliance](https://msilverblatt.github.io/protomcp/reference/mcp-compliance/)
- [CLI Reference](https://msilverblatt.github.io/protomcp/reference/cli/)

## License

MIT
