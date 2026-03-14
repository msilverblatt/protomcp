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
pub mod server_context;
pub mod local_middleware;
pub mod telemetry;
pub mod sidecar;

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
pub use server_context::{server_context, resolve_contexts, clear_context_registry};
pub use local_middleware::{local_middleware, build_middleware_chain, clear_local_middleware};
pub use telemetry::{telemetry_sink, emit_telemetry, clear_telemetry_sinks, ToolCallEvent};
pub use sidecar::{sidecar, start_sidecars, stop_all_sidecars, clear_sidecar_registry, SidecarDef};
