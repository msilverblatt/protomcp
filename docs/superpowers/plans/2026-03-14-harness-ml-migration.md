# Harness-ML Migration: Make protomcp the best MCP framework for complex servers

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 8 features across ALL 4 protomcp SDKs (Python, Go, TypeScript, Rust) that eliminate boilerplate in complex MCP servers — tool groups, local middleware, union type schemas, server context, telemetry, validation, sidecar management, and handler auto-discovery.

**Architecture:** Each feature is a new module per SDK with its own registry and API. Features integrate via each SDK's `runner` (dispatch) and `tool` (schema generation). All SDKs produce identical MCP protocol behavior — the only differences are language idioms. Python is the reference implementation; Go/TS/Rust follow the same design.

**Tech Stack:**
- **Python**: `typing` module, pytest
- **Go**: functional options, `testing` package
- **TypeScript**: Zod schemas, vitest
- **Rust**: builder pattern, `#[cfg(test)]` inline tests
- **All**: protobuf (no proto changes — features map to existing `ToolDefinition`)

**Spec:** `/tmp/protomcp/docs/plans/harness-ml-migration.md`

---

## SDK Idiom Map

| Concept | Python | Go | TypeScript | Rust |
|---------|--------|----|------------|------|
| Registration | `@decorator` | `FuncName(opts...)` | `funcName({opts})` | `func_name().method().register()` |
| Schema source | type hints | manual `ArgDef` builders | Zod validators | manual `ArgDef` builders |
| Test framework | pytest | `testing` | vitest | `#[cfg(test)]` |
| Runner | sync loop | sync loop | async loop | async (tokio) |
| Exports | `__init__.py` `__all__` | package-level functions | `index.ts` re-exports | `lib.rs` `pub use` |
| Discovery | Yes (importlib) | No (compiled) | Yes (dynamic import) | No (compiled) |

---

## File Structure

### New files to create

**Python SDK:**

| File | Responsibility |
|------|---------------|
| `sdk/python/src/protomcp/group.py` | `@tool_group` / `@action` decorators, registry, schema gen (union + separate) |
| `sdk/python/src/protomcp/local_middleware.py` | `@local_middleware` decorator, chain builder |
| `sdk/python/src/protomcp/server_context.py` | `@server_context` decorator, resolver registry |
| `sdk/python/src/protomcp/telemetry.py` | `@telemetry_sink` decorator, `ToolCallEvent` dataclass |
| `sdk/python/src/protomcp/sidecar.py` | `@sidecar` decorator, process lifecycle |
| `sdk/python/src/protomcp/discovery.py` | `configure(handlers_dir=...)`, module scanner |
| `sdk/python/tests/test_schema_types.py` | Union/complex type schema tests |
| `sdk/python/tests/test_group.py` | Tool group tests |
| `sdk/python/tests/test_group_validation.py` | Validation tests |
| `sdk/python/tests/test_local_middleware.py` | Local middleware tests |
| `sdk/python/tests/test_server_context.py` | Server context tests |
| `sdk/python/tests/test_telemetry.py` | Telemetry tests |
| `sdk/python/tests/test_sidecar.py` | Sidecar tests |
| `sdk/python/tests/test_discovery.py` | Discovery tests |

**Go SDK:**

| File | Responsibility |
|------|---------------|
| `sdk/go/protomcp/group.go` | `ToolGroup()` + `Action()` builders, registry, schema gen |
| `sdk/go/protomcp/group_test.go` | Tool group tests |
| `sdk/go/protomcp/local_middleware.go` | `LocalMiddleware()` registration, chain builder |
| `sdk/go/protomcp/local_middleware_test.go` | Local middleware tests |
| `sdk/go/protomcp/server_context.go` | `ServerContext()` registration, resolver |
| `sdk/go/protomcp/server_context_test.go` | Server context tests |
| `sdk/go/protomcp/telemetry.go` | `TelemetrySink()` registration, `ToolCallEvent` |
| `sdk/go/protomcp/telemetry_test.go` | Telemetry tests |
| `sdk/go/protomcp/sidecar.go` | `Sidecar()` builder, process lifecycle |
| `sdk/go/protomcp/sidecar_test.go` | Sidecar tests |

**TypeScript SDK:**

| File | Responsibility |
|------|---------------|
| `sdk/typescript/src/group.ts` | `toolGroup()` function, action schemas via Zod |
| `sdk/typescript/src/group.test.ts` | Tool group tests |
| `sdk/typescript/src/localMiddleware.ts` | `localMiddleware()` function, chain builder |
| `sdk/typescript/src/localMiddleware.test.ts` | Local middleware tests |
| `sdk/typescript/src/serverContext.ts` | `serverContext()` function, resolver registry |
| `sdk/typescript/src/serverContext.test.ts` | Server context tests |
| `sdk/typescript/src/telemetry.ts` | `telemetrySink()`, `ToolCallEvent` |
| `sdk/typescript/src/telemetry.test.ts` | Telemetry tests |
| `sdk/typescript/src/sidecar.ts` | `sidecar()` function, process lifecycle |
| `sdk/typescript/src/sidecar.test.ts` | Sidecar tests |
| `sdk/typescript/src/discovery.ts` | `configure({handlersDir})`, dynamic import scanner |
| `sdk/typescript/src/discovery.test.ts` | Discovery tests |

**Rust SDK:**

| File | Responsibility |
|------|---------------|
| `sdk/rust/src/group.rs` | `tool_group()` builder, action registration |
| `sdk/rust/src/local_middleware.rs` | `local_middleware()` registration, chain builder |
| `sdk/rust/src/server_context.rs` | `server_context()` registration, resolver |
| `sdk/rust/src/telemetry.rs` | `telemetry_sink()` registration, `ToolCallEvent` |
| `sdk/rust/src/sidecar.rs` | `sidecar()` builder, process lifecycle |

**Examples and Docs:**

| File | Responsibility |
|------|---------------|
| `examples/python/tool_groups.py` | Python tool groups example |
| `examples/python/advanced_server.py` | Python middleware + telemetry + context example |
| `examples/go/tool_groups/main.go` | Go tool groups example |
| `examples/typescript/tool_groups.ts` | TypeScript tool groups example |
| `examples/rust/tool_groups/main.rs` | Rust tool groups example |

### Files to modify

| File | Changes |
|------|---------|
| `sdk/python/src/protomcp/tool.py` | Replace `_python_type_to_json` with `_type_to_schema` |
| `sdk/python/src/protomcp/runner.py` | Group dispatch, middleware chain, context, telemetry, sidecars, discovery |
| `sdk/python/src/protomcp/__init__.py` | Export new APIs |
| `sdk/go/protomcp/tool.go` | Add `ArrayArg`, `ObjectArg`, `UnionArg`, `LiteralArg`; include groups in `GetRegisteredTools` |
| `sdk/go/protomcp/runner.go` | Group dispatch, middleware chain, context, telemetry, sidecars |
| `sdk/typescript/src/tool.ts` | Include groups in `getRegisteredTools` |
| `sdk/typescript/src/runner.ts` | Group dispatch, middleware chain, context, telemetry, sidecars, discovery |
| `sdk/typescript/src/index.ts` | Export new APIs |
| `sdk/rust/src/tool.rs` | Add `array`, `object`, `union_of`, `literal` to `ArgDef`; include groups |
| `sdk/rust/src/runner.rs` | Group dispatch, middleware chain, context, telemetry, sidecars |
| `sdk/rust/src/lib.rs` | Export new modules |
| `docs/src/content/docs/guides/writing-tools-python.mdx` | All new sections |
| `docs/src/content/docs/guides/writing-tools-go.mdx` | All new sections |
| `docs/src/content/docs/guides/writing-tools-typescript.mdx` | All new sections |
| `docs/src/content/docs/guides/writing-tools-rust.mdx` | All new sections |
| `docs/src/content/docs/reference/python-api.mdx` | New API entries |

---

## Chunk 1: Union Type Schemas + Tool Groups

### Task 1: Python — Recursive type-to-schema (Item 3)

**Files:**
- Create: `sdk/python/tests/test_schema_types.py`
- Modify: `sdk/python/src/protomcp/tool.py`

