// Full showcase: structured output, dynamic tools, error handling.
// Run: pmcp dev examples/rust/full_showcase/src/main.rs

use protomcp::{tool, ToolResult, ArgDef};

#[tokio::main]
async fn main() {
    tool("calculator")
        .description("Perform arithmetic operations with structured output")
        .arg(ArgDef::number("a"))
        .arg(ArgDef::number("b"))
        .arg(ArgDef::string("operation"))
        .read_only_hint(true)
        .handler(|_ctx, args| {
            let a = args["a"].as_f64().unwrap_or(0.0);
            let b = args["b"].as_f64().unwrap_or(0.0);
            let op = args["operation"].as_str().unwrap_or("");

            let result = match op {
                "add" => a + b,
                "subtract" => a - b,
                "multiply" => a * b,
                "divide" => {
                    if b == 0.0 {
                        return ToolResult::error(
                            "division by zero",
                            "INVALID_INPUT",
                            "provide a non-zero divisor",
                            false,
                        );
                    }
                    a / b
                }
                _ => {
                    return ToolResult::error(
                        format!("unknown operation: {}", op),
                        "INVALID_INPUT",
                        "use add, subtract, multiply, or divide",
                        false,
                    );
                }
            };

            let output = serde_json::json!({
                "result": result,
                "operation": op,
                "operands": [a, b],
            });
            ToolResult::new(output.to_string())
        })
        .register();

    tool("enable_admin")
        .description("Enable admin tools")
        .handler(|_ctx, _args| {
            let mut r = ToolResult::new("admin tools enabled");
            r.enable_tools = vec!["admin_panel".to_string()];
            r
        })
        .register();

    tool("admin_panel")
        .description("Admin panel — only available after enable_admin")
        .destructive_hint(true)
        .handler(|_ctx, _args| {
            ToolResult::new("admin action performed")
        })
        .register();

    protomcp::run().await;
}
