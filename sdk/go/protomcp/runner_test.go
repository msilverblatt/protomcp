package protomcp

import (
	"encoding/json"
	"testing"
)

func TestBuildResultJSON(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"simple", "hello"},
		{"quotes", `He said "hello"`},
		{"backslash", `path\to\file`},
		{"newline", "line1\nline2"},
		{"tab", "col1\tcol2"},
		{"all_special", "He said \"hi\"\npath\\to\\file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildResultJSON(tc.input)
			var parsed []map[string]interface{}
			if err := json.Unmarshal([]byte(result), &parsed); err != nil {
				t.Fatalf("invalid JSON for input %q: %v\nGot: %s", tc.input, err, result)
			}
			if parsed[0]["text"] != tc.input {
				t.Fatalf("round-trip failed: got %q, want %q", parsed[0]["text"], tc.input)
			}
		})
	}
}