- [ ] **Step 1: Write failing tests for union types**

```python
# sdk/python/tests/test_schema_types.py
import json
from protomcp.tool import _type_to_schema

def test_str_schema():
    assert _type_to_schema(str) == {"type": "string"}

def test_int_schema():
    assert _type_to_schema(int) == {"type": "integer"}

def test_float_schema():
    assert _type_to_schema(float) == {"type": "number"}

def test_bool_schema():
    assert _type_to_schema(bool) == {"type": "boolean"}

def test_plain_list():
    assert _type_to_schema(list) == {"type": "array"}

def test_plain_dict():
    assert _type_to_schema(dict) == {"type": "object"}

def test_list_str():
    assert _type_to_schema(list[str]) == {"type": "array", "items": {"type": "string"}}

def test_list_int():
    assert _type_to_schema(list[int]) == {"type": "array", "items": {"type": "integer"}}

def test_list_dict():
    assert _type_to_schema(list[dict]) == {"type": "array", "items": {"type": "object"}}

def test_dict_str_any():
    from typing import Any
    result = _type_to_schema(dict[str, Any])
    assert result == {"type": "object", "additionalProperties": {}}

def test_dict_str_str():
    result = _type_to_schema(dict[str, str])
    assert result == {"type": "object", "additionalProperties": {"type": "string"}}

def test_optional_str():
    from typing import Optional
    result = _type_to_schema(Optional[str])
    assert result == {"anyOf": [{"type": "string"}, {"type": "null"}]}

def test_union_str_dict():
    result = _type_to_schema(str | dict)
    assert result == {"anyOf": [{"type": "string"}, {"type": "object"}]}

def test_union_str_dict_none():
    result = _type_to_schema(str | dict | None)
    assert result == {"anyOf": [{"type": "string"}, {"type": "object"}, {"type": "null"}]}

def test_union_list_str_list_dict_none():
    result = _type_to_schema(list[str] | list[dict] | None)
    assert result == {
        "anyOf": [
            {"type": "array", "items": {"type": "string"}},
            {"type": "array", "items": {"type": "object"}},
            {"type": "null"},
        ]
    }

def test_literal():
    from typing import Literal
    result = _type_to_schema(Literal["a", "b", "c"])
    assert result == {"type": "string", "enum": ["a", "b", "c"]}

def test_literal_ints():
    from typing import Literal
    result = _type_to_schema(Literal[1, 2, 3])
    assert result == {"enum": [1, 2, 3]}

def test_nested_list_of_list():
    result = _type_to_schema(list[list[str]])
    assert result == {"type": "array", "items": {"type": "array", "items": {"type": "string"}}}

def test_unknown_type_defaults_to_string():
    class Custom:
        pass
    assert _type_to_schema(Custom) == {"type": "string"}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_schema_types.py -v`
Expected: FAIL — `_type_to_schema` does not exist

- [ ] **Step 3: Implement `_type_to_schema` in tool.py**

Replace `_python_type_to_json`, `_PYTHON_TYPE_TO_JSON_SCHEMA` with:

```python
import typing
from typing import Any, get_origin, get_args

_PRIMITIVE_TYPE_MAP = {
    str: "string",
    int: "integer",
    float: "number",
    bool: "boolean",
    list: "array",
    dict: "object",
}

def _type_to_schema(hint) -> dict:
    """Convert a Python type hint to a JSON Schema dict."""
    origin = get_origin(hint)
    args = get_args(hint)

    if origin is typing.Union:
        non_none = [a for a in args if a is not type(None)]
        schemas = [_type_to_schema(a) for a in non_none]
        if type(None) in args:
            schemas.append({"type": "null"})
        if len(schemas) == 1:
            return schemas[0]
        return {"anyOf": schemas}

    if origin is list:
        if args:
            return {"type": "array", "items": _type_to_schema(args[0])}
        return {"type": "array"}

    if origin is dict:
        schema: dict[str, Any] = {"type": "object"}
        if len(args) == 2:
            schema["additionalProperties"] = _type_to_schema(args[1])
        return schema

    if origin is typing.Literal:
        types = set(type(v).__name__ for v in args)
        if len(types) == 1:
            t = type(args[0])
            json_type = _PRIMITIVE_TYPE_MAP.get(t)
            if json_type:
                return {"type": json_type, "enum": list(args)}
        return {"enum": list(args)}

    json_type = _PRIMITIVE_TYPE_MAP.get(hint)
    if json_type:
        return {"type": json_type}
    return {"type": "string"}
```

Update `_generate_schema` and `_generate_dataclass_schema` to call `_type_to_schema(hint)` instead of `{"type": _python_type_to_json(hint)}`. Remove `_python_type_to_json`.

- [ ] **Step 4: Run tests**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_schema_types.py sdk/python/tests/test_tool.py sdk/python/tests/test_tool_advanced.py -v`
Expected: All PASS

- [ ] **Step 5: Write integration test — tool with union params**

Add to `test_schema_types.py`:

```python
from protomcp.tool import clear_registry, get_registered_tools
from protomcp import tool, ToolResult

def test_tool_with_union_params():
    clear_registry()

    @tool("Test union types")
    def process(data: str | dict | None = None, items: list[str] | list[dict] | None = None) -> ToolResult:
        return ToolResult(result="ok")

    tools = get_registered_tools()
    schema = json.loads(tools[0].input_schema_json)
    assert schema["properties"]["data"] == {
        "anyOf": [{"type": "string"}, {"type": "object"}, {"type": "null"}],
        "default": None,
    }

def test_tool_with_literal_param():
    clear_registry()
    from typing import Literal

    @tool("Test literal")
    def choose(mode: Literal["fast", "accurate", "balanced"]) -> ToolResult:
        return ToolResult(result=mode)

    tools = get_registered_tools()
    schema = json.loads(tools[0].input_schema_json)
    assert schema["properties"]["mode"] == {"type": "string", "enum": ["fast", "accurate", "balanced"]}
    assert schema["required"] == ["mode"]
```

- [ ] **Step 6: Run all tests, commit**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_schema_types.py -v`

```bash
git add sdk/python/src/protomcp/tool.py sdk/python/tests/test_schema_types.py
git commit -m "feat(python-sdk): recursive type-to-schema with union, list[T], dict[K,V], Literal support"
```

---

### Task 2: Go — Complex arg type builders (Item 3)

**Files:**
- Modify: `sdk/go/protomcp/tool.go`
- Modify: `sdk/go/protomcp/tool_test.go`

- [ ] **Step 1: Write failing tests for new arg types**

Add to `sdk/go/protomcp/tool_test.go`:

```go
func TestArrayArg(t *testing.T) {
    ClearRegistry()
    Tool("test", Args(ArrayArg("items", "string")))
    tools := GetRegisteredTools()
    schema := tools[0].InputSchema
    props := schema["properties"].(map[string]interface{})
    items := props["items"].(map[string]interface{})
    if items["type"] != "array" { t.Fatal("expected array") }
    innerItems := items["items"].(map[string]interface{})
    if innerItems["type"] != "string" { t.Fatal("expected string items") }
}

func TestObjectArg(t *testing.T) {
    ClearRegistry()
    Tool("test", Args(ObjectArg("config")))
    tools := GetRegisteredTools()
    props := tools[0].InputSchema["properties"].(map[string]interface{})
    if props["config"].(map[string]interface{})["type"] != "object" { t.Fatal("expected object") }
}

func TestUnionArg(t *testing.T) {
    ClearRegistry()
    Tool("test", Args(UnionArg("data", "string", "object")))
    tools := GetRegisteredTools()
    props := tools[0].InputSchema["properties"].(map[string]interface{})
    data := props["data"].(map[string]interface{})
    anyOf := data["anyOf"].([]interface{})
    if len(anyOf) != 2 { t.Fatal("expected 2 anyOf entries") }
}

func TestLiteralArg(t *testing.T) {
    ClearRegistry()
    Tool("test", Args(LiteralArg("mode", "fast", "slow", "balanced")))
    tools := GetRegisteredTools()
    props := tools[0].InputSchema["properties"].(map[string]interface{})
    mode := props["mode"].(map[string]interface{})
    if mode["type"] != "string" { t.Fatal("expected string") }
    enum := mode["enum"].([]interface{})
    if len(enum) != 3 { t.Fatal("expected 3 enum values") }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp && cd sdk/go && go test ./protomcp/ -run TestArrayArg -v`
