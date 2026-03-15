use std::sync::{Arc, Mutex};
use serde_json::Value;
use crate::context::ToolContext;
use crate::result::ToolResult;
use crate::tool::{ArgDef, ToolDef, arg_to_schema};

static GROUP_REGISTRY: Mutex<Vec<GroupDef>> = Mutex::new(Vec::new());

pub type CrossRuleFn = Box<dyn Fn(&Value) -> bool + Send + Sync>;

pub struct CrossRule {
    pub check: CrossRuleFn,
    pub message: String,
}

pub struct ActionDef {
    pub name: String,
    pub description: String,
    pub args: Vec<ArgDef>,
    pub handler: Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>,
    pub requires: Vec<String>,
    pub enum_fields: Vec<(String, Vec<String>)>,
    pub cross_rules: Vec<CrossRule>,
}

pub struct GroupDef {
    pub name: String,
    pub description: String,
    pub actions: Vec<ActionDef>,
    pub strategy: String,
}

pub struct GroupBuilder {
    name: String,
    description: String,
    strategy: String,
    actions: Vec<ActionDef>,
}

pub struct ActionBuilder {
    name: String,
    description: String,
    args: Vec<ArgDef>,
    handler: Option<Box<dyn Fn(ToolContext, Value) -> ToolResult + Send + Sync>>,
    requires: Vec<String>,
    enum_fields: Vec<(String, Vec<String>)>,
    cross_rules: Vec<CrossRule>,
}

pub fn tool_group(name: &str) -> GroupBuilder {
    GroupBuilder {
        name: name.to_string(),
        description: String::new(),
        strategy: "union".to_string(),
        actions: Vec::new(),
    }
}

impl GroupBuilder {
    pub fn description(mut self, desc: &str) -> Self {
        self.description = desc.to_string();
        self
    }

    pub fn strategy(mut self, strategy: &str) -> Self {
        self.strategy = strategy.to_string();
        self
    }

    pub fn action(mut self, name: &str, build_fn: impl FnOnce(ActionBuilder) -> ActionBuilder) -> Self {
        let builder = ActionBuilder {
            name: name.to_string(),
            description: String::new(),
            args: Vec::new(),
            handler: None,
            requires: Vec::new(),
            enum_fields: Vec::new(),
            cross_rules: Vec::new(),
        };
        let built = build_fn(builder);
        self.actions.push(ActionDef {
            name: built.name,
            description: built.description,
            args: built.args,
            handler: built.handler.unwrap_or_else(|| Box::new(|_, _| ToolResult::new(""))),
            requires: built.requires,
            enum_fields: built.enum_fields,
            cross_rules: built.cross_rules,
        });
        self
    }

    pub fn register(self) {
        let strategy = self.strategy.clone();
        let gd = GroupDef {
            name: self.name,
            description: self.description,
            actions: self.actions,
            strategy: self.strategy,
        };
        GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).push(gd);

        // Also register the generated ToolDefs into the tool registry
        // so that with_registry() sees them.
        let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
        let group = guard.last().unwrap();
        let tool_defs = if strategy == "separate" {
            group_to_separate_defs(group)
        } else {
            vec![group_to_union_def(group)]
        };
        drop(guard);
        for td in tool_defs {
            crate::tool::push_to_registry(td);
        }
    }
}

impl ActionBuilder {
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

    pub fn requires(mut self, reqs: &[&str]) -> Self {
        self.requires = reqs.iter().map(|s| s.to_string()).collect();
        self
    }

    pub fn enum_field(mut self, field: &str, values: &[&str]) -> Self {
        self.enum_fields.push((field.to_string(), values.iter().map(|s| s.to_string()).collect()));
        self
    }

    pub fn cross_rule<F>(mut self, check: F, message: &str) -> Self
    where
        F: Fn(&Value) -> bool + Send + Sync + 'static,
    {
        self.cross_rules.push(CrossRule {
            check: Box::new(check),
            message: message.to_string(),
        });
        self
    }
}

