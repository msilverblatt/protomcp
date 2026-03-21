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
pub mod workflow;

pub use tool::{tool, ToolDef, ArgDef, clear_registry, arg_to_schema};
pub use group::{tool_group, groups_to_tool_defs, clear_group_registry};
pub use workflow::{workflow as workflow_def, workflows_to_tool_defs, clear_workflow_registry, StepResult, StepHistoryEntry};
pub use result::ToolResult;
pub use context::ToolContext;
pub use runner::run;
pub use log::ServerLogger;
pub use manager::ToolManager;
pub use middleware::{middleware, clear_middleware_registry};
pub use resource::{register_resource, register_resource_template, ResourceDef, ResourceTemplateDef, ResourceContent, clear_resource_registry, clear_resource_template_registry};
pub use prompt::{register_prompt, PromptDef, PromptArg, PromptMessage, clear_prompt_registry};
pub use completion::{register_completion, CompletionResult};
pub use server_context::{server_context, resolve_contexts, clear_context_registry};
pub use local_middleware::{local_middleware, build_middleware_chain, clear_local_middleware};
pub use telemetry::{telemetry_sink, emit_telemetry, clear_telemetry_sinks, ToolCallEvent};
pub use sidecar::{sidecar, start_sidecars, stop_all_sidecars, clear_sidecar_registry, SidecarDef};

/// Clears all global registries. Call at the start of every test to ensure isolation.
pub fn clear_all_registries() {
    clear_registry();
    clear_group_registry();
    clear_workflow_registry();
    clear_middleware_registry();
    clear_local_middleware();
    clear_telemetry_sinks();
    clear_context_registry();
    clear_sidecar_registry();
    clear_prompt_registry();
    clear_resource_registry();
    clear_resource_template_registry();
}

/// A process-wide mutex to serialize tests that share global registry state.
/// Acquire this at the top of any test that reads or writes global registries.
#[cfg(test)]
pub static TEST_LOCK: std::sync::Mutex<()> = std::sync::Mutex::new(());