Expected: FAIL — functions don't exist

- [ ] **Step 3: Add new ArgDef builders to tool.go**

```go
func ArrayArg(name string, itemType string) ArgDef {
    return ArgDef{Name: name, Type: "array", ItemType: itemType}
}

func ObjectArg(name string) ArgDef {
    return ArgDef{Name: name, Type: "object"}
}

func UnionArg(name string, types ...string) ArgDef {
    return ArgDef{Name: name, Type: "union", UnionTypes: types}
}

func LiteralArg(name string, values ...string) ArgDef {
    return ArgDef{Name: name, Type: "literal", EnumValues: values}
}
```

Update `ArgDef` struct:
```go
type ArgDef struct {
    Name       string
    Type       string
    ItemType   string   // for array: item type name
    UnionTypes []string // for union: list of JSON Schema types
    EnumValues []string // for literal: enum values
}
```

Update `Args()` function's schema building to handle the new types:
```go
func Args(args ...ArgDef) ToolOption {
    return func(td *ToolDef) {
        props := map[string]interface{}{}
        required := []string{}
        for _, a := range args {
            props[a.Name] = argToSchema(a)
            required = append(required, a.Name)
        }
        td.InputSchema = map[string]interface{}{
            "type": "object", "properties": props, "required": required,
        }
    }
}

func argToSchema(a ArgDef) map[string]interface{} {
    switch a.Type {
    case "array":
        schema := map[string]interface{}{"type": "array"}
        if a.ItemType != "" {
            schema["items"] = map[string]interface{}{"type": a.ItemType}
        }
        return schema
    case "union":
        anyOf := make([]interface{}, len(a.UnionTypes))
        for i, t := range a.UnionTypes {
            anyOf[i] = map[string]interface{}{"type": t}
        }
        return map[string]interface{}{"anyOf": anyOf}
    case "literal":
        vals := make([]interface{}, len(a.EnumValues))
        for i, v := range a.EnumValues {
            vals[i] = v
        }
        return map[string]interface{}{"type": "string", "enum": vals}
    default:
        return map[string]interface{}{"type": a.Type}
    }
}
```

- [ ] **Step 4: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp/sdk/go && go test ./protomcp/ -v`

```bash
git add sdk/go/protomcp/tool.go sdk/go/protomcp/tool_test.go
git commit -m "feat(go-sdk): ArrayArg, ObjectArg, UnionArg, LiteralArg for complex schema types"
```

---

### Task 3: TypeScript — Verify Zod already handles union schemas (Item 3)

TypeScript uses Zod + `zodToJsonSchema` which already supports `z.union()`, `z.array()`, `z.literal()`, etc. This task verifies it works and adds test coverage.

**Files:**
- Modify: `sdk/typescript/src/tool.test.ts`

- [ ] **Step 1: Write verification tests**

```typescript
// Add to sdk/typescript/src/tool.test.ts
import { z } from 'zod';
import { tool, getRegisteredTools, clearRegistry } from './tool.js';

