use std::sync::Mutex;

pub type ResourceHandler = Box<dyn Fn(&str) -> Vec<ResourceContent> + Send + Sync>;

pub struct ResourceContent {
    pub uri: String,
    pub text: String,
    pub blob: Vec<u8>,
    pub mime_type: String,
}

impl ResourceContent {
    pub fn text(uri: &str, text: &str) -> Self {
        Self {
            uri: uri.to_string(),
            text: text.to_string(),
            blob: Vec::new(),
            mime_type: "text/plain".to_string(),
        }
    }
}

pub struct ResourceDef {
    pub uri: String,
    pub name: String,
    pub description: String,
    pub mime_type: String,
    pub handler: ResourceHandler,
}

pub struct ResourceTemplateDef {
    pub uri_template: String,
    pub name: String,
    pub description: String,
    pub mime_type: String,
    pub handler: ResourceHandler,
}

static RESOURCE_REGISTRY: Mutex<Vec<ResourceDef>> = Mutex::new(Vec::new());
static TEMPLATE_REGISTRY: Mutex<Vec<ResourceTemplateDef>> = Mutex::new(Vec::new());

pub fn register_resource(def: ResourceDef) {
    RESOURCE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(def);
}

pub fn register_resource_template(def: ResourceTemplateDef) {
    TEMPLATE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(def);
}

pub(crate) fn with_resources<F, R>(f: F) -> R
where F: FnOnce(&[ResourceDef]) -> R {
    let guard = RESOURCE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    f(&guard)
}

pub(crate) fn with_resource_templates<F, R>(f: F) -> R
where F: FnOnce(&[ResourceTemplateDef]) -> R {
    let guard = TEMPLATE_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    f(&guard)
}
