// Benchmark tool fixture for protomcp — Rust implementation.
// Provides echo, add, compute, generate, parse_json tools.

use protomcp::{tool, ArgDef, ToolResult};

fn main() {
    tool("echo")
        .description("Echo the input back")
        .arg(ArgDef::string("message"))
        .handler(|_ctx, args| {
            let msg = args["message"].as_str().unwrap_or("");
            ToolResult::new(msg)
        })
        .register();

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

    tool("compute")
        .description("CPU-bound work: hash a string N times")
        .arg(ArgDef::int("iterations"))
        .handler(|_ctx, args| {
            let iters = args["iterations"].as_i64().unwrap_or(0) as usize;
            let mut result = "seed".to_string();
            for _ in 0..iters {
                use std::collections::hash_map::DefaultHasher;
                use std::hash::{Hash, Hasher};
                let mut hasher = DefaultHasher::new();
                result.hash(&mut hasher);
                result = format!("{:016x}", hasher.finish());
            }
            ToolResult::new(result)
        })
        .register();

    tool("generate")
        .description("Return a string of the requested size in bytes")
        .arg(ArgDef::int("size"))
        .handler(|_ctx, args| {
            let size = args["size"].as_i64().unwrap_or(0) as usize;
            ToolResult::new("X".repeat(size))
        })
        .register();

    tool("parse_json")
        .description("Parse JSON and return it serialized back")
        .arg(ArgDef::string("data"))
        .handler(|_ctx, args| {
            let data = args["data"].as_str().unwrap_or("{}");
            let parsed: serde_json::Value = serde_json::from_str(data).unwrap_or(serde_json::json!(null));
            ToolResult::new(serde_json::to_string(&parsed).unwrap_or_default())
        })
        .register();

    let rt = tokio::runtime::Runtime::new().unwrap();
    rt.block_on(protomcp::run());
}
