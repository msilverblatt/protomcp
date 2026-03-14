use std::collections::HashMap;
use std::sync::Mutex;

pub struct CompletionResult {
    pub values: Vec<String>,
    pub total: i32,
    pub has_more: bool,
}

type CompletionHandler = Box<dyn Fn(&str) -> CompletionResult + Send + Sync>;

static COMPLETION_REGISTRY: Mutex<Option<HashMap<(String, String, String), CompletionHandler>>> = Mutex::new(None);

pub fn register_completion(ref_type: &str, ref_name: &str, arg_name: &str, handler: CompletionHandler) {
    let mut guard = COMPLETION_REGISTRY.lock().unwrap();
    let map = guard.get_or_insert_with(HashMap::new);
    map.insert((ref_type.to_string(), ref_name.to_string(), arg_name.to_string()), handler);
}

pub(crate) fn get_completion_handler(ref_type: &str, ref_name: &str, arg_name: &str) -> Option<CompletionResult> {
    let guard = COMPLETION_REGISTRY.lock().unwrap();
    if let Some(map) = guard.as_ref() {
        let key = (ref_type.to_string(), ref_name.to_string(), arg_name.to_string());
        if let Some(handler) = map.get(&key) {
            return Some(handler(""));
        }
    }
    None
}