pub fn groups_to_tool_defs() -> Vec<ToolDef> {
    let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    let mut defs = Vec::new();
    for group in guard.iter() {
        if group.strategy == "separate" {
            defs.extend(group_to_separate_defs(group));
        } else {
            defs.push(group_to_union_def(group));
        }
    }
    defs
}

fn group_to_union_def(group: &GroupDef) -> ToolDef {
    let action_names: Vec<Value> = group.actions.iter()
        .map(|a| Value::String(a.name.clone()))
        .collect();

    let one_of: Vec<Value> = group.actions.iter().map(|act| {
        let mut props = serde_json::Map::new();
        props.insert("action".to_string(), serde_json::json!({"const": act.name}));
        let mut required = vec![Value::String("action".to_string())];
        for arg in &act.args {
            props.insert(arg.name.clone(), arg_to_schema(arg));
            required.push(Value::String(arg.name.clone()));
        }
        serde_json::json!({
            "type": "object",
            "properties": props,
            "required": required,
        })
    }).collect();

    let schema = serde_json::json!({
        "type": "object",
        "properties": {
            "action": {
                "type": "string",
                "enum": action_names,
            }
        },
        "required": ["action"],
        "oneOf": one_of,
    });

    let names: Vec<&str> = group.actions.iter().map(|a| a.name.as_str()).collect();
    let action_list = names.join(", ");
    let desc = if group.description.is_empty() {
        format!("Actions: {}", action_list)
    } else {
        format!("{} Actions: {}", group.description, action_list)
    };

    // Build dispatch closure. We need the action names/handlers accessible.
    // Since GroupDef isn't Clone, we store the group name to look it up at dispatch time.
    let group_name = group.name.clone();
    ToolDef {
        name: group.name.clone(),
        description: desc,
        input_schema: schema,
        handler: Arc::new(move |ctx, args| {
            dispatch_group_action_by_name(&group_name, ctx, args)
        }),
        destructive: false,
        idempotent: false,
        read_only: false,
        open_world: false,
        task_support: false,
        hidden: false,
    }
}

fn group_to_separate_defs(group: &GroupDef) -> Vec<ToolDef> {
    let mut defs = Vec::new();
    for act in &group.actions {
        let mut properties = serde_json::Map::new();
        let mut required = Vec::new();
        for arg in &act.args {
            properties.insert(arg.name.clone(), arg_to_schema(arg));
            required.push(Value::String(arg.name.clone()));
        }
        let mut schema = serde_json::json!({
            "type": "object",
            "properties": properties,
        });
        if !required.is_empty() {
            schema["required"] = Value::Array(required);
        }
        let desc = if act.description.is_empty() {
            format!("{} {}", group.name, act.name)
        } else {
            act.description.clone()
        };

        let group_name = group.name.clone();
        let action_name = act.name.clone();
        defs.push(ToolDef {
            name: format!("{}.{}", group.name, act.name),
            description: desc,
            input_schema: schema,
            handler: Arc::new(move |ctx, args| {
                dispatch_specific_action(&group_name, &action_name, ctx, args)
            }),
            destructive: false,
            idempotent: false,
            read_only: false,
            open_world: false,
            task_support: false,
            hidden: false,
        });
    }
    defs
}

fn dispatch_group_action_by_name(group_name: &str, ctx: ToolContext, args: Value) -> ToolResult {
    let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    let group = guard.iter().find(|g| g.name == group_name);
    match group {
        Some(g) => dispatch_group_action(g, ctx, args),
        None => ToolResult::error(
            format!("Group '{}' not found", group_name),
            "GROUP_NOT_FOUND",
            "",
            false,
        ),
    }
}

fn dispatch_specific_action(group_name: &str, action_name: &str, ctx: ToolContext, args: Value) -> ToolResult {
    let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
    let group = guard.iter().find(|g| g.name == group_name);
    match group {
        Some(g) => {
            let act = g.actions.iter().find(|a| a.name == action_name);
            match act {
                Some(a) => {
                    if let Some(err) = validate_action(a, &args) {
                        return err;
                    }
                    (a.handler)(ctx, args)
                }
                None => ToolResult::error(
                    format!("Action '{}' not found in group '{}'", action_name, group_name),
                    "UNKNOWN_ACTION",
                    "",
                    false,
                ),
            }
        }
        None => ToolResult::error(
            format!("Group '{}' not found", group_name),
            "GROUP_NOT_FOUND",
            "",
            false,
        ),
    }
}

