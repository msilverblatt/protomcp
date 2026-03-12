package config_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/protomcp/protomcp/internal/config"
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
		{"server.rs", "cargo", []string{"run", "server.rs"}},
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
