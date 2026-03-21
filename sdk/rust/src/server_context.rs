use std::collections::HashMap;
use std::sync::Mutex;
use serde_json::Value;

type ContextResolver = Box<dyn Fn(&mut Value) -> Value + Send + Sync>;

struct ContextDef {
    param_name: String,
    resolver: ContextResolver,
}

static CONTEXT_REGISTRY: Mutex<Vec<ContextDef>> = Mutex::new(Vec::new());

/// Register a server context resolver. The resolver receives mutable args
/// and can inspect/remove keys. Returns the resolved value that will be
/// injected into the handler args under `param_name`.
pub fn server_context(
    param_name: &str,
    resolver: impl Fn(&mut Value) -> Value + Send + Sync + 'static,
) {
    CONTEXT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(ContextDef {
        param_name: param_name.to_string(),
        resolver: Box::new(resolver),
    });
}

/// Run all registered context resolvers against args.
/// Returns a map of param_name -> resolved value.
pub fn resolve_contexts(args: &mut Value) -> HashMap<String, Value> {
    let guard = CONTEXT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    let mut resolved = HashMap::new();
    for ctx_def in guard.iter() {
        let val = (ctx_def.resolver)(args);
        resolved.insert(ctx_def.param_name.clone(), val);
    }
    resolved
}

pub fn clear_context_registry() {
    CONTEXT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    static TEST_LOCK: Mutex<()> = Mutex::new(());

    fn lock_and_clear() -> std::sync::MutexGuard<'static, ()> {
        let guard = TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        clear_context_registry();
        guard
    }

    #[test]
    fn test_register_and_resolve() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();

        server_context("user_id", |args| {
            let token = args.get("auth_token").and_then(|v| v.as_str()).unwrap_or("");
            let user = if token == "abc123" { "user_42" } else { "anonymous" };
            // Remove the auth_token from args so handler doesn't see it
            if let Some(obj) = args.as_object_mut() {
                obj.remove("auth_token");
            }
            Value::String(user.to_string())
        });

        let mut args = json!({"auth_token": "abc123", "query": "hello"});
        let resolved = resolve_contexts(&mut args);

        assert_eq!(resolved.get("user_id").unwrap(), "user_42");
        // auth_token should have been removed
        assert!(args.get("auth_token").is_none());
        // query should remain
        assert_eq!(args.get("query").unwrap(), "hello");

        clear_context_registry();
    }

    #[test]
    fn test_multiple_contexts() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();

        server_context("ctx_a", |_args| Value::String("val_a".to_string()));
        server_context("ctx_b", |_args| Value::Number(serde_json::Number::from(42)));

        let mut args = json!({"foo": "bar"});
        let resolved = resolve_contexts(&mut args);

        assert_eq!(resolved.len(), 2);
        assert_eq!(resolved.get("ctx_a").unwrap(), "val_a");
        assert_eq!(resolved.get("ctx_b").unwrap(), 42);

        clear_context_registry();
    }

    #[test]
    fn test_clear_registry() {
        let _lock = crate::TEST_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        crate::clear_all_registries();
        server_context("x", |_| Value::Null);
        {
            let guard = CONTEXT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            assert_eq!(guard.len(), 1);
        }
        clear_context_registry();
        {
            let guard = CONTEXT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            assert_eq!(guard.len(), 0);
        }
    }
}