describe('complex schema types', () => {
  beforeEach(() => clearRegistry());

  it('generates union schema', () => {
    tool({
      name: 'test',
      description: 'test',
      args: z.object({ data: z.union([z.string(), z.object({}).passthrough()]) }),
      handler: () => 'ok',
    });
    const schema = JSON.parse(getRegisteredTools()[0].inputSchemaJson);
    expect(schema.properties.data.anyOf).toBeDefined();
    expect(schema.properties.data.anyOf.length).toBe(2);
  });

  it('generates array of string schema', () => {
    tool({
      name: 'test',
      description: 'test',
      args: z.object({ items: z.array(z.string()) }),
      handler: () => 'ok',
    });
    const schema = JSON.parse(getRegisteredTools()[0].inputSchemaJson);
    expect(schema.properties.items.type).toBe('array');
    expect(schema.properties.items.items.type).toBe('string');
  });

  it('generates literal/enum schema', () => {
    tool({
      name: 'test',
      description: 'test',
      args: z.object({ mode: z.enum(['fast', 'slow', 'balanced']) }),
      handler: () => 'ok',
    });
    const schema = JSON.parse(getRegisteredTools()[0].inputSchemaJson);
    expect(schema.properties.mode.enum).toEqual(['fast', 'slow', 'balanced']);
  });

  it('generates nullable schema', () => {
    tool({
      name: 'test',
      description: 'test',
      args: z.object({ value: z.string().nullable() }),
      handler: () => 'ok',
    });
    const schema = JSON.parse(getRegisteredTools()[0].inputSchemaJson);
    expect(schema.properties.value.anyOf || schema.properties.value.nullable).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run tests**

Run: `cd /Users/msilverblatt/hotmcp/sdk/typescript && npx vitest run --reporter=verbose`
Expected: All PASS (Zod + zodToJsonSchema already handles these)

- [ ] **Step 3: Commit**

```bash
git add sdk/typescript/src/tool.test.ts
git commit -m "test(ts-sdk): verify Zod handles union, array, literal, nullable schema generation"
```

---

### Task 4: Rust — Complex arg type builders (Item 3)

**Files:**
- Modify: `sdk/rust/src/tool.rs`

- [ ] **Step 1: Write failing tests**

Add to the `#[cfg(test)]` block in `sdk/rust/src/tool.rs`:

```rust
#[test]
fn test_array_arg() {
    clear_registry();
    tool("test")
        .description("test")
        .arg(ArgDef::array("items", "string"))
        .handler(|_, _| ToolResult::new("ok"))
        .register();
    with_registry(|tools| {
        let schema = &tools[0].input_schema;
        let items_schema = &schema["properties"]["items"];
        assert_eq!(items_schema["type"], "array");
        assert_eq!(items_schema["items"]["type"], "string");
    });
    clear_registry();
}

#[test]
fn test_union_arg() {
    clear_registry();
    tool("test")
        .description("test")
        .arg(ArgDef::union("data", &["string", "object"]))
        .handler(|_, _| ToolResult::new("ok"))
        .register();
    with_registry(|tools| {
        let schema = &tools[0].input_schema;
        let data = &schema["properties"]["data"];
        assert!(data["anyOf"].is_array());
        assert_eq!(data["anyOf"].as_array().unwrap().len(), 2);
    });
    clear_registry();
}

#[test]
fn test_literal_arg() {
    clear_registry();
    tool("test")
        .description("test")
        .arg(ArgDef::literal("mode", &["fast", "slow"]))
        .handler(|_, _| ToolResult::new("ok"))
        .register();
    with_registry(|tools| {
        let schema = &tools[0].input_schema;
        let mode = &schema["properties"]["mode"];
        assert_eq!(mode["type"], "string");
        assert_eq!(mode["enum"].as_array().unwrap().len(), 2);
    });
    clear_registry();
}
```

- [ ] **Step 2: Implement new ArgDef constructors**

Add to `sdk/rust/src/tool.rs`:

```rust
impl ArgDef {
    pub fn array(name: &str, item_type: &str) -> Self {
        Self { name: name.to_string(), type_name: "array".to_string(),
               item_type: Some(item_type.to_string()), union_types: None, enum_values: None }
    }
    pub fn union(name: &str, types: &[&str]) -> Self {
        Self { name: name.to_string(), type_name: "union".to_string(),
               item_type: None, union_types: Some(types.iter().map(|s| s.to_string()).collect()), enum_values: None }
    }
    pub fn literal(name: &str, values: &[&str]) -> Self {
        Self { name: name.to_string(), type_name: "literal".to_string(),
               item_type: None, union_types: None, enum_values: Some(values.iter().map(|s| s.to_string()).collect()) }
    }
    pub fn object(name: &str) -> Self {
        Self { name: name.to_string(), type_name: "object".to_string(),
               item_type: None, union_types: None, enum_values: None }
    }
}
```

Update `ArgDef` struct to add optional fields and update `register()` schema builder to use `arg_to_schema()` helper.

- [ ] **Step 3: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp/sdk/rust && cargo test`

```bash
git add sdk/rust/src/tool.rs
git commit -m "feat(rust-sdk): array, object, union, literal ArgDef constructors for complex schemas"
```

---

### Task 5: Python — Tool group decorator and registry (Item 1)

**Files:**
- Create: `sdk/python/src/protomcp/group.py`
- Create: `sdk/python/tests/test_group.py`

- [ ] **Step 1: Write failing tests**

```python
# sdk/python/tests/test_group.py
import json
from protomcp.group import tool_group, action, get_registered_groups, clear_group_registry, groups_to_tool_defs

def test_register_group():
    clear_group_registry()

    @tool_group("data", description="Manage data")
    class DataTools:
        @action("add", description="Add a dataset")
        def add(self, data_path: str) -> str:
            return f"added {data_path}"

        @action("profile", description="Profile the dataset")
        def profile(self, category: str | None = None) -> str:
            return "profiled"

    groups = get_registered_groups()
    assert len(groups) == 1
    assert groups[0].name == "data"
    assert len(groups[0].actions) == 2

def test_action_schemas():
    clear_group_registry()

    @tool_group("models", description="Manage models")
    class ModelTools:
        @action("create", description="Create a model")
        def create(self, name: str, model_type: str = "regressor") -> str:
            return f"created {name}"

    groups = get_registered_groups()
    a = groups[0].actions[0]
    assert a.input_schema["properties"]["model_type"] == {"type": "string", "default": "regressor"}
    assert a.input_schema["required"] == ["name"]

def test_union_strategy_schema():
    clear_group_registry()

    @tool_group("data", description="Manage data", strategy="union")
    class DataTools:
        @action("add", description="Add")
        def add(self, data_path: str, auto_clean: bool = False) -> str:
            return "added"
        @action("profile", description="Profile")
        def profile(self, category: str | None = None) -> str:
            return "profiled"

    tool_defs = groups_to_tool_defs()
    assert len(tool_defs) == 1
    schema = json.loads(tool_defs[0].input_schema_json)
    assert schema["properties"]["action"]["enum"] == ["add", "profile"]
    assert "oneOf" in schema
    assert len(schema["oneOf"]) == 2

def test_separate_strategy_schema():
    clear_group_registry()

    @tool_group("models", description="Manage models", strategy="separate")
    class ModelTools:
        @action("create", description="Create")
        def create(self, name: str) -> str:
            return "created"
        @action("list", description="List")
        def list_models(self) -> str:
            return "listed"

    tool_defs = groups_to_tool_defs()
    assert len(tool_defs) == 2
    assert {td.name for td in tool_defs} == {"models.create", "models.list"}

def test_dispatch():
    clear_group_registry()

    @tool_group("calc", description="Calculator")
    class CalcTools:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> str:
            return str(a + b)

    groups = get_registered_groups()
    from protomcp.group import _dispatch_group_action
    assert _dispatch_group_action(groups[0], action="add", a=2, b=3) == "5"

def test_dispatch_unknown_action():
    clear_group_registry()

    @tool_group("calc", description="Calculator")
    class CalcTools:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> str:
            return str(a + b)

    groups = get_registered_groups()
    from protomcp.group import _dispatch_group_action
    from protomcp.result import ToolResult
    result = _dispatch_group_action(groups[0], action="subtract", a=2, b=3)
    assert isinstance(result, ToolResult)
    assert result.is_error

def test_groups_in_registered_tools():
    from protomcp.tool import get_registered_tools, clear_registry
    clear_registry()
    clear_group_registry()

    @tool_group("calc", description="Calculator")
    class CalcTools:
        @action("add", description="Add")
        def add(self, a: int, b: int) -> str:
            return str(a + b)

    tools = get_registered_tools()
    assert any(t.name == "calc" for t in tools)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_group.py -v`

- [ ] **Step 3: Implement group.py**

Create `sdk/python/src/protomcp/group.py` with:
- `ActionDef` dataclass: `name`, `description`, `handler`, `input_schema`, `requires`, `enum_fields`, `cross_rules`, `hints`
- `GroupDef` dataclass: `name`, `description`, `actions`, `instance`, `strategy`, tool metadata pass-throughs
- `@action(name, description, ...)` — marks method, stores `_action_def` on the function
- `@tool_group(name, description, strategy, ...)` — instantiates class, collects `@action` methods, generates per-action schemas using `_type_to_schema`, registers in `_group_registry`
- `groups_to_tool_defs()` — converts groups to `ToolDef` list (union or separate strategy)
- `_group_to_union_tool(group)` — single tool with `oneOf` discriminated schema
- `_group_to_separate_tools(group)` — one tool per action with namespaced names
- `_dispatch_group_action(group, **kwargs)` — parse `action` from kwargs, dispatch to correct handler with fuzzy suggestions on unknown action
- `_fuzzy_suggest(value, valid)` — `difflib.get_close_matches`

Update `sdk/python/src/protomcp/tool.py` `get_registered_tools()`:
```python
def get_registered_tools() -> list[ToolDef]:
    from protomcp.group import get_registered_groups, groups_to_tool_defs
    return list(_registry) + groups_to_tool_defs()
```

Update `sdk/python/src/protomcp/__init__.py` — add exports for `tool_group`, `action`, `get_registered_groups`, `clear_group_registry`.

- [ ] **Step 4: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_group.py -v`

```bash
git add sdk/python/src/protomcp/group.py sdk/python/tests/test_group.py sdk/python/src/protomcp/tool.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): tool groups with union/separate strategies and per-action schemas"
```

---

### Task 6: Go — Tool groups (Item 1)

**Files:**
- Create: `sdk/go/protomcp/group.go`
- Create: `sdk/go/protomcp/group_test.go`
- Modify: `sdk/go/protomcp/tool.go` (`GetRegisteredTools` includes groups)

- [ ] **Step 1: Write failing tests**

```go
// sdk/go/protomcp/group_test.go
package protomcp

import (
    "encoding/json"
    "testing"
)

func TestToolGroupRegistration(t *testing.T) {
    ClearGroupRegistry()
    ToolGroup("data",
        GroupDescription("Manage data"),
        Action("add", ActionDescription("Add data"),
            ActionArgs(StrArg("data_path")),
            ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
                return ToolResult{ResultText: "added"}
            }),
        ),
        Action("profile", ActionDescription("Profile data"),
            ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
                return ToolResult{ResultText: "profiled"}
            }),
        ),
    )
    groups := GetRegisteredGroups()
    if len(groups) != 1 { t.Fatalf("expected 1 group, got %d", len(groups)) }
    if groups[0].Name != "data" { t.Fatalf("expected name 'data', got '%s'", groups[0].Name) }
    if len(groups[0].Actions) != 2 { t.Fatalf("expected 2 actions, got %d", len(groups[0].Actions)) }
}

func TestGroupUnionSchema(t *testing.T) {
    ClearGroupRegistry()
    ClearRegistry()
    ToolGroup("data",
        GroupDescription("Manage data"),
        Action("add", ActionDescription("Add"), ActionArgs(StrArg("path"))),
        Action("profile", ActionDescription("Profile")),
    )
    tools := GetRegisteredTools()
    var found bool
    for _, td := range tools {
        if td.Name == "data" {
            found = true
            var schema map[string]interface{}
            json.Unmarshal([]byte(td.InputSchemaJSON()), &schema)
            if _, ok := schema["oneOf"]; !ok { t.Fatal("expected oneOf") }
            props := schema["properties"].(map[string]interface{})
            action := props["action"].(map[string]interface{})
            if action["type"] != "string" { t.Fatal("expected action type string") }
        }
    }
    if !found { t.Fatal("data tool not found in registry") }
}

func TestGroupSeparateStrategy(t *testing.T) {
    ClearGroupRegistry()
    ClearRegistry()
    ToolGroup("models",
        GroupDescription("Models"),
        GroupStrategy("separate"),
        Action("create", ActionDescription("Create"), ActionArgs(StrArg("name"))),
        Action("list", ActionDescription("List")),
    )
    tools := GetRegisteredTools()
    names := map[string]bool{}
    for _, td := range tools { names[td.Name] = true }
    if !names["models.create"] || !names["models.list"] {
        t.Fatalf("expected models.create and models.list, got %v", names)
    }
}

func TestGroupDispatch(t *testing.T) {
    ClearGroupRegistry()
    ToolGroup("calc",
        GroupDescription("Calculator"),
        Action("add", ActionDescription("Add"), ActionArgs(IntArg("a"), IntArg("b")),
            ActionHandler(func(ctx ToolContext, args map[string]interface{}) ToolResult {
                a := int(args["a"].(float64))
                b := int(args["b"].(float64))
                return ToolResult{ResultText: fmt.Sprintf("%d", a+b)}
            }),
        ),
    )
    groups := GetRegisteredGroups()
    result := DispatchGroupAction(groups[0], map[string]interface{}{"action": "add", "a": float64(2), "b": float64(3)})
    if result.ResultText != "5" { t.Fatalf("expected '5', got '%s'", result.ResultText) }
}
```

- [ ] **Step 2: Implement group.go**

Create `sdk/go/protomcp/group.go` with:
- `GroupDef` struct: `Name`, `Description`, `Actions []ActionDef`, `Strategy`
- `ActionDef` struct: `Name`, `Description`, `Args []ArgDef`, `Handler`, `Requires`, `EnumFields`
- `ToolGroup(name, opts...)` registration function with functional options
- `GroupDescription`, `GroupStrategy`, `Action(name, opts...)` option functions
- `ActionDescription`, `ActionArgs`, `ActionHandler` option functions
- `GroupsToToolDefs()` — converts to `[]ToolDef` (union or separate)
- `DispatchGroupAction(group, args)` — dispatches by `action` field
- Update `GetRegisteredTools()` in `tool.go` to include `GroupsToToolDefs()`

- [ ] **Step 3: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp/sdk/go && go test ./protomcp/ -v`

```bash
git add sdk/go/protomcp/group.go sdk/go/protomcp/group_test.go sdk/go/protomcp/tool.go
git commit -m "feat(go-sdk): tool groups with union/separate strategies and dispatch"
```

---

### Task 7: TypeScript — Tool groups (Item 1)

**Files:**
- Create: `sdk/typescript/src/group.ts`
- Create: `sdk/typescript/src/group.test.ts`
- Modify: `sdk/typescript/src/tool.ts` (`getRegisteredTools` includes groups)
- Modify: `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write failing tests**

```typescript
// sdk/typescript/src/group.test.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { z } from 'zod';
import { toolGroup, getRegisteredGroups, clearGroupRegistry, groupsToToolDefs } from './group.js';
import { clearRegistry, getRegisteredTools } from './tool.js';

describe('tool groups', () => {
  beforeEach(() => { clearGroupRegistry(); clearRegistry(); });

  it('registers a group with actions', () => {
    toolGroup({
      name: 'data',
      description: 'Manage data',
      actions: {
        add: { description: 'Add data', args: z.object({ path: z.string() }),
               handler: (args) => 'added' },
        profile: { description: 'Profile', args: z.object({}),
                   handler: () => 'profiled' },
      },
    });
    const groups = getRegisteredGroups();
    expect(groups).toHaveLength(1);
    expect(groups[0].name).toBe('data');
    expect(Object.keys(groups[0].actions)).toHaveLength(2);
  });

  it('generates union schema by default', () => {
    toolGroup({
      name: 'data',
      description: 'Data',
      actions: {
        add: { description: 'Add', args: z.object({ path: z.string() }), handler: () => 'ok' },
        profile: { description: 'Profile', args: z.object({}), handler: () => 'ok' },
      },
    });
    const defs = groupsToToolDefs();
    expect(defs).toHaveLength(1);
    const schema = JSON.parse(defs[0].inputSchemaJson);
    expect(schema.properties.action.enum).toEqual(['add', 'profile']);
    expect(schema.oneOf).toHaveLength(2);
  });

  it('generates separate tools with strategy=separate', () => {
    toolGroup({
      name: 'models',
      description: 'Models',
      strategy: 'separate',
      actions: {
        create: { description: 'Create', args: z.object({ name: z.string() }), handler: () => 'ok' },
        list: { description: 'List', args: z.object({}), handler: () => 'ok' },
      },
    });
    const defs = groupsToToolDefs();
    expect(defs).toHaveLength(2);
    expect(defs.map(d => d.name).sort()).toEqual(['models.create', 'models.list']);
  });

  it('appears in getRegisteredTools', () => {
    toolGroup({
      name: 'calc',
      description: 'Calc',
      actions: { add: { description: 'Add', args: z.object({ a: z.number(), b: z.number() }), handler: (args) => String(args.a + args.b) } },
    });
    const tools = getRegisteredTools();
    expect(tools.some(t => t.name === 'calc')).toBe(true);
  });
});
```

- [ ] **Step 2: Implement group.ts**

Create `sdk/typescript/src/group.ts` with:
- `ActionOptions` interface: `description`, `args` (ZodObject), `handler`, `requires?`, `enumFields?`
- `GroupOptions` interface: `name`, `description`, `actions` (Record of ActionOptions), `strategy?`
- `GroupDef` interface stored in registry
- `toolGroup(options)` — registers group, generates per-action schemas via `zodToJsonSchema`
- `groupsToToolDefs()` — converts groups to `ToolDef[]`
- Union strategy: builds `oneOf` with `action` discriminator
- Separate strategy: builds `name.action` namespaced tools
- Dispatch handler: parses `action` from args, routes to correct handler
- Update `getRegisteredTools()` in `tool.ts` to include `groupsToToolDefs()`
- Update `index.ts` to export `toolGroup`, `getRegisteredGroups`, `clearGroupRegistry`

- [ ] **Step 3: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp/sdk/typescript && npx vitest run --reporter=verbose`

```bash
git add sdk/typescript/src/group.ts sdk/typescript/src/group.test.ts sdk/typescript/src/tool.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): tool groups with Zod-based per-action schemas and union/separate strategies"
```

---

### Task 8: Rust — Tool groups (Item 1)

**Files:**
- Create: `sdk/rust/src/group.rs`
- Modify: `sdk/rust/src/tool.rs` (include groups)
- Modify: `sdk/rust/src/lib.rs` (export)

- [ ] **Step 1: Write failing tests**

In `sdk/rust/src/group.rs`:

```rust
#[cfg(test)]
mod tests {
    use super::*;
    use crate::tool::{ArgDef, clear_registry};
    use crate::result::ToolResult;

    #[test]
    fn test_group_registration() {
        clear_group_registry();
        tool_group("data")
            .description("Manage data")
            .action("add", |a| a
                .description("Add data")
                .arg(ArgDef::string("path"))
                .handler(|_, _| ToolResult::new("added")))
            .action("profile", |a| a
                .description("Profile")
                .handler(|_, _| ToolResult::new("profiled")))
            .register();

        with_group_registry(|groups| {
            assert_eq!(groups.len(), 1);
            assert_eq!(groups[0].name, "data");
            assert_eq!(groups[0].actions.len(), 2);
        });
        clear_group_registry();
    }

    #[test]
    fn test_group_union_schema() {
        clear_group_registry();
        clear_registry();
        tool_group("data")
            .description("Data")
            .action("add", |a| a.description("Add").arg(ArgDef::string("path")).handler(|_, _| ToolResult::new("ok")))
            .action("profile", |a| a.description("Profile").handler(|_, _| ToolResult::new("ok")))
            .register();

        // groups_to_tool_defs included in with_registry via tool.rs
        crate::tool::with_registry(|tools| {
            let data = tools.iter().find(|t| t.name == "data").expect("data tool");
            let schema = &data.input_schema;
            assert!(schema["oneOf"].is_array());
            assert_eq!(schema["properties"]["action"]["type"], "string");
        });
        clear_group_registry();
        clear_registry();
    }
}
```

- [ ] **Step 2: Implement group.rs**

Create `sdk/rust/src/group.rs` with:
- `GroupDef` struct, `ActionDef` struct
- `tool_group(name) -> GroupBuilder` — builder pattern
- `GroupBuilder`: `.description()`, `.action(name, |builder| ...)`, `.strategy()`, `.register()`
- `ActionBuilder`: `.description()`, `.arg()`, `.handler()`
- `groups_to_tool_defs()` — returns `Vec<ToolDef>`
- Union schema: `oneOf` with action discriminator
- Separate: namespaced `group.action` tools
- Dispatch: match on `action` field in args JSON
- Update `lib.rs` to `pub mod group;` and export

- [ ] **Step 3: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp/sdk/rust && cargo test`

```bash
git add sdk/rust/src/group.rs sdk/rust/src/tool.rs sdk/rust/src/lib.rs
git commit -m "feat(rust-sdk): tool groups with builder pattern and union/separate strategies"
```

---

### Task 9: Examples and docs for Chunk 1

**Files:**
- Create: `examples/python/tool_groups.py`
- Create: `examples/go/tool_groups/main.go`
- Create: `examples/typescript/tool_groups.ts`
- Modify: `docs/src/content/docs/guides/writing-tools-python.mdx`
- Modify: `docs/src/content/docs/guides/writing-tools-go.mdx`
- Modify: `docs/src/content/docs/guides/writing-tools-typescript.mdx`
- Modify: `docs/src/content/docs/guides/writing-tools-rust.mdx`
- Modify: `docs/src/content/docs/reference/python-api.mdx`

- [ ] **Step 1: Create Python example** (`examples/python/tool_groups.py`)
- [ ] **Step 2: Create Go example** (`examples/go/tool_groups/main.go`)
- [ ] **Step 3: Create TypeScript example** (`examples/typescript/tool_groups.ts`)
- [ ] **Step 4: Update all 4 language guides** — add union types table and tool groups section
- [ ] **Step 5: Update Python API reference** — add `@tool_group`, `@action`, `GroupDef`, `ActionDef`
- [ ] **Step 6: Commit**

```bash
git add examples/ docs/
git commit -m "docs: union types, tool groups across all SDKs with examples"
```

---

## Chunk 2: Declarative Validation + Server Context

### Task 10: Python — Declarative per-action validation (Item 6)

**Files:**
- Create: `sdk/python/tests/test_group_validation.py`
- Modify: `sdk/python/src/protomcp/group.py`

- [ ] **Step 1: Write failing tests**

Test `requires`, `enum_fields` (with fuzzy match), `cross_rules`, and `hints` on `@action`. See original spec for full test code covering:
- Missing required param → error with param name
- Valid required param → passes through
- Invalid enum value → error with "Did you mean?"
- Cross-param rule violation → error
- Hints appended to successful result

- [ ] **Step 2: Add validation to `_dispatch_group_action`**

Add `_validate_action(action_def, kwargs)` and `_collect_hints(action_def, kwargs)` to group.py. Run validation before calling handler; collect hints after.

- [ ] **Step 3: Run tests, commit**

Run: `cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/test_group_validation.py -v`

```bash
git add sdk/python/src/protomcp/group.py sdk/python/tests/test_group_validation.py
git commit -m "feat(python-sdk): declarative per-action validation with requires, enum_fields, cross_rules, hints"
```

### Task 11: Go — Declarative validation (Item 6)

**Files:**
- Modify: `sdk/go/protomcp/group.go`
- Modify: `sdk/go/protomcp/group_test.go`

- [ ] **Step 1: Write tests for validation**

Test `Requires("field")`, `EnumField("strategy", "median", "mean", "mode")`, `CrossRule(fn, msg)` options on `Action()`.

- [ ] **Step 2: Implement validation in DispatchGroupAction**

Add `ActionRequires(fields...)`, `ActionEnumField(name, values...)`, `ActionCrossRule(fn, msg)` option functions. Validate before dispatch. Use Levenshtein or simple substring match for fuzzy suggestions.

- [ ] **Step 3: Run tests, commit**

```bash
git add sdk/go/protomcp/group.go sdk/go/protomcp/group_test.go
git commit -m "feat(go-sdk): declarative per-action validation with requires, enum fields, cross rules"
```

### Task 12: TypeScript — Declarative validation (Item 6)

**Files:**
- Modify: `sdk/typescript/src/group.ts`
- Modify: `sdk/typescript/src/group.test.ts`

- [ ] **Step 1: Write tests, implement**

Add `requires`, `enumFields`, `crossRules`, `hints` to `ActionOptions`. TypeScript already has Zod validation for basic type checking, but declarative requires/enum/cross-rules add MCP-specific validation with structured error responses. Implement validation in the dispatch function.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/group.ts sdk/typescript/src/group.test.ts
git commit -m "feat(ts-sdk): declarative per-action validation with requires, enumFields, crossRules"
```

### Task 13: Rust — Declarative validation (Item 6)

**Files:**
- Modify: `sdk/rust/src/group.rs`

- [ ] **Step 1: Write tests, implement**

Add `.requires(&["field"])`, `.enum_field("name", &["a", "b"])`, `.cross_rule(fn, msg)` to `ActionBuilder`. Validate in dispatch. Use `strsim` crate or simple Levenshtein for fuzzy matching.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/rust/src/group.rs
git commit -m "feat(rust-sdk): declarative per-action validation"
```

---

### Task 14: Python — Server context (Item 4)

**Files:**
- Create: `sdk/python/src/protomcp/server_context.py`
- Create: `sdk/python/tests/test_server_context.py`
- Modify: `sdk/python/src/protomcp/runner.py`
- Modify: `sdk/python/src/protomcp/group.py`
- Modify: `sdk/python/src/protomcp/__init__.py`

- [ ] **Step 1: Write failing tests**

Test: register resolver, `resolve_contexts` pops from args, default values, `expose=True/False`, multiple contexts.

- [ ] **Step 2: Implement server_context.py**

`@server_context(param_name, expose=True)` — decorator that registers `ContextDef(param_name, resolver, expose)`.
`resolve_contexts(args)` — runs all resolvers, returns dict.
`get_hidden_context_params()` — params with `expose=False`.

- [ ] **Step 3: Integrate into runner.py and group.py**

In `_handle_call_tool`: call `resolve_contexts(args)`, inject into handler kwargs.
In `_dispatch_group_action`: same pattern.

- [ ] **Step 4: Run tests, commit**

```bash
git add sdk/python/src/protomcp/server_context.py sdk/python/tests/test_server_context.py sdk/python/src/protomcp/runner.py sdk/python/src/protomcp/group.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): server context resolvers for shared params like project_dir"
```

### Task 15: Go — Server context (Item 4)

**Files:**
- Create: `sdk/go/protomcp/server_context.go`
- Create: `sdk/go/protomcp/server_context_test.go`
- Modify: `sdk/go/protomcp/runner.go`

- [ ] **Step 1: Write tests, implement**

`ServerContext(paramName, resolver, opts...)` — registers a resolver.
`ResolveContexts(args)` — runs resolvers, pops from args, returns resolved map.
Integrate into `handleCallTool` and `DispatchGroupAction`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/go/protomcp/server_context.go sdk/go/protomcp/server_context_test.go sdk/go/protomcp/runner.go
git commit -m "feat(go-sdk): server context resolvers"
```

### Task 16: TypeScript — Server context (Item 4)

**Files:**
- Create: `sdk/typescript/src/serverContext.ts`
- Create: `sdk/typescript/src/serverContext.test.ts`
- Modify: `sdk/typescript/src/runner.ts`, `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write tests, implement**

`serverContext(paramName, resolver, {expose})` function.
`resolveContexts(args)` function.
Integrate into runner dispatch.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/serverContext.ts sdk/typescript/src/serverContext.test.ts sdk/typescript/src/runner.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): server context resolvers"
```

### Task 17: Rust — Server context (Item 4)

**Files:**
- Create: `sdk/rust/src/server_context.rs`
- Modify: `sdk/rust/src/runner.rs`, `sdk/rust/src/lib.rs`

- [ ] **Step 1: Write tests, implement**

`server_context(param_name, resolver)` registration.
`resolve_contexts(args)` function.
Integrate into runner dispatch.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/rust/src/server_context.rs sdk/rust/src/runner.rs sdk/rust/src/lib.rs
git commit -m "feat(rust-sdk): server context resolvers"
```

### Task 18: Docs for Chunk 2

- [ ] **Step 1: Add validation sections to all 4 language guides**
- [ ] **Step 2: Add server context sections to all 4 language guides**
- [ ] **Step 3: Update Python API reference**
- [ ] **Step 4: Commit**

```bash
git add docs/
git commit -m "docs: declarative validation and server context across all SDKs"
```

---

## Chunk 3: Local Middleware + Telemetry

### Task 19: Python — Local middleware (Item 2)

**Files:**
- Create: `sdk/python/src/protomcp/local_middleware.py`
- Create: `sdk/python/tests/test_local_middleware.py`
- Modify: `sdk/python/src/protomcp/runner.py`
- Modify: `sdk/python/src/protomcp/__init__.py`

- [ ] **Step 1: Write failing tests**

Test: registration, priority ordering, chain execution order, arg modification, short-circuit, exception catching, empty chain.

- [ ] **Step 2: Implement local_middleware.py**

`@local_middleware(priority=N)` — registers `LocalMiddlewareDef(priority, handler)`.
`build_middleware_chain(tool_name, handler)` — builds chain sorted by priority, each calls `next_handler(ctx, args)`.
Chain signature: `(ctx, args_dict) -> ToolResult`.

- [ ] **Step 3: Integrate into runner.py**

Wrap handler invocation with `build_middleware_chain` in `_handle_call_tool`.

- [ ] **Step 4: Run tests, commit**

```bash
git add sdk/python/src/protomcp/local_middleware.py sdk/python/tests/test_local_middleware.py sdk/python/src/protomcp/runner.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): local middleware with priority-ordered chain"
```

### Task 20: Go — Local middleware (Item 2)

**Files:**
- Create: `sdk/go/protomcp/local_middleware.go`
- Create: `sdk/go/protomcp/local_middleware_test.go`
- Modify: `sdk/go/protomcp/runner.go`

- [ ] **Step 1: Write tests, implement**

`LocalMiddleware(priority, handler)` — handler signature: `func(ctx ToolContext, toolName string, args map[string]interface{}, next func(ToolContext, map[string]interface{}) ToolResult) ToolResult`.
`BuildMiddlewareChain(toolName, handler)` — returns chain function.
Integrate into `handleCallTool`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/go/protomcp/local_middleware.go sdk/go/protomcp/local_middleware_test.go sdk/go/protomcp/runner.go
git commit -m "feat(go-sdk): local middleware with priority-ordered chain"
```

### Task 21: TypeScript — Local middleware (Item 2)

**Files:**
- Create: `sdk/typescript/src/localMiddleware.ts`
- Create: `sdk/typescript/src/localMiddleware.test.ts`
- Modify: `sdk/typescript/src/runner.ts`, `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write tests, implement**

`localMiddleware(priority, handler)` — handler: `(ctx, toolName, args, next) => any`.
`buildMiddlewareChain(toolName, handler)` function.
Integrate into runner's callTool handling.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/localMiddleware.ts sdk/typescript/src/localMiddleware.test.ts sdk/typescript/src/runner.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): local middleware with priority-ordered chain"
```

### Task 22: Rust — Local middleware (Item 2)

**Files:**
- Create: `sdk/rust/src/local_middleware.rs`
- Modify: `sdk/rust/src/runner.rs`, `sdk/rust/src/lib.rs`

- [ ] **Step 1: Write tests, implement**

`local_middleware(priority, handler)` — handler: `Box<dyn Fn(ToolContext, &str, Value, &dyn Fn(ToolContext, Value) -> ToolResult) -> ToolResult>`.
`build_middleware_chain(tool_name, handler)` function.
Integrate into `handle_call_tool`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/rust/src/local_middleware.rs sdk/rust/src/runner.rs sdk/rust/src/lib.rs
git commit -m "feat(rust-sdk): local middleware with priority-ordered chain"
```

---

### Task 23: Python — Telemetry sinks (Item 5)

**Files:**
- Create: `sdk/python/src/protomcp/telemetry.py`
- Create: `sdk/python/tests/test_telemetry.py`
- Modify: `sdk/python/src/protomcp/runner.py`
- Modify: `sdk/python/src/protomcp/__init__.py`

- [ ] **Step 1: Write failing tests**

Test: registration, emit start/success/error/progress, sink failure swallowed, multiple sinks.

- [ ] **Step 2: Implement telemetry.py**

`ToolCallEvent` dataclass: `tool_name`, `phase`, `args`, `action`, `result`, `error`, `duration_ms`, `progress`, `total`, `message`.
`@telemetry_sink` — registers sink function.
`emit_telemetry(event)` — calls all sinks in try/except (fail-safe).

- [ ] **Step 3: Integrate into runner.py**

Emit `start` before handler, `success`/`error` after, with timing.

- [ ] **Step 4: Run tests, commit**

```bash
git add sdk/python/src/protomcp/telemetry.py sdk/python/tests/test_telemetry.py sdk/python/src/protomcp/runner.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): telemetry sinks with fail-safe ToolCallEvent emission"
```

### Task 24: Go — Telemetry (Item 5)

**Files:**
- Create: `sdk/go/protomcp/telemetry.go`
- Create: `sdk/go/protomcp/telemetry_test.go`
- Modify: `sdk/go/protomcp/runner.go`

- [ ] **Step 1: Write tests, implement**

`ToolCallEvent` struct. `TelemetrySink(handler)` registration. `EmitTelemetry(event)` — fail-safe.
Integrate into `handleCallTool`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/go/protomcp/telemetry.go sdk/go/protomcp/telemetry_test.go sdk/go/protomcp/runner.go
git commit -m "feat(go-sdk): telemetry sinks"
```

### Task 25: TypeScript — Telemetry (Item 5)

**Files:**
- Create: `sdk/typescript/src/telemetry.ts`
- Create: `sdk/typescript/src/telemetry.test.ts`
- Modify: `sdk/typescript/src/runner.ts`, `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write tests, implement**

`ToolCallEvent` interface. `telemetrySink(handler)` function. `emitTelemetry(event)` — fail-safe.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/telemetry.ts sdk/typescript/src/telemetry.test.ts sdk/typescript/src/runner.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): telemetry sinks"
```

### Task 26: Rust — Telemetry (Item 5)

**Files:**
- Create: `sdk/rust/src/telemetry.rs`
- Modify: `sdk/rust/src/runner.rs`, `sdk/rust/src/lib.rs`

- [ ] **Step 1: Write tests, implement**

`ToolCallEvent` struct. `telemetry_sink(handler)` registration. `emit_telemetry(event)` — fail-safe.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/rust/src/telemetry.rs sdk/rust/src/runner.rs sdk/rust/src/lib.rs
git commit -m "feat(rust-sdk): telemetry sinks"
```

### Task 27: Examples and docs for Chunk 3

**Files:**
- Create: `examples/python/advanced_server.py`
- Modify: all 4 language guide docs
- Modify: `docs/src/content/docs/reference/python-api.mdx`

- [ ] **Step 1: Create Python advanced_server.py example** — middleware + telemetry + context
- [ ] **Step 2: Add local middleware section to all 4 language guides**
- [ ] **Step 3: Add telemetry section to all 4 language guides**
- [ ] **Step 4: Update Python API reference**
- [ ] **Step 5: Commit**

```bash
git add examples/ docs/
git commit -m "docs: local middleware and telemetry across all SDKs with examples"
```

---

## Chunk 4: Sidecars + Handler Auto-Discovery

### Task 28: Python — Sidecar management (Item 7)

**Files:**
- Create: `sdk/python/src/protomcp/sidecar.py`
- Create: `sdk/python/tests/test_sidecar.py`
- Modify: `sdk/python/src/protomcp/runner.py`
- Modify: `sdk/python/src/protomcp/__init__.py`

- [ ] **Step 1: Write failing tests**

Test: registration, PID file path, health check success/failure (mocked), stop nonexistent sidecar.

- [ ] **Step 2: Implement sidecar.py**

`@sidecar(name, command, health_check, start_on, ...)` decorator.
`SidecarDef` dataclass with `pid_file_path` property.
`_start_sidecar`, `_stop_sidecar`, `_check_health` functions.
`start_sidecars(trigger)`, `stop_all_sidecars()`.
`atexit.register(stop_all_sidecars)`.

- [ ] **Step 3: Integrate into runner.py**

`start_sidecars("server_start")` in `run()`.
`start_sidecars("first_tool_call")` on first tool call (flag guard).

- [ ] **Step 4: Run tests, commit**

```bash
git add sdk/python/src/protomcp/sidecar.py sdk/python/tests/test_sidecar.py sdk/python/src/protomcp/runner.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): sidecar process management with health checks"
```

### Task 29: Go — Sidecar management (Item 7)

**Files:**
- Create: `sdk/go/protomcp/sidecar.go`
- Create: `sdk/go/protomcp/sidecar_test.go`
- Modify: `sdk/go/protomcp/runner.go`

- [ ] **Step 1: Write tests, implement**

`Sidecar(name, command, opts...)` with `HealthCheck(url)`, `StartOn(trigger)` options.
`StartSidecars(trigger)`, `StopAllSidecars()`.
Process management via `os/exec`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/go/protomcp/sidecar.go sdk/go/protomcp/sidecar_test.go sdk/go/protomcp/runner.go
git commit -m "feat(go-sdk): sidecar process management"
```

### Task 30: TypeScript — Sidecar management (Item 7)

**Files:**
- Create: `sdk/typescript/src/sidecar.ts`
- Create: `sdk/typescript/src/sidecar.test.ts`
- Modify: `sdk/typescript/src/runner.ts`, `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write tests, implement**

`sidecar({name, command, healthCheck, startOn})` function.
Process management via `child_process.spawn`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/sidecar.ts sdk/typescript/src/sidecar.test.ts sdk/typescript/src/runner.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): sidecar process management"
```

### Task 31: Rust — Sidecar management (Item 7)

**Files:**
- Create: `sdk/rust/src/sidecar.rs`
- Modify: `sdk/rust/src/runner.rs`, `sdk/rust/src/lib.rs`

- [ ] **Step 1: Write tests, implement**

`sidecar(name, command)` builder with `.health_check()`, `.start_on()`, `.register()`.
Process management via `tokio::process::Command`.

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/rust/src/sidecar.rs sdk/rust/src/runner.rs sdk/rust/src/lib.rs
git commit -m "feat(rust-sdk): sidecar process management"
```

