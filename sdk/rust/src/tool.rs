use std::sync::{Arc, Mutex};
use serde_json::Value;
use crate::context::ToolContext;
use crate::result::ToolResult;

static REGISTRY: Mutex<Vec<ToolDef>> = Mutex::new(Vec::new());

pub struct ArgDef {
    pub name: String,
    pub type_name: String,
    pub item_type: Option<String>,
    pub union_types: Option<Vec<String>>,
    pub enum_values: Option<Vec<String>>,
}

impl ArgDef {
    pub fn int(name: &str) -> Self { Self { name: name.to_string(), type_name: "integer".to_string(), item_type: None, union_types: None, enum_values: None } }
    pub fn string(name: &str) -> Self { Self { name: name.to_string(), type_name: "string".to_string(), item_type: None, union_types: None, enum_values: None } }
    pub fn number(name: &str) -> Self { Self { name: name.to_string(), type_name: "number".to_string(), item_type: None, union_types: None, enum_values: None } }
    pub fn boolean(name: &str) -> Self { Self { name: name.to_string(), type_name: "boolean".to_string(), item_type: None, union_types: None, enum_values: None } }

    pub fn array(name: &str, item_type: &str) -> Self {
        Self { name: name.to_string(), type_name: "array".to_string(), item_type: Some(item_type.to_string()), union_types: None, enum_values: None }
    }

    pub fn object(name: &str) -> Self {
        Self { name: name.to_string(), type_name: "object".to_string(), item_type: None, union_types: None, enum_values: None }
    }

    pub fn union(name: &str, types: &[&str]) -> Self {
        Self { name: name.to_string(), type_name: "union".to_string(), item_type: None, union_types: Some(types.iter().map(|s| s.to_string()).collect()), enum_values: None }
    }

    pub fn literal(name: &str, values: &[&str]) -> Self {
        Self { name: name.to_string(), type_name: "literal".to_string(), item_type: None, union_types: None, enum_values: Some(values.iter().map(|s| s.to_string()).collect()) }
    }
}

pub struct ToolDef {
    pub name: String,
    pub description: String,
    pub input_schema: Value,
    pub handler: Arc<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>,
    pub destructive: bool,
    pub idempotent: bool,
    pub read_only: bool,
    pub open_world: bool,
    pub task_support: bool,
    pub hidden: bool,
}

pub struct ToolBuilder {
    name: String,
    description: String,
    args: Vec<ArgDef>,
    handler: Option<Arc<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>>,
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

pub fn arg_to_schema(arg: &ArgDef) -> Value {
    match arg.type_name.as_str() {
        "array" => {
            let mut schema = serde_json::Map::new();
            schema.insert("type".to_string(), Value::String("array".to_string()));
            if let Some(ref item_type) = arg.item_type {
                let mut items = serde_json::Map::new();
                items.insert("type".to_string(), Value::String(item_type.clone()));
                schema.insert("items".to_string(), Value::Object(items));
            }
            Value::Object(schema)
        }
        "union" => {
            if let Some(ref types) = arg.union_types {
                let any_of: Vec<Value> = types.iter().map(|t| {
                    serde_json::json!({"type": t})
                }).collect();
                serde_json::json!({"anyOf": any_of})
            } else {
                serde_json::json!({"type": "string"})
            }
        }
        "literal" => {
            if let Some(ref values) = arg.enum_values {
                let enum_vals: Vec<Value> = values.iter().map(|v| Value::String(v.clone())).collect();
                serde_json::json!({"type": "string", "enum": enum_vals})
            } else {
                serde_json::json!({"type": "string"})
            }
        }
        _ => {
            serde_json::json!({"type": arg.type_name})
        }
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
        self.handler = Some(Arc::new(f));
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
            properties.insert(arg.name.clone(), arg_to_schema(arg));
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
            handler: self.handler.unwrap_or_else(|| Arc::new(|_, _| ToolResult::new(""))),
            destructive: self.destructive,
            idempotent: self.idempotent,
            read_only: self.read_only,
            open_world: self.open_world,
            task_support: self.task_support,
            hidden: false,
        };

        REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(td);
    }
}

pub(crate) fn with_registry<F, R>(f: F) -> R
where
    F: FnOnce(&[ToolDef]) -> R,
{
    let guard = REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    f(&guard)
}

/// Adds a ToolDef directly to the tool registry (used by group registration).
pub(crate) fn push_to_registry(td: ToolDef) {
    REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(td);
}

pub fn clear_registry() {
    REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}

