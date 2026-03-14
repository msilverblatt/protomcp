package protomcp

import (
	"os"
	"testing"
	"time"
)

func TestSidecarRegistration(t *testing.T) {
	ClearSidecarRegistry()
	defer ClearSidecarRegistry()

	Sidecar("redis", []string{"redis-server"}, StartOn("server_start"), HealthTimeout(5*time.Second))
	Sidecar("worker", []string{"python", "worker.py"})

	sidecars := GetRegisteredSidecars()
	if len(sidecars) != 2 {
		t.Fatalf("expected 2 sidecars, got %d", len(sidecars))
	}
	if sidecars[0].Name != "redis" {
		t.Errorf("expected name 'redis', got '%s'", sidecars[0].Name)
	}
	if sidecars[0].StartOn != "server_start" {
		t.Errorf("expected start_on 'server_start', got '%s'", sidecars[0].StartOn)
	}
	if sidecars[0].HealthTimeout != 5*time.Second {
		t.Errorf("expected health timeout 5s, got %v", sidecars[0].HealthTimeout)
	}
	if sidecars[1].Name != "worker" {
		t.Errorf("expected name 'worker', got '%s'", sidecars[1].Name)
	}
	if sidecars[1].StartOn != "first_tool_call" {
		t.Errorf("expected default start_on 'first_tool_call', got '%s'", sidecars[1].StartOn)
	}
}

func TestSidecarHealthCheckOption(t *testing.T) {
	ClearSidecarRegistry()
	defer ClearSidecarRegistry()

	Sidecar("api", []string{"node", "server.js"}, HealthCheck("http://localhost:3000/health"))

	sidecars := GetRegisteredSidecars()
	if sidecars[0].HealthCheck != "http://localhost:3000/health" {
		t.Errorf("expected health check URL, got '%s'", sidecars[0].HealthCheck)
	}
}

func TestSidecarStartAndStop(t *testing.T) {
	ClearSidecarRegistry()
	defer ClearSidecarRegistry()

	// Use a command that runs briefly then exits (sleep 10)
	Sidecar("sleeper", []string{"sleep", "10"}, StartOn("test_trigger"))

	StartSidecars("test_trigger")

	// Verify process is tracked
	runningProcMu.Lock()
	cmd, exists := runningProcesses["sleeper"]
	runningProcMu.Unlock()

	if !exists {
		t.Fatal("expected sleeper to be running")
	}
	if cmd.Process == nil {
		t.Fatal("expected process to be set")
	}

	// Verify PID file was created
	pidPath := pidFilePath("sleeper")
	if _, err := os.Stat(pidPath); os.IsNotExist(err) {
		t.Error("expected PID file to exist")
	}

	StopAllSidecars()

	// Verify PID file was cleaned up
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed after stop")
	}

	// Verify process is no longer tracked
	runningProcMu.Lock()
	_, exists = runningProcesses["sleeper"]
	runningProcMu.Unlock()
	if exists {
		t.Error("expected sleeper to be removed from running processes")
	}
}

func TestSidecarStartOnlyMatchingTrigger(t *testing.T) {
	ClearSidecarRegistry()
	defer ClearSidecarRegistry()

	Sidecar("s1", []string{"sleep", "10"}, StartOn("server_start"))
	Sidecar("s2", []string{"sleep", "10"}, StartOn("first_tool_call"))

	StartSidecars("server_start")
	defer StopAllSidecars()

	runningProcMu.Lock()
	_, s1Running := runningProcesses["s1"]
	_, s2Running := runningProcesses["s2"]
	runningProcMu.Unlock()

	if !s1Running {
		t.Error("expected s1 to be running")
	}
	if s2Running {
		t.Error("expected s2 to NOT be running yet")
	}
}

func TestClearSidecarRegistry(t *testing.T) {
	ClearSidecarRegistry()
	defer ClearSidecarRegistry()

	Sidecar("x", []string{"echo", "hi"})
	ClearSidecarRegistry()

	sidecars := GetRegisteredSidecars()
	if len(sidecars) != 0 {
		t.Errorf("expected 0 sidecars after clear, got %d", len(sidecars))
	}
}

func TestPidFilePath(t *testing.T) {
	path := pidFilePath("myservice")
	home, _ := os.UserHomeDir()
	expected := home + "/.protomcp/sidecars/myservice.pid"
	if path != expected {
		t.Errorf("expected '%s', got '%s'", expected, path)
	}
}
