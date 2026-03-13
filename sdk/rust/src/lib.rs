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

pub use tool::{tool, ToolDef, ArgDef, clear_registry};
pub use result::ToolResult;
pub use context::ToolContext;
pub use runner::run;
pub use log::ServerLogger;
pub use manager::ToolManager;
pub use middleware::{middleware, clear_middleware_registry};