/// Test-only lock to serialize tests that mutate the global REGISTRY.
/// Multiple modules (tool, group, workflow) push into the same REGISTRY,
/// so all their tests must hold this lock to avoid races.
#[cfg(test)]
pub(crate) static TEST_REGISTRY_LOCK: Mutex<()> = Mutex::new(());

#[cfg(test)]
mod tests {
    use super::*;

    fn lock_and_clear() -> std::sync::MutexGuard<'static, ()> {
        let guard = TEST_REGISTRY_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        clear_registry();
        guard
    }

    #[test]
    fn test_tool_registration() {
        let _lock = lock_and_clear();
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
    fn test_array_arg() {
        let _lock = lock_and_clear();
        tool("list_items")
            .description("List items")
            .arg(ArgDef::array("tags", "string"))
            .handler(|_, _| ToolResult::new("ok"))
            .register();

        with_registry(|tools| {
            let schema = &tools[0].input_schema;
            let props = schema["properties"].as_object().unwrap();
            let tags = props["tags"].as_object().unwrap();
            assert_eq!(tags["type"], "array");
            assert_eq!(tags["items"]["type"], "string");
        });
        clear_registry();
    }

    #[test]
    fn test_object_arg() {
        let _lock = lock_and_clear();
        tool("set_config")
            .description("Set config")
            .arg(ArgDef::object("config"))
            .handler(|_, _| ToolResult::new("ok"))
            .register();

        with_registry(|tools| {
            let schema = &tools[0].input_schema;
            let props = schema["properties"].as_object().unwrap();
            let config = props["config"].as_object().unwrap();
            assert_eq!(config["type"], "object");
        });
        clear_registry();
    }

    #[test]
    fn test_union_arg() {
        let _lock = lock_and_clear();
        tool("process")
            .description("Process data")
            .arg(ArgDef::union("data", &["string", "object"]))
            .handler(|_, _| ToolResult::new("ok"))
            .register();

        with_registry(|tools| {
            let schema = &tools[0].input_schema;
            let props = schema["properties"].as_object().unwrap();
            let data = props["data"].as_object().unwrap();
            let any_of = data["anyOf"].as_array().unwrap();
            assert_eq!(any_of.len(), 2);
            assert_eq!(any_of[0]["type"], "string");
            assert_eq!(any_of[1]["type"], "object");
        });
        clear_registry();
    }

    #[test]
    fn test_literal_arg() {
        let _lock = lock_and_clear();
        tool("set_mode")
            .description("Set mode")
            .arg(ArgDef::literal("mode", &["fast", "slow", "balanced"]))
            .handler(|_, _| ToolResult::new("ok"))
            .register();

        with_registry(|tools| {
            let schema = &tools[0].input_schema;
            let props = schema["properties"].as_object().unwrap();
            let mode = props["mode"].as_object().unwrap();
            assert_eq!(mode["type"], "string");
            let enum_vals = mode["enum"].as_array().unwrap();
            assert_eq!(enum_vals.len(), 3);
            assert_eq!(enum_vals[0], "fast");
            assert_eq!(enum_vals[1], "slow");
            assert_eq!(enum_vals[2], "balanced");
        });
        clear_registry();
    }

    #[test]
    fn test_tool_metadata() {
        let _lock = lock_and_clear();
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

    #[test]
    fn test_hidden_defaults_to_false() {
        let _lock = lock_and_clear();
        tool("my_tool")
            .description("A tool")
            .handler(|_, _| ToolResult::new("ok"))
            .register();

        with_registry(|tools| {
            assert_eq!(tools.len(), 1);
            assert!(!tools[0].hidden, "hidden should default to false");
        });
        clear_registry();
    }

    #[test]
    fn test_hidden_field_set_by_direct_construction() {
        let _lock = lock_and_clear();
        // Since there's no builder method for hidden, verify the field
        // is accessible and can be set directly on ToolDef
        let td = ToolDef {
            name: "secret".to_string(),
            description: "A hidden tool".to_string(),
            input_schema: serde_json::json!({"type": "object", "properties": {}}),
            handler: Arc::new(|_, _| ToolResult::new("secret")),
            destructive: false,
            idempotent: false,
            read_only: false,
            open_world: false,
            task_support: false,
            hidden: true,
        };
        assert!(td.hidden, "hidden should be true when set explicitly");

        let td2 = ToolDef {
            name: "visible".to_string(),
            description: "A visible tool".to_string(),
            input_schema: serde_json::json!({"type": "object", "properties": {}}),
            handler: Arc::new(|_, _| ToolResult::new("visible")),
            destructive: false,
            idempotent: false,
            read_only: false,
            open_world: false,
            task_support: false,
            hidden: false,
        };
        assert!(!td2.hidden, "hidden should be false when set explicitly");
    }
}
