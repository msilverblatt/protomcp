use std::sync::Mutex;

pub struct PromptArg {
    pub name: String,
    pub description: String,
    pub required: bool,
}

pub struct PromptMessage {
    pub role: String,
    pub content: String,
}

pub struct PromptDef {
    pub name: String,
    pub description: String,
    pub arguments: Vec<PromptArg>,
    pub handler: Box<dyn Fn(serde_json::Value) -> Vec<PromptMessage> + Send + Sync>,
}

static PROMPT_REGISTRY: Mutex<Vec<PromptDef>> = Mutex::new(Vec::new());

pub fn register_prompt(def: PromptDef) {
    PROMPT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(def);
}

pub(crate) fn with_prompts<F, R>(f: F) -> R
where F: FnOnce(&[PromptDef]) -> R {
    let guard = PROMPT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    f(&guard)
}

pub fn clear_prompt_registry() {
    PROMPT_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}