---

### Task 32: Python — Handler auto-discovery (Item 8)

**Files:**
- Create: `sdk/python/src/protomcp/discovery.py`
- Create: `sdk/python/tests/test_discovery.py`
- Modify: `sdk/python/src/protomcp/runner.py`
- Modify: `sdk/python/src/protomcp/__init__.py`

- [ ] **Step 1: Write failing tests**

Test: configure, discover from directory, skip `_` prefixed files, rediscover clears and reimports.

- [ ] **Step 2: Implement discovery.py**

`configure(handlers_dir, hot_reload)` — stores config.
`discover_handlers()` — scans directory, imports modules via `importlib.util.spec_from_file_location`.
On hot reload: clears group registry, re-imports all.

- [ ] **Step 3: Integrate into runner.py**

`discover_handlers()` before main loop.
`discover_handlers()` in `_handle_reload` if `hot_reload` configured.

- [ ] **Step 4: Run tests, commit**

```bash
git add sdk/python/src/protomcp/discovery.py sdk/python/tests/test_discovery.py sdk/python/src/protomcp/runner.py sdk/python/src/protomcp/__init__.py
git commit -m "feat(python-sdk): handler auto-discovery with hot reload"
```

### Task 33: TypeScript — Handler auto-discovery (Item 8)

**Files:**
- Create: `sdk/typescript/src/discovery.ts`
- Create: `sdk/typescript/src/discovery.test.ts`
- Modify: `sdk/typescript/src/runner.ts`, `sdk/typescript/src/index.ts`

