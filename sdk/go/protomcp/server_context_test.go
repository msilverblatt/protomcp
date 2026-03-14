package protomcp

import "testing"

func TestServerContextResolve(t *testing.T) {
	ClearContextRegistry()
	defer ClearContextRegistry()

	ServerContext("user_id", func(args map[string]interface{}) interface{} {
		return "user-123"
	})

	args := map[string]interface{}{"foo": "bar"}
	resolved := ResolveContexts(args)

	if resolved["user_id"] != "user-123" {
		t.Errorf("expected 'user-123', got '%v'", resolved["user_id"])
	}
}

func TestServerContextResolverCanReadArgs(t *testing.T) {
	ClearContextRegistry()
	defer ClearContextRegistry()

	ServerContext("greeting", func(args map[string]interface{}) interface{} {
		name, _ := args["name"].(string)
		return "Hello, " + name
	})

	args := map[string]interface{}{"name": "Alice"}
	resolved := ResolveContexts(args)

	if resolved["greeting"] != "Hello, Alice" {
		t.Errorf("expected 'Hello, Alice', got '%v'", resolved["greeting"])
	}
}

func TestServerContextResolverCanDeleteFromArgs(t *testing.T) {
	ClearContextRegistry()
	defer ClearContextRegistry()

	ServerContext("token", func(args map[string]interface{}) interface{} {
		val := args["auth_token"]
		delete(args, "auth_token")
		return val
	})

	args := map[string]interface{}{"auth_token": "secret", "data": "value"}
	resolved := ResolveContexts(args)

	if resolved["token"] != "secret" {
		t.Errorf("expected 'secret', got '%v'", resolved["token"])
	}
	if _, exists := args["auth_token"]; exists {
		t.Error("expected auth_token to be deleted from args")
	}
}

func TestServerContextExpose(t *testing.T) {
	ClearContextRegistry()
	defer ClearContextRegistry()

	ServerContext("visible", func(args map[string]interface{}) interface{} {
		return "v"
	})
	ServerContext("hidden", func(args map[string]interface{}) interface{} {
		return "h"
	}, Expose(false))

	hidden := GetHiddenContextParams()
	if hidden["visible"] {
		t.Error("visible should not be hidden")
	}
	if !hidden["hidden"] {
		t.Error("hidden should be hidden")
	}
}

func TestClearContextRegistry(t *testing.T) {
	ClearContextRegistry()
	defer ClearContextRegistry()

	ServerContext("x", func(args map[string]interface{}) interface{} { return nil })
	ClearContextRegistry()

	resolved := ResolveContexts(map[string]interface{}{})
	if len(resolved) != 0 {
		t.Errorf("expected empty resolved, got %d entries", len(resolved))
	}
}
