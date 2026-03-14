package protomcp

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// SidecarDef holds a registered sidecar process definition.
type SidecarDef struct {
	Name            string
	Command         []string
	HealthCheck     string
	StartOn         string // "server_start" or "first_tool_call"
	HealthTimeout   time.Duration
	HealthInterval  time.Duration
	ShutdownTimeout time.Duration
}

type SidecarOption func(*SidecarDef)

var (
	sidecarRegistry   []SidecarDef
	sidecarMu         sync.Mutex
	runningProcesses  map[string]*exec.Cmd
	runningProcMu     sync.Mutex
)

func init() {
	runningProcesses = make(map[string]*exec.Cmd)
}

// Sidecar registers a sidecar process.
func Sidecar(name string, command []string, opts ...SidecarOption) {
	sidecarMu.Lock()
	defer sidecarMu.Unlock()
	sd := SidecarDef{
		Name:            name,
		Command:         command,
		StartOn:         "first_tool_call",
		HealthTimeout:   30 * time.Second,
		HealthInterval:  1 * time.Second,
		ShutdownTimeout: 3 * time.Second,
	}
	for _, opt := range opts {
		opt(&sd)
	}
	sidecarRegistry = append(sidecarRegistry, sd)
}

// HealthCheck sets the health check URL for a sidecar.
func HealthCheck(url string) SidecarOption {
	return func(sd *SidecarDef) { sd.HealthCheck = url }
}

// StartOn sets the trigger for starting the sidecar ("server_start" or "first_tool_call").
func StartOn(trigger string) SidecarOption {
	return func(sd *SidecarDef) { sd.StartOn = trigger }
}

// HealthTimeout sets the maximum time to wait for health check to pass.
func HealthTimeout(d time.Duration) SidecarOption {
	return func(sd *SidecarDef) { sd.HealthTimeout = d }
}

func pidFilePath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".protomcp", "sidecars", name+".pid")
}

func checkHealth(sc SidecarDef) bool {
	if sc.HealthCheck == "" {
		return true
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(sc.HealthCheck)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func startSidecar(sc SidecarDef) {
	runningProcMu.Lock()
	if cmd, exists := runningProcesses[sc.Name]; exists {
		if cmd.Process != nil && cmd.ProcessState == nil {
			// Still running
			runningProcMu.Unlock()
			if checkHealth(sc) {
				return
			}
			runningProcMu.Lock()
		}
	}
	runningProcMu.Unlock()

	if len(sc.Command) == 0 {
		return
	}

	pidDir := filepath.Dir(pidFilePath(sc.Name))
	os.MkdirAll(pidDir, 0755)

	cmd := exec.Command(sc.Command[0], sc.Command[1:]...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return
	}

	runningProcMu.Lock()
	runningProcesses[sc.Name] = cmd
	runningProcMu.Unlock()

	// Write PID file
	pidPath := pidFilePath(sc.Name)
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0644)

	// Wait for health check if configured
	if sc.HealthCheck != "" {
		deadline := time.Now().Add(sc.HealthTimeout)
		for time.Now().Before(deadline) {
			if checkHealth(sc) {
				return
			}
			time.Sleep(sc.HealthInterval)
		}
	}
}

func stopSidecar(sc SidecarDef) {
	runningProcMu.Lock()
	cmd, exists := runningProcesses[sc.Name]
	delete(runningProcesses, sc.Name)
	runningProcMu.Unlock()

	if exists && cmd.Process != nil {
		cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(sc.ShutdownTimeout):
			cmd.Process.Kill()
			<-done
		}
	}

	// Clean up PID file
	os.Remove(pidFilePath(sc.Name))
}

// StartSidecars starts all sidecars matching the given trigger.
func StartSidecars(trigger string) {
	sidecarMu.Lock()
	defs := make([]SidecarDef, len(sidecarRegistry))
	copy(defs, sidecarRegistry)
	sidecarMu.Unlock()

	for _, sc := range defs {
		if sc.StartOn == trigger {
			startSidecar(sc)
		}
	}
}

// StopAllSidecars stops all running sidecar processes.
func StopAllSidecars() {
	sidecarMu.Lock()
	defs := make([]SidecarDef, len(sidecarRegistry))
	copy(defs, sidecarRegistry)
	sidecarMu.Unlock()

	for _, sc := range defs {
		stopSidecar(sc)
	}
}

// GetRegisteredSidecars returns a copy of the sidecar registry.
func GetRegisteredSidecars() []SidecarDef {
	sidecarMu.Lock()
	defer sidecarMu.Unlock()
	result := make([]SidecarDef, len(sidecarRegistry))
	copy(result, sidecarRegistry)
	return result
}

// ClearSidecarRegistry removes all registered sidecars.
func ClearSidecarRegistry() {
	sidecarMu.Lock()
	defer sidecarMu.Unlock()
	sidecarRegistry = nil
}
