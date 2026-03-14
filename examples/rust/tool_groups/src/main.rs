// Demonstrates tool groups with per-action schemas, validation, and strategies.
// Run: pmcp dev examples/rust/tool_groups/src/main.rs

use protomcp::{tool_group, ToolResult, ArgDef};

#[tokio::main]
async fn main() {
    // Union strategy (default): single tool "db" with action discriminator
    tool_group("db")
        .description("Database operations")
        .action("query", |a| {
            a.description("Run a read-only SQL query")
                .arg(ArgDef::string("sql"))
                .arg(ArgDef::int("limit"))
                .requires(&["sql"])
                .handler(|_ctx, args| {
                    let sql = args["sql"].as_str().unwrap_or("");
                    let limit = args["limit"].as_i64().unwrap_or(100);
                    ToolResult::new(format!("Executed: {} (limit {})", sql, limit))
                })
        })
        .action("insert", |a| {
            a.description("Insert a record into a table")
                .arg(ArgDef::string("table"))
                .arg(ArgDef::string("data"))
                .requires(&["table", "data"])
                .enum_field("table", &["users", "events", "logs"])
                .handler(|_ctx, args| {
                    let table = args["table"].as_str().unwrap_or("");
                    let data = args["data"].as_str().unwrap_or("");
                    ToolResult::new(format!("Inserted into {}: {}", table, data))
                })
        })
        .action("migrate", |a| {
            a.description("Run a schema migration")
                .arg(ArgDef::string("version"))
                .arg(ArgDef::bool("dry_run"))
                .requires(&["version"])
                .handler(|_ctx, args| {
                    let version = args["version"].as_str().unwrap_or("");
                    let dry_run = args["dry_run"].as_bool().unwrap_or(false);
                    let mode = if dry_run { "dry run" } else { "applied" };
                    ToolResult::new(format!("Migration {} {}", version, mode))
                })
        })
        .register();

    // Separate strategy: each action becomes its own tool (files.read, files.write, etc.)
    tool_group("files")
        .description("File operations")
        .strategy("separate")
        .action("read", |a| {
            a.description("Read a file by path")
                .arg(ArgDef::string("path"))
                .requires(&["path"])
                .handler(|_ctx, args| {
                    let path = args["path"].as_str().unwrap_or("");
                    ToolResult::new(format!("Contents of {}", path))
                })
        })
        .action("write", |a| {
            a.description("Write content to a file")
                .arg(ArgDef::string("path"))
                .arg(ArgDef::string("content"))
                .requires(&["path", "content"])
                .handler(|_ctx, args| {
                    let path = args["path"].as_str().unwrap_or("");
                    let content = args["content"].as_str().unwrap_or("");
                    ToolResult::new(format!("Wrote {} bytes to {}", content.len(), path))
                })
        })
        .action("search", |a| {
            a.description("Search files by pattern")
                .arg(ArgDef::string("pattern"))
                .arg(ArgDef::string("scope"))
                .requires(&["pattern"])
                .enum_field("scope", &["workspace", "project", "global"])
                .handler(|_ctx, args| {
                    let pattern = args["pattern"].as_str().unwrap_or("");
                    let scope = args["scope"].as_str().unwrap_or("workspace");
                    ToolResult::new(format!("Searching '{}' in {}", pattern, scope))
                })
        })
        .register();

    protomcp::run().await;
}