- [ ] **Step 1: Write tests, implement**

`configure({handlersDir, hotReload})` function.
`discoverHandlers()` — scans directory, `import()` each `.ts`/`.js` file.
On hot reload: clears group registry, re-imports (ESM cache invalidation via query string or `delete require.cache`).

- [ ] **Step 2: Run tests, commit**

```bash
git add sdk/typescript/src/discovery.ts sdk/typescript/src/discovery.test.ts sdk/typescript/src/runner.ts sdk/typescript/src/index.ts
git commit -m "feat(ts-sdk): handler auto-discovery with hot reload"
```

**Note:** Go and Rust are compiled languages — handler auto-discovery does not apply. Tools must be registered at compile time.

---

### Task 34: Final docs and examples

**Files:**
- Modify: all 4 language guide docs
- Modify: `docs/src/content/docs/reference/python-api.mdx`
- Modify: `examples/python/full_showcase.py`

- [ ] **Step 1: Add sidecar section to all 4 language guides**
- [ ] **Step 2: Add handler discovery section to Python and TypeScript guides**
- [ ] **Step 3: Update Python API reference with all remaining APIs**
- [ ] **Step 4: Add tool group to full_showcase.py**
- [ ] **Step 5: Run all tests across all SDKs**

```bash
cd /Users/msilverblatt/hotmcp && python -m pytest sdk/python/tests/ -v
cd /Users/msilverblatt/hotmcp/sdk/go && go test ./protomcp/ -v
cd /Users/msilverblatt/hotmcp/sdk/typescript && npx vitest run
cd /Users/msilverblatt/hotmcp/sdk/rust && cargo test
```

