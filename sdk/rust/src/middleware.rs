use std::collections::HashMap;
use std::sync::Mutex;

/// Handler function type for middleware intercept callbacks.
pub type MiddlewareHandler = Box<dyn Fn(&str, &str, &str, &str, bool) -> HashMap<String, String> + Send + Sync>;

pub struct MiddlewareDef {
    pub name: String,
    pub priority: i32,
    pub handler: MiddlewareHandler,
}

static MIDDLEWARE_REGISTRY: Mutex<Vec<MiddlewareDef>> = Mutex::new(Vec::new());

/// Register a custom middleware handler that intercepts tool calls.
/// Priority determines execution order: lower numbers run first in the before phase.
pub fn middleware<F>(name: &str, priority: i32, handler: F)
where
    F: Fn(&str, &str, &str, &str, bool) -> HashMap<String, String> + Send + Sync + 'static,
{
    MIDDLEWARE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(MiddlewareDef {
        name: name.to_string(),
        priority,
        handler: Box::new(handler),
    });
}

pub(crate) fn with_middleware_registry<F, R>(f: F) -> R
where
    F: FnOnce(&[MiddlewareDef]) -> R,
{
    let guard = MIDDLEWARE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    f(&guard)
}

pub fn clear_middleware_registry() {
    MIDDLEWARE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}

#[cfg(test)]
mod tests {
    use super::*;

    static TEST_LOCK: Mutex<()> = Mutex::new(());

    #[test]
    fn test_middleware_registration() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();
        middleware("audit", 10, |_phase, _tool, _args, _result, _err| {
            HashMap::new()
        });
        with_middleware_registry(|mws| {
            assert_eq!(mws.len(), 1);
            assert_eq!(mws[0].name, "audit");
            assert_eq!(mws[0].priority, 10);
        });
        clear_middleware_registry();
    }
}
