use std::sync::Mutex;
use serde_json::Value;
use crate::context::ToolContext;
use crate::result::ToolResult;

static REGISTRY: Mutex<Vec<ToolDef>> = Mutex::new(Vec::new());

pub struct ArgDef {
    pub name: String,
    pub type_name: String,
}

impl ArgDef {
    pub fn int(name: &str) -> Self { Self { name: name.to_string(), type_name: "integer".to_string() } }
    pub fn string(name: &str) -> Self { Self { name: name.to_string(), type_name: "string".to_string() } }
    pub fn number(name: &str) -> Self { Self { name: name.to_string(), type_name: "number".to_string() } }
    pub fn boolean(name: &str) -> Self { Self { name: name.to_string(), type_name: "boolean".to_string() } }
}

pub struct ToolDef {
    pub name: String,
    pub description: String,
    pub input_schema: Value,
    pub handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>,
    pub destructive: bool,
    pub idempotent: bool,
    pub read_only: bool,
    pub open_world: bool,
    pub task_support: bool,
}

pub struct ToolBuilder {
    name: String,
    description: String,
    args: Vec<ArgDef>,
    handler: Option<Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>>,
    destructive: bool,
    idempotent: bool,
    read_only: bool,
    open_world: bool,
    task_support: bool,
}

pub fn tool(name: &str) -> ToolBuilder {
    ToolBuilder {
        name: name.to_string(),
        description: String::new(),
        args: Vec::new(),
        handler: None,
        destructive: false,
        idempotent: false,
        read_only: false,
        open_world: false,
        task_support: false,
    }
}

impl ToolBuilder {
    pub fn description(mut self, desc: &str) -> Self {
        self.description = desc.to_string();
        self
    }

    pub fn arg(mut self, arg: ArgDef) -> Self {
        self.args.push(arg);
        self
    }

    pub fn handler<F>(mut self, f: F) -> Self
    where
        F: Fn(ToolContext, Value) -> ToolResult + Send + Sync + 'static,
    {
        self.handler = Some(Box::new(f));
        self
    }

    pub fn destructive_hint(mut self, v: bool) -> Self { self.destructive = v; self }
    pub fn idempotent_hint(mut self, v: bool) -> Self { self.idempotent = v; self }
    pub fn read_only_hint(mut self, v: bool) -> Self { self.read_only = v; self }
    pub fn open_world_hint(mut self, v: bool) -> Self { self.open_world = v; self }
    pub fn task_support_hint(mut self, v: bool) -> Self { self.task_support = v; self }

    pub fn register(self) {
        let mut properties = serde_json::Map::new();
        let mut required = Vec::new();
        for arg in &self.args {
            let mut prop = serde_json::Map::new();
            prop.insert("type".to_string(), Value::String(arg.type_name.clone()));
            properties.insert(arg.name.clone(), Value::Object(prop));
            required.push(Value::String(arg.name.clone()));
        }

        let input_schema = serde_json::json!({
            "type": "object",
            "properties": properties,
            "required": required,
        });

        let td = ToolDef {
            name: self.name,
            description: self.description,
            input_schema,
            handler: self.handler.unwrap_or_else(|| Box::new(|_, _| ToolResult::new(""))),
            destructive: self.destructive,
            idempotent: self.idempotent,
            read_only: self.read_only,
            open_world: self.open_world,
            task_support: self.task_support,
        };

        REGISTRY.lock().unwrap().push(td);
    }
}

pub(crate) fn with_registry<F, R>(f: F) -> R
where
    F: FnOnce(&[ToolDef]) -> R,
{
    let guard = REGISTRY.lock().unwrap();
    f(&guard)
}

pub fn clear_registry() {
    REGISTRY.lock().unwrap().clear();
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_tool_registration() {
        clear_registry();
        tool("add")
            .description("Add two numbers")
            .arg(ArgDef::int("a"))
            .arg(ArgDef::int("b"))
            .handler(|_, _| ToolResult::new("3"))
            .register();

        with_registry(|tools| {
            assert_eq!(tools.len(), 1);
            assert_eq!(tools[0].name, "add");
            assert_eq!(tools[0].description, "Add two numbers");
        });
        clear_registry();
    }

    #[test]
    fn test_tool_metadata() {
        clear_registry();
        tool("delete_user")
            .description("Delete a user")
            .destructive_hint(true)
            .handler(|_, _| ToolResult::new("deleted"))
            .register();

        with_registry(|tools| {
            assert!(tools[0].destructive);
        });
        clear_registry();
    }
}
