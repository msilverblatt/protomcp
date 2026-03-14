// File search with cancellation support.
// Run: pmcp dev examples/rust/real_world/src/main.rs

use protomcp::{tool, ToolResult, ArgDef};
use std::path::Path;

#[tokio::main]
async fn main() {
    tool("search_files")
        .description("Search for files matching a pattern in a directory")
        .arg(ArgDef::string("directory"))
        .arg(ArgDef::string("pattern"))
        .read_only_hint(true)
        .handler(|ctx, args| {
            let dir = args["directory"].as_str().unwrap_or(".");
            let pattern = args["pattern"].as_str().unwrap_or("");

            let mut matches = Vec::new();
            if let Ok(entries) = std::fs::read_dir(Path::new(dir)) {
                for entry in entries.flatten() {
                    if ctx.is_cancelled() {
                        return ToolResult::new("search cancelled");
                    }
                    if let Some(name) = entry.file_name().to_str() {
                        if name.contains(pattern) {
                            matches.push(entry.path().display().to_string());
                        }
                    }
                }
            }

            ToolResult::new(matches.join("\n"))
        })
        .register();

    protomcp::run().await;
}
