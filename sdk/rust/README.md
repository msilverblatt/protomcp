# protomcp

[![CI](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml/badge.svg)](https://github.com/msilverblatt/protomcp/actions/workflows/ci.yml)
[![crates.io](https://img.shields.io/crates/v/protomcp)](https://crates.io/crates/protomcp)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://github.com/msilverblatt/protomcp/blob/main/LICENSE)

Rust SDK for [protomcp](https://github.com/msilverblatt/protomcp) -- build MCP servers with tools, resources, and prompts in one file, one command.

## Install

Add to your `Cargo.toml`:

```toml
[dependencies]
protomcp = "0.1"
tokio = { version = "1", features = ["full"] }
```

You also need the `pmcp` CLI:

```sh
brew install msilverblatt/tap/protomcp
```

## Quick Start

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

## Tool Groups

Group related actions under a single tool with per-action schemas:

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

## Documentation

- [Full documentation](https://msilverblatt.github.io/protomcp/)
- [Rust Guide](https://msilverblatt.github.io/protomcp/guides/writing-tools-rust/)
- [CLI Reference](https://msilverblatt.github.io/protomcp/reference/cli/)
- [Examples](https://github.com/msilverblatt/protomcp/tree/main/examples/rust)

## License

MIT
