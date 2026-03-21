# protomcp

[![CI](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Build powerful MCP workflows with dynamic tool lists and server-defined workflows. Lightweight protobuf-based MCP runtime built on the official Go SDK.**

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

The unix socket + protobuf layer adds ~0.5ms of overhead per tool call.

## Quick Start

### Install

```sh
brew install msilverblatt/tap/protomcp
```

### Install SDK

```sh
# Python
pip install protomcp

# TypeScript
npm install protomcp

# Go
go get github.com/msilverblatt/protomcp/sdk/go/protomcp

# Rust
# Add to Cargo.toml: protomcp = "0.1"
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

## Advanced Features

- **Native Tool Groups** -- Group related actions with per-action schemas (oneOf discriminated unions). Each action gets its own required fields, enums, and validation rules while appearing as a single tool to the LLM.
- **Server-Defined Workflows** -- Multi-step state machines where the visible tool surface IS the state. The server defines the process, the agent follows it. Supports `no_cancel`, error recovery, dynamic branching, and tool visibility control.
- **Local Middleware** -- In-process middleware chain for error handling, timing, auto-install, or any cross-cutting concern. Middleware receives the tool name, args, and a `next` handler.
- **Server Context** -- Inject shared parameters (project directory, DB connection, auth tokens) into tool handlers automatically. Hidden contexts stay out of the tool schema entirely.
- **Telemetry** -- Structured `ToolCallEvent`s (start, success, error, progress) emitted to pluggable sinks. Wire up logging, metrics, or tracing with a single decorator.
- **Declarative Validation** -- Required fields, enum fuzzy matching (with "did you mean?" suggestions), and cross-parameter rules defined alongside your actions.
- **Sidecar Management** -- Managed companion processes with health checks, started and stopped alongside your server.
- **Handler Discovery** -- Auto-discover tool handlers from a directory so you can organize large servers into separate files.

See the [full documentation](https://msilverblatt.github.io/protomcp/) for details on each feature.

## Tool Groups

Real-world MCP tools tend to accumulate dozens of parameters behind a single endpoint. Tool groups let you split actions into clean, per-action schemas. By default, each action becomes its own tool (e.g. `db.query`, `db.insert`) — the **separate** strategy. For clients that support `oneOf` schemas, the **union** strategy is available as an opt-in, exposing all actions as a single tool with a discriminated union.

**Before** -- one tool, 20+ params:

```python
@tool("Manage data")
def data(action: str, data_path: str | None = None, join_on: list[str] | None = None,
         prefix: str | None = None, column: str | None = None, strategy: str = "median",
         # ... 15 more params ...
         ) -> ToolResult:
    ...
```

**After** -- per-action schemas, one class:

```python
@tool_group("data", description="Manage data")
class DataTools:
    @action("add", description="Ingest a dataset")
    def add(self, data_path: str, join_on: list[str] | None = None) -> ToolResult:
        ...

    @action("profile", description="Profile the dataset")
    def profile(self, category: str | None = None) -> ToolResult:
        ...
```

The same pattern works across all four languages:

**Python**

```python
from protomcp import tool_group, action, ToolResult

@tool_group("math", description="Math operations")
class MathTools:
    @action("add", description="Add two numbers")
    def add(self, a: int, b: int) -> ToolResult:
        return ToolResult(result=str(a + b))

    @action("multiply", description="Multiply two numbers")
    def multiply(self, a: int, b: int) -> ToolResult:
        return ToolResult(result=str(a * b))
```

**Go**

```go
protomcp.ToolGroup("math",
    protomcp.Description("Math operations"),
    protomcp.Action("add", protomcp.Description("Add two numbers"),
        protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
        protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
            return protomcp.Result(fmt.Sprintf("%d", int(args["a"].(float64))+int(args["b"].(float64))))
        }),
    ),
    protomcp.Action("multiply", protomcp.Description("Multiply two numbers"),
        protomcp.Args(protomcp.IntArg("a"), protomcp.IntArg("b")),
        protomcp.Handler(func(ctx protomcp.ToolContext, args map[string]interface{}) protomcp.ToolResult {
            return protomcp.Result(fmt.Sprintf("%d", int(args["a"].(float64))*int(args["b"].(float64))))
        }),
    ),
)
```

**TypeScript**

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

**Rust**

```rust
use protomcp::{tool_group, action, ToolResult, ArgDef};

tool_group("math")
    .description("Math operations")
    .action(action("add")
        .description("Add two numbers")
        .arg(ArgDef::int("a"))
        .arg(ArgDef::int("b"))
        .handler(|_ctx, args| {
            let a = args["a"].as_i64().unwrap_or(0);
            let b = args["b"].as_i64().unwrap_or(0);
            ToolResult::new(format!("{}", a + b))
        }))
    .action(action("multiply")
        .description("Multiply two numbers")
        .arg(ArgDef::int("a"))
        .arg(ArgDef::int("b"))
        .handler(|_ctx, args| {
            let a = args["a"].as_i64().unwrap_or(0);
            let b = args["b"].as_i64().unwrap_or(0);
            ToolResult::new(format!("{}", a * b))
        }))
    .register();
```

## Server-Defined Workflows

Define multi-step processes as state machines. The agent only sees valid next actions at each point — the server controls progression. No more hoping the agent calls tools in the right order.

```python
@workflow("deploy", allow_during=["status"])
class DeployWorkflow:
    @step(initial=True, next=["approve", "reject"])
    def review(self, pr_url: str) -> StepResult:
        return StepResult(result=f"3 files changed in {pr_url}")

    @step(next=["run_tests"])
    def approve(self, reason: str) -> StepResult:
        return StepResult(result="Approved")

    @step(next=["promote", "rollback"], no_cancel=True)
    def run_tests(self) -> StepResult:
        results = execute_tests()
        if results.all_passed:
            return StepResult(result="All passed", next=["promote"])
        return StepResult(result="Failures", next=["rollback"])

    @step(terminal=True, no_cancel=True)
    def promote(self) -> StepResult:
        return StepResult(result="Live in production")
```

At each step, the framework automatically enables/disables tools so the agent can only take valid actions. `no_cancel=True` means the agent is committed — no backing out mid-migration. `allow_during=["status"]` lets the agent check status between steps without leaving the workflow.

Workflows also support:
- **Dynamic next** — narrow available next steps based on runtime state
- **Error handling** — failed steps stay in state for retry, or route to recovery steps via `on_error`
- **Lifecycle hooks** — `on_cancel` for cleanup, `on_complete` for audit logging
- **Tool visibility** — `allow_during` / `block_during` with glob patterns, step-level overrides

## When to Use protomcp

protomcp is not a replacement for the official MCP SDKs — it's built on top of the [official Go SDK](https://github.com/modelcontextprotocol/go-sdk). Use protomcp when:

- **You want the same API across languages** — switch between Python, TypeScript, Go, and Rust with identical concepts and patterns
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
- **Tool Groups** — per-action schemas with separate (default) and union strategies
- **Advanced Server** — middleware, telemetry, server context working together
- **Workflows** — deployment pipeline as a server-defined state machine

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
