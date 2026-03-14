pub mod proto {
    include!(concat!(env!("OUT_DIR"), "/protomcp.rs"));
}

mod tool;
mod result;
mod context;
mod manager;
mod log;
mod transport;
mod runner;
mod middleware;
pub mod resource;
pub mod prompt;
pub mod completion;
pub mod group;

pub use tool::{tool, ToolDef, ArgDef, clear_registry, arg_to_schema};
pub use group::{tool_group, groups_to_tool_defs, clear_group_registry};
pub use result::ToolResult;
pub use context::ToolContext;
pub use runner::run;
pub use log::ServerLogger;
pub use manager::ToolManager;
pub use middleware::{middleware, clear_middleware_registry};
pub use resource::{register_resource, register_resource_template, ResourceDef, ResourceTemplateDef, ResourceContent};
pub use prompt::{register_prompt, PromptDef, PromptArg, PromptMessage};
pub use completion::{register_completion, CompletionResult};
