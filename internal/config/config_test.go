package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/config"
)

func TestParseDefaults(t *testing.T) {
	cfg, err := config.Parse([]string{"dev", "server.py"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Command != "dev" {
		t.Errorf("Command = %q, want %q", cfg.Command, "dev")
	}
	if cfg.File != "server.py" {
		t.Errorf("File = %q, want %q", cfg.File, "server.py")
	}
	if cfg.Transport != "stdio" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "stdio")
	}
	if cfg.CallTimeout != 5*time.Minute {
		t.Errorf("CallTimeout = %v, want %v", cfg.CallTimeout, 5*time.Minute)
	}
	if cfg.HotReloadImmediate {
		t.Error("HotReloadImmediate should default to false")
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
}

func TestParseWithFlags(t *testing.T) {
	cfg, err := config.Parse([]string{
		"run", "tools.ts",
		"--transport", "grpc",
		"--hot-reload", "immediate",
		"--call-timeout", "30s",
		"--log-level", "debug",
		"--socket", "/tmp/test.sock",
		"--runtime", "python3.12",
	})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Command != "run" {
		t.Errorf("Command = %q, want %q", cfg.Command, "run")
	}
	if cfg.File != "tools.ts" {
		t.Errorf("File = %q, want %q", cfg.File, "tools.ts")
	}
	if cfg.Transport != "grpc" {
		t.Errorf("Transport = %q, want %q", cfg.Transport, "grpc")
	}
	if !cfg.HotReloadImmediate {
		t.Error("HotReloadImmediate should be true")
	}
	if cfg.CallTimeout != 30*time.Second {
		t.Errorf("CallTimeout = %v, want %v", cfg.CallTimeout, 30*time.Second)
	}
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q, want %q", cfg.SocketPath, "/tmp/test.sock")
	}
	if cfg.Runtime != "python3.12" {
		t.Errorf("Runtime = %q, want %q", cfg.Runtime, "python3.12")
	}
}

func TestParseMissingFile(t *testing.T) {
	_, err := config.Parse([]string{"dev"})
	if err == nil {
		t.Error("expected error for missing file argument")
	}
}

func TestParseInvalidCommand(t *testing.T) {
	_, err := config.Parse([]string{"foo", "server.py"})
	if err == nil {
		t.Error("expected error for invalid command")
	}
}

func TestParseValidateCommand(t *testing.T) {
	cfg, err := config.Parse([]string{"validate", "tools.py"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Command != "validate" {
		t.Errorf("Command = %q, want %q", cfg.Command, "validate")
	}
	if cfg.File != "tools.py" {
		t.Errorf("File = %q, want %q", cfg.File, "tools.py")
	}
}

func TestParseValidateStrict(t *testing.T) {
	cfg, err := config.Parse([]string{"validate", "tools.py", "--strict", "--format", "json"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !cfg.Strict {
		t.Error("Strict should be true")
	}
	if cfg.Format != "json" {
		t.Errorf("Format = %q, want %q", cfg.Format, "json")
	}
}

func TestParseAuthFlags(t *testing.T) {
	cfg, err := config.Parse([]string{"run", "tools.py", "--auth", "token:MY_TOKEN", "--auth", "apikey:MY_KEY"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(cfg.Auth) != 2 {
		t.Fatalf("Auth len = %d, want 2", len(cfg.Auth))
	}
	if cfg.Auth[0] != "token:MY_TOKEN" {
		t.Errorf("Auth[0] = %q, want %q", cfg.Auth[0], "token:MY_TOKEN")
	}
	if cfg.Auth[1] != "apikey:MY_KEY" {
		t.Errorf("Auth[1] = %q, want %q", cfg.Auth[1], "apikey:MY_KEY")
	}
}

func TestParseAuthInvalidFormat(t *testing.T) {
	_, err := config.Parse([]string{"run", "tools.py", "--auth", "badformat"})
	if err == nil {
		t.Error("expected error for invalid auth format")
	}
}

func TestParseAuthUnknownScheme(t *testing.T) {
	_, err := config.Parse([]string{"run", "tools.py", "--auth", "jwt:MY_TOKEN"})
	if err == nil {
		t.Error("expected error for unknown auth scheme")
	}
}

func TestParseFormatInvalid(t *testing.T) {
	_, err := config.Parse([]string{"validate", "tools.py", "--format", "xml"})
	if err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestRuntimeCommand(t *testing.T) {
	tests := []struct {
		file     string
		wantCmd  string
		wantArgs []string
	}{
		{"server.py", "python3", []string{"server.py"}},
		{"server.ts", "npx", []string{"tsx", "server.ts"}},
		{"server.js", "node", []string{"server.js"}},
		{"server.go", "go", []string{"run", "server.go"}},
		{"server.rs", "cargo", []string{"run", "--manifest-path", "Cargo.toml"}},
		{"server", "server", nil},
	}
	for _, tt := range tests {
		cmd, args := config.RuntimeCommand(tt.file)
		if cmd != tt.wantCmd {
			t.Errorf("RuntimeCommand(%q) cmd = %q, want %q", tt.file, cmd, tt.wantCmd)
		}
		if !reflect.DeepEqual(args, tt.wantArgs) {
			t.Errorf("RuntimeCommand(%q) args = %v, want %v", tt.file, args, tt.wantArgs)
		}
	}
}
