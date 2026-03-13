package tasks

import (
	"fmt"
	"sync"
)

type TaskState struct {
	State   string
	Message string
}

type Manager struct {
	mu    sync.RWMutex
	tasks map[string]*TaskState
}

func NewManager() *Manager {
	return &Manager{tasks: make(map[string]*TaskState)}
}

func (m *Manager) Register(taskID, requestID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[taskID] = &TaskState{State: "running"}
}

func (m *Manager) GetStatus(taskID string) (*TaskState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("unknown task: %s", taskID)
	}
	return state, nil
}

func (m *Manager) UpdateStatus(taskID, state, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ts, ok := m.tasks[taskID]
	if !ok {
		return fmt.Errorf("unknown task: %s", taskID)
	}
	ts.State = state
	ts.Message = message
	return nil
}

func (m *Manager) FailAll(reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ts := range m.tasks {
		if ts.State == "running" {
			ts.State = "failed"
			ts.Message = reason
		}
	}
}