- [ ] **Step 6: Commit**

```bash
git add docs/ examples/
git commit -m "docs: complete all SDK documentation for harness-ml migration features"
```

---

## Summary

| Task | SDK | Item | What |
|------|-----|------|------|
| 1 | Python | 3 | `_type_to_schema` recursive function |
| 2 | Go | 3 | `ArrayArg`, `ObjectArg`, `UnionArg`, `LiteralArg` |
| 3 | TypeScript | 3 | Verify Zod handles complex types |
| 4 | Rust | 3 | `array`, `object`, `union`, `literal` ArgDef |
| 5 | Python | 1 | `@tool_group` + `@action` + union/separate schemas |
| 6 | Go | 1 | `ToolGroup()` + `Action()` builders |
| 7 | TypeScript | 1 | `toolGroup()` with Zod actions |
| 8 | Rust | 1 | `tool_group()` builder |
| 9 | All | 1+3 | Examples + docs for types and groups |
| 10 | Python | 6 | Declarative validation |
| 11 | Go | 6 | Declarative validation |
| 12 | TypeScript | 6 | Declarative validation |
| 13 | Rust | 6 | Declarative validation |
| 14 | Python | 4 | Server context |
| 15 | Go | 4 | Server context |
| 16 | TypeScript | 4 | Server context |
| 17 | Rust | 4 | Server context |
| 18 | All | 4+6 | Docs for validation + context |
| 19 | Python | 2 | Local middleware |
| 20 | Go | 2 | Local middleware |
| 21 | TypeScript | 2 | Local middleware |
| 22 | Rust | 2 | Local middleware |
| 23 | Python | 5 | Telemetry sinks |
| 24 | Go | 5 | Telemetry sinks |
| 25 | TypeScript | 5 | Telemetry sinks |
| 26 | Rust | 5 | Telemetry sinks |
| 27 | All | 2+5 | Examples + docs for middleware + telemetry |
| 28 | Python | 7 | Sidecar management |
| 29 | Go | 7 | Sidecar management |
| 30 | TypeScript | 7 | Sidecar management |
| 31 | Rust | 7 | Sidecar management |
| 32 | Python | 8 | Handler auto-discovery |
| 33 | TypeScript | 8 | Handler auto-discovery |
| 34 | All | 7+8 | Final docs + examples + full test suite |

### Parallelization guide

Within each chunk, SDK tasks are **independent** and can run in parallel:
- Tasks 1-4 (complex types): all 4 SDKs in parallel
- Tasks 5-8 (tool groups): all 4 SDKs in parallel (after types complete)
- Tasks 10-13 (validation): all 4 in parallel
- Tasks 14-17 (server context): all 4 in parallel
- Tasks 19-22 (local middleware): all 4 in parallel
- Tasks 23-26 (telemetry): all 4 in parallel
- Tasks 28-31 (sidecars): all 4 in parallel
- Tasks 32-33 (discovery): Python + TS in parallel

Doc/example tasks (9, 18, 27, 34) run after their chunk's implementation tasks complete.
