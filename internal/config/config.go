package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Command            string
	File               string
	Transport          string
	HotReloadImmediate bool
	CallTimeout        time.Duration
	LogLevel           string
	SocketPath         string
	Runtime            string
	Host               string
	Port               int
	Auth               []string
	Strict             bool
	Format             string
	TestSubcommand     string // "list", "call", "scenario"
	TestToolName       string // tool name for "call"
	TestArgs           string // --args JSON string
	ShowTrace          bool   // --trace flag (default true)
}

func Parse(args []string) (*Config, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("usage: pmcp <dev|run|validate|test|playground> <file> [flags]")
	}

	cmd := args[0]
	if cmd == "--help" || cmd == "-h" || cmd == "help" {
		return &Config{Command: "help"}, nil
	}
	if cmd == "--version" || cmd == "-v" || cmd == "version" {
		return &Config{Command: "version"}, nil
	}
	if cmd != "dev" && cmd != "run" && cmd != "validate" && cmd != "test" && cmd != "playground" {
		return nil, fmt.Errorf("unknown command %q: must be 'dev', 'run', 'validate', 'test', or 'playground'", cmd)
	}

	cfg := &Config{
		Command:     cmd,
		Transport:   "stdio",
		CallTimeout: 5 * time.Minute,
		LogLevel:    "info",
		Host:        "localhost",
		Port:        8080,
		ShowTrace:   true,
	}

	i := 1
	if i >= len(args) || args[i] == "--" {
		return nil, fmt.Errorf("missing file argument")
	}
	if args[i][0] != '-' {
		cfg.File = args[i]
		i++
	} else {
		return nil, fmt.Errorf("missing file argument")
	}

	// Parse test subcommands
	if cfg.Command == "test" {
		if i >= len(args) {
			return nil, fmt.Errorf("missing test subcommand: must be 'list', 'call', or 'scenario'")
		}
		sub := args[i]
		if sub != "list" && sub != "call" && sub != "scenario" {
			return nil, fmt.Errorf("unknown test subcommand %q: must be 'list', 'call', or 'scenario'", sub)
		}
		cfg.TestSubcommand = sub
		i++

		if sub == "call" {
			if i >= len(args) || strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("missing tool name for 'test call'")
			}
			cfg.TestToolName = args[i]
			i++
		}
	}

	for i < len(args) {
		switch args[i] {
		case "--transport":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--transport requires a value")
			}
			cfg.Transport = args[i]
		case "--hot-reload":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--hot-reload requires a value")
			}
			if args[i] == "immediate" {
				cfg.HotReloadImmediate = true
			}
		case "--call-timeout":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--call-timeout requires a value")
			}
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid --call-timeout: %w", err)
			}
			cfg.CallTimeout = d
		case "--log-level":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--log-level requires a value")
			}
			cfg.LogLevel = args[i]
		case "--socket":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--socket requires a value")
			}
			cfg.SocketPath = args[i]
		case "--runtime":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--runtime requires a value")
			}
			cfg.Runtime = args[i]
		case "--host":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--host requires a value")
			}
			cfg.Host = args[i]
		case "--port":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--port requires a value")
			}
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid --port: %w", err)
			}
			cfg.Port = p
		case "--auth":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--auth requires a value")
			}
			parts := strings.SplitN(args[i], ":", 2)
			if len(parts) != 2 || parts[1] == "" {
				return nil, fmt.Errorf("invalid --auth format %q: expected scheme:ENV_VAR (e.g. token:MY_TOKEN)", args[i])
			}
			scheme := parts[0]
			if scheme != "token" && scheme != "apikey" {
				return nil, fmt.Errorf("unknown auth scheme %q: must be 'token' or 'apikey'", scheme)
			}
			cfg.Auth = append(cfg.Auth, args[i])
		case "--strict":
			cfg.Strict = true
		case "--args":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--args requires a value")
			}
			cfg.TestArgs = args[i]
		case "--trace":
			cfg.ShowTrace = true
		case "--trace=false":
			cfg.ShowTrace = false
		case "--trace=true":
			cfg.ShowTrace = true
		case "--format":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--format requires a value")
			}
			if args[i] != "text" && args[i] != "json" {
				return nil, fmt.Errorf("invalid --format %q: must be 'text' or 'json'", args[i])
			}
			cfg.Format = args[i]
		default:
			return nil, fmt.Errorf("unknown flag %q", args[i])
		}
		i++
	}

	if cfg.SocketPath == "" {
		dir := os.Getenv("XDG_RUNTIME_DIR")
		if dir == "" {
			dir = os.TempDir()
		}
		cfg.SocketPath = filepath.Join(dir, "pmcp", fmt.Sprintf("%d.sock", os.Getpid()))
	}

	return cfg, nil
}

// RuntimeCommand returns the command and args to run a tool file.
func RuntimeCommand(file string) (string, []string) {
	ext := filepath.Ext(file)
	switch ext {
	case ".py":
		if env := os.Getenv("PROTOMCP_PYTHON"); env != "" {
			return env, []string{file}
		}
		// Prefer uv if available and a pyproject.toml exists near the file
		if hasUV() && findPyprojectToml(file) != "" {
			return "uv", []string{"run", "python", file}
		}
		return "python3", []string{file}
	case ".ts":
		if env := os.Getenv("PROTOMCP_NODE"); env != "" {
			return env, []string{file}
		}
		return "npx", []string{"tsx", file}
	case ".js":
		cmd := "node"
		if env := os.Getenv("PROTOMCP_NODE"); env != "" {
			cmd = env
		}
		return cmd, []string{file}
	case ".go":
		return "go", []string{"run", file}
	case ".rs":
		dir := filepath.Dir(file)
		manifest := filepath.Join(dir, "Cargo.toml")
		if _, err := os.Stat(manifest); err != nil {
			manifest = filepath.Join(filepath.Dir(dir), "Cargo.toml")
		}
		return "cargo", []string{"run", "--manifest-path", manifest}
	default:
		return file, nil
	}
}

// hasUV checks if the uv command is available on PATH.
func hasUV() bool {
	_, err := exec.LookPath("uv")
	return err == nil
}

// findPyprojectToml walks up from the file's directory looking for pyproject.toml.
func findPyprojectToml(file string) string {
	dir, err := filepath.Abs(filepath.Dir(file))
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "pyproject.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
