// A minimal protomcp tool — adds and multiplies numbers.
// Run: pmcp dev examples/rust/basic/src/main.rs

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

    tool("multiply")
        .description("Multiply two numbers")
        .arg(ArgDef::int("a"))
        .arg(ArgDef::int("b"))
        .handler(|_ctx, args| {
            let a = args["a"].as_i64().unwrap_or(0);
            let b = args["b"].as_i64().unwrap_or(0);
            ToolResult::new(format!("{}", a * b))
        })
        .register();

    protomcp::run().await;
}