fn validate_action(action: &ActionDef, args: &Value) -> Option<ToolResult> {
    // Check requires
    for field_name in &action.requires {
        let val = args.get(field_name.as_str());
        let is_missing = match val {
            None => true,
            Some(Value::Null) => true,
            Some(Value::String(s)) => s.is_empty(),
            _ => false,
        };
        if is_missing {
            return Some(ToolResult::error(
                format!("Missing required field: {}", field_name),
                "MISSING_REQUIRED",
                "",
                false,
            ));
        }
    }

    // Check enum_fields
    for (field_name, valid_values) in &action.enum_fields {
        if let Some(val) = args.get(field_name.as_str()).and_then(|v| v.as_str()) {
            if !valid_values.iter().any(|v| v == val) {
                let candidates: Vec<&str> = valid_values.iter().map(|s| s.as_str()).collect();
                let suggestion = fuzzy_match(val, &candidates);
                let suggestion_text = match &suggestion {
                    Some(s) => format!(" Did you mean '{}'?", s),
                    None => String::new(),
                };
                return Some(ToolResult::error(
                    format!(
                        "Invalid value '{}' for field '{}'.{} Valid options: {}",
                        val, field_name, suggestion_text, valid_values.join(", ")
                    ),
                    "INVALID_ENUM",
                    "",
                    false,
                ));
            }
        }
    }

    // Check cross_rules
    for rule in &action.cross_rules {
        if (rule.check)(args) {
            return Some(ToolResult::error(
                rule.message.clone(),
                "CROSS_PARAM_VIOLATION",
                "",
                false,
            ));
        }
    }

    None
}

fn dispatch_group_action(group: &GroupDef, ctx: ToolContext, args: Value) -> ToolResult {
    let action_name = match args.get("action").and_then(|v| v.as_str()) {
        Some(name) => name.to_string(),
        None => {
            let names: Vec<&str> = group.actions.iter().map(|a| a.name.as_str()).collect();
            return ToolResult::error(
                format!("Missing 'action' field. Available actions: {}", names.join(", ")),
                "MISSING_ACTION",
                "",
                false,
            );
        }
    };

    let act = group.actions.iter().find(|a| a.name == action_name);
    match act {
        Some(a) => {
            if let Some(err) = validate_action(a, &args) {
                return err;
            }
            (a.handler)(ctx, args)
        }
        None => {
            let names: Vec<&str> = group.actions.iter().map(|a| a.name.as_str()).collect();
            let suggestion = fuzzy_match(&action_name, &names);
            let mut msg = format!("Unknown action '{}'.", action_name);
            if let Some(s) = &suggestion {
                msg.push_str(&format!(" Did you mean '{}'?", s));
            }
            msg.push_str(&format!(" Available actions: {}", names.join(", ")));
            ToolResult::error(
                msg,
                "UNKNOWN_ACTION",
                suggestion.unwrap_or_default(),
                false,
            )
        }
    }
}

fn fuzzy_match(input: &str, candidates: &[&str]) -> Option<String> {
    if candidates.is_empty() {
        return None;
    }
    let input_lower = input.to_lowercase();
    let mut best = String::new();
    let mut best_dist = input.len() + 10;
    for c in candidates {
        let d = levenshtein(&input_lower, &c.to_lowercase());
        if d < best_dist {
            best_dist = d;
            best = c.to_string();
        }
    }
    let threshold = input.len() / 2 + 1;
    if best_dist <= threshold {
        Some(best)
    } else {
        None
    }
}

