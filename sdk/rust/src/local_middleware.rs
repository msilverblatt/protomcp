use std::sync::{Arc, Mutex};
use serde_json::Value;
use crate::context::ToolContext;
use crate::result::ToolResult;

/// A local (in-process) middleware handler type.
/// Receives: (ctx, tool_name, args, next) -> ToolResult
/// where next is the next handler in the chain: (ToolContext, Value) -> ToolResult
type LocalMwFn = dyn Fn(ToolContext, &str, Value, &dyn Fn(ToolContext, Value) -> ToolResult) -> ToolResult
    + Send
    + Sync;

struct LocalMiddlewareDef {
    priority: i32,
    handler: Arc<LocalMwFn>,
}

static LOCAL_MW_REGISTRY: Mutex<Vec<LocalMiddlewareDef>> = Mutex::new(Vec::new());

/// Register a local middleware that wraps tool handlers in-process.
/// Priority determines execution order: lower numbers run first (outermost).
pub fn local_middleware(
    priority: i32,
    handler: impl Fn(ToolContext, &str, Value, &dyn Fn(ToolContext, Value) -> ToolResult) -> ToolResult
        + Send
        + Sync
        + 'static,
) {
    LOCAL_MW_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(LocalMiddlewareDef {
        priority,
        handler: Arc::new(handler),
    });
}

/// Build a middleware chain wrapping the given handler.
/// Returns a boxed function that, when called, runs through all registered
/// local middleware in priority order before reaching the actual handler.
pub fn build_middleware_chain(
    tool_name: &str,
    handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>,
) -> Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync> {
    // Snapshot the middleware handlers (as Arcs) so we don't hold the lock during execution.
    let mut sorted: Vec<Arc<LocalMwFn>> = {
        let guard = LOCAL_MW_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
        if guard.is_empty() {
            return handler;
        }
        let mut entries: Vec<(i32, Arc<LocalMwFn>)> = guard
            .iter()
            .map(|mw| (mw.priority, Arc::clone(&mw.handler)))
            .collect();
        entries.sort_by_key(|(p, _)| *p);
        entries.into_iter().map(|(_, h)| h).collect()
    };

    let tool_name = tool_name.to_string();

    // Build chain from innermost (last in sorted = highest priority) outward.
    type ChainFn = Arc<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>;
    let mut chain: ChainFn = Arc::from(handler);

    // Reverse so we wrap from innermost to outermost.
    sorted.reverse();
    for mw_handler in sorted {
        let next = chain.clone();
        let tn = tool_name.clone();
        let mw = mw_handler.clone();
        chain = Arc::new(move |ctx: ToolContext, args: Value| {
            let next_ref = next.clone();
            let next_fn = move |c: ToolContext, a: Value| -> ToolResult {
                next_ref(c, a)
            };
            mw(ctx, &tn, args, &next_fn)
        });
    }

    let final_chain = chain;
    Box::new(move |ctx, args| final_chain(ctx, args))
}

pub fn clear_local_middleware() {
    LOCAL_MW_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use std::sync::atomic::{AtomicBool, AtomicI32};

    static TEST_LOCK: Mutex<()> = Mutex::new(());

    fn lock_and_clear() -> std::sync::MutexGuard<'static, ()> {
        let guard = TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        clear_local_middleware();
        guard
    }

    fn dummy_ctx() -> ToolContext {
        ToolContext::new(
            String::new(),
            Arc::new(AtomicBool::new(false)),
            Box::new(|_| {}),
        )
    }

    #[test]
    fn test_no_middleware() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();
        let handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync> =
            Box::new(|_, _| ToolResult::new("direct"));
        let chain = build_middleware_chain("test_tool", handler);
        let result = chain(dummy_ctx(), serde_json::json!({}));
        assert_eq!(result.result_text, "direct");
        clear_local_middleware();
    }

    #[test]
    fn test_single_middleware() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();

        local_middleware(10, |ctx, _tool_name, args, next| {
            let mut result = next(ctx, args);
            result.result_text = format!("[wrapped] {}", result.result_text);
            result
        });

        let handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync> =
            Box::new(|_, _| ToolResult::new("inner"));
        let chain = build_middleware_chain("my_tool", handler);
        let result = chain(dummy_ctx(), serde_json::json!({}));
        assert_eq!(result.result_text, "[wrapped] inner");

        clear_local_middleware();
    }

    #[test]
    fn test_priority_ordering() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();

        let call_order = Arc::new(Mutex::new(Vec::<i32>::new()));

        let co1 = call_order.clone();
        local_middleware(20, move |ctx, _tn, args, next| {
            co1.lock().unwrap_or_else(|e| e.into_inner()).push(20);
            next(ctx, args)
        });

        let co2 = call_order.clone();
        local_middleware(5, move |ctx, _tn, args, next| {
            co2.lock().unwrap_or_else(|e| e.into_inner()).push(5);
            next(ctx, args)
        });

        let co3 = call_order.clone();
        local_middleware(10, move |ctx, _tn, args, next| {
            co3.lock().unwrap_or_else(|e| e.into_inner()).push(10);
            next(ctx, args)
        });

        let handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync> =
            Box::new(|_, _| ToolResult::new("done"));
        let chain = build_middleware_chain("test", handler);
        let result = chain(dummy_ctx(), serde_json::json!({}));
        assert_eq!(result.result_text, "done");

        let order = call_order.lock().unwrap_or_else(|e| e.into_inner());
        assert_eq!(*order, vec![5, 10, 20]);

        clear_local_middleware();
    }

    #[test]
    fn test_middleware_can_short_circuit() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();

        local_middleware(1, |_ctx, _tn, _args, _next| {
            ToolResult::error("blocked", "BLOCKED", "", false)
        });

        let called = Arc::new(AtomicI32::new(0));
        let called2 = called.clone();
        let handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync> =
            Box::new(move |_, _| {
                called2.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
                ToolResult::new("should not reach")
            });

        let chain = build_middleware_chain("test", handler);
        let result = chain(dummy_ctx(), serde_json::json!({}));
        assert!(result.is_error);
        assert_eq!(result.error_code, "BLOCKED");
        assert_eq!(called.load(std::sync::atomic::Ordering::SeqCst), 0);

        clear_local_middleware();
    }
}