#[allow(clippy::needless_range_loop)]
fn levenshtein(a: &str, b: &str) -> usize {
    let a_bytes = a.as_bytes();
    let b_bytes = b.as_bytes();
    let la = a_bytes.len();
    let lb = b_bytes.len();
    let mut matrix = vec![vec![0usize; lb + 1]; la + 1];
    for i in 0..=la { matrix[i][0] = i; }
    for j in 0..=lb { matrix[0][j] = j; }
    for i in 1..=la {
        for j in 1..=lb {
            let cost = if a_bytes[i - 1] == b_bytes[j - 1] { 0 } else { 1 };
            matrix[i][j] = (matrix[i - 1][j] + 1)
                .min(matrix[i][j - 1] + 1)
                .min(matrix[i - 1][j - 1] + cost);
        }
    }
    matrix[la][lb]
}

pub fn clear_group_registry() {
    GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner()).clear();
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tool::{clear_registry, TEST_REGISTRY_LOCK};
    use std::sync::Arc;
    use std::sync::atomic::AtomicBool;

    fn lock_and_clear() -> std::sync::MutexGuard<'static, ()> {
        let guard = TEST_REGISTRY_LOCK.lock().unwrap_or_else(|e| e.into_inner());
        clear_group_registry();
        clear_registry();
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
    fn test_group_registration() {
        let _lock = lock_and_clear();
        tool_group("math")
            .description("Math operations")
            .action("add", |a| {
                a.description("Add two numbers")
                    .arg(ArgDef::int("a"))
                    .arg(ArgDef::int("b"))
                    .handler(|_, _| ToolResult::new("sum"))
            })
            .action("multiply", |a| {
                a.description("Multiply two numbers")
                    .arg(ArgDef::int("x"))
                    .arg(ArgDef::int("y"))
                    .handler(|_, _| ToolResult::new("product"))
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            assert_eq!(guard.len(), 1);
            assert_eq!(guard[0].name, "math");
            assert_eq!(guard[0].description, "Math operations");
            assert_eq!(guard[0].actions.len(), 2);
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_union_strategy_schema() {
        let _lock = lock_and_clear();
        tool_group("db")
            .description("DB ops")
            .action("query", |a| {
                a.description("Run query").arg(ArgDef::string("sql"))
            })
            .action("insert", |a| {
                a.description("Insert record")
                    .arg(ArgDef::string("table"))
                    .arg(ArgDef::object("data"))
            })
            .register();

        let defs = groups_to_tool_defs();
        assert_eq!(defs.len(), 1);
        assert_eq!(defs[0].name, "db");

        let schema = &defs[0].input_schema;
        let props = schema["properties"].as_object().unwrap();
        let action_prop = props["action"].as_object().unwrap();
        let enum_vals = action_prop["enum"].as_array().unwrap();
        assert_eq!(enum_vals.len(), 2);

        let one_of = schema["oneOf"].as_array().unwrap();
        assert_eq!(one_of.len(), 2);

        let entry0 = one_of[0].as_object().unwrap();
        let entry0_props = entry0["properties"].as_object().unwrap();
        assert_eq!(entry0_props["action"]["const"], "query");
        assert!(entry0_props.contains_key("sql"));

        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_separate_strategy_schema() {
        let _lock = lock_and_clear();
        tool_group("files")
            .description("File ops")
            .strategy("separate")
            .action("read", |a| {
                a.description("Read a file").arg(ArgDef::string("path"))
            })
            .action("write", |a| {
                a.description("Write a file")
                    .arg(ArgDef::string("path"))
                    .arg(ArgDef::string("content"))
            })
            .register();

        let defs = groups_to_tool_defs();
        assert_eq!(defs.len(), 2);
        let names: Vec<&str> = defs.iter().map(|d| d.name.as_str()).collect();
        assert!(names.contains(&"files.read"));
        assert!(names.contains(&"files.write"));

        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_dispatch_correct_action() {
        let _lock = lock_and_clear();
        tool_group("calc")
            .action("add", |a| {
                a.arg(ArgDef::int("a"))
                    .arg(ArgDef::int("b"))
                    .handler(|_, args| {
                        let a = args["a"].as_i64().unwrap_or(0);
                        let b = args["b"].as_i64().unwrap_or(0);
                        ToolResult::new(format!("{}", a + b))
                    })
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "add",
                "a": 3,
                "b": 4,
            }));
            assert!(!result.is_error);
            assert_eq!(result.result_text, "7");
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_dispatch_unknown_action() {
        let _lock = lock_and_clear();
        tool_group("calc2")
            .action("add", |a| a.handler(|_, _| ToolResult::new("ok")))
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "ad",
            }));
            assert!(result.is_error);
            assert!(result.result_text.contains("Unknown action"));
            assert!(result.result_text.contains("add"));
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_dispatch_missing_action() {
        let _lock = lock_and_clear();
        tool_group("calc3")
            .action("add", |a| a.handler(|_, _| ToolResult::new("ok")))
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({}));
            assert!(result.is_error);
            assert!(result.result_text.contains("Missing"));
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_groups_in_tool_defs() {
        let _lock = lock_and_clear();
        tool_group("tools_test")
            .description("Test group")
            .action("ping", |a| a.handler(|_, _| ToolResult::new("pong")))
            .register();

        crate::tool::with_registry(|tools| {
            let found = tools.iter().any(|d| d.name == "tools_test");
            assert!(found);
        });

        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_validation_requires() {
        let _lock = lock_and_clear();
        tool_group("val_req")
            .action("create", |a| {
                a.requires(&["name", "email"])
                    .handler(|_, _| ToolResult::new("ok"))
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            // Missing required field
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "create",
                "name": "Alice",
            }));
            assert!(result.is_error);
            assert!(result.result_text.contains("Missing required field: email"));
            assert_eq!(result.error_code, "MISSING_REQUIRED");

            // All fields present
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "create",
                "name": "Alice",
                "email": "alice@example.com",
            }));
            assert!(!result.is_error);
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_validation_enum_field() {
        let _lock = lock_and_clear();
        tool_group("val_enum")
            .action("set_mode", |a| {
                a.enum_field("mode", &["fast", "slow", "balanced"])
                    .handler(|_, _| ToolResult::new("ok"))
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            // Invalid enum value
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "set_mode",
                "mode": "turbo",
            }));
            assert!(result.is_error);
            assert!(result.result_text.contains("Invalid value 'turbo'"));
            assert_eq!(result.error_code, "INVALID_ENUM");

            // Valid enum value
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "set_mode",
                "mode": "fast",
            }));
            assert!(!result.is_error);
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_validation_cross_rule() {
        let _lock = lock_and_clear();
        tool_group("val_cross")
            .action("transfer", |a| {
                a.cross_rule(
                    |args| {
                        let from = args.get("from").and_then(|v| v.as_str()).unwrap_or("");
                        let to = args.get("to").and_then(|v| v.as_str()).unwrap_or("");
                        from == to
                    },
                    "Source and destination cannot be the same",
                )
                .handler(|_, _| ToolResult::new("transferred"))
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            // Violates cross rule
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "transfer",
                "from": "A",
                "to": "A",
            }));
            assert!(result.is_error);
            assert!(result.result_text.contains("Source and destination cannot be the same"));
            assert_eq!(result.error_code, "CROSS_PARAM_VIOLATION");

            // Valid
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "transfer",
                "from": "A",
                "to": "B",
            }));
            assert!(!result.is_error);
            assert_eq!(result.result_text, "transferred");
        }
        clear_group_registry();
        clear_registry();
    }

    #[test]
    fn test_validation_enum_fuzzy_suggestion() {
        let _lock = lock_and_clear();
        tool_group("val_fuzzy")
            .action("color", |a| {
                a.enum_field("color", &["red", "green", "blue"])
                    .handler(|_, _| ToolResult::new("ok"))
            })
            .register();

        {
            let guard = GROUP_REGISTRY.lock().unwrap_or_else(|e| e.into_inner());
            let result = dispatch_group_action(&guard[0], dummy_ctx(), serde_json::json!({
                "action": "color",
                "color": "gren",
            }));
            assert!(result.is_error);
            assert!(result.result_text.contains("Did you mean"));
        }
        clear_group_registry();
        clear_registry();
    }
}
