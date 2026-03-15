package toollist

import (
	"fmt"
	"sort"
	"sync"
)

type mode int

const (
	modeOpen mode = iota
	modeAllowlist
	modeBlocklist
)

// Manager manages which tools are visible based on open/allowlist/blocklist modes.
type Manager struct {
	mu         sync.RWMutex
	registered []string
	mode       mode
	allowed    map[string]bool
	blocked    map[string]bool
	disabled   map[string]bool
}

// New creates a Manager in open mode.
func New() *Manager {
	return &Manager{
		mode:     modeOpen,
		allowed:  make(map[string]bool),
		blocked:  make(map[string]bool),
		disabled: make(map[string]bool),
	}
}

// SetRegistered sets the full list of known tool names.
func (m *Manager) SetRegistered(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registered = make([]string, len(names))
	copy(m.registered, names)
}

// GetActive returns the currently active tools based on mode.
func (m *Manager) GetActive() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getActiveLocked()
}

func (m *Manager) getActiveLocked() []string {
	var active []string
	switch m.mode {
	case modeOpen:
		for _, name := range m.registered {
			if !m.disabled[name] {
				active = append(active, name)
			}
		}
	case modeAllowlist:
		for _, name := range m.registered {
			if m.allowed[name] {
				active = append(active, name)
			}
		}
	case modeBlocklist:
		for _, name := range m.registered {
			if !m.blocked[name] {
				active = append(active, name)
			}
		}
	}
	sort.Strings(active)
	return active
}

// Enable adds tools based on current mode.
func (m *Manager) Enable(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.mode {
	case modeOpen:
		for _, n := range names {
			delete(m.disabled, n)
		}
	case modeAllowlist:
		for _, n := range names {
			m.allowed[n] = true
		}
	case modeBlocklist:
		for _, n := range names {
			delete(m.blocked, n)
		}
	}
}

// Disable removes tools based on current mode.
func (m *Manager) Disable(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch m.mode {
	case modeOpen:
		for _, n := range names {
			m.disabled[n] = true
		}
	case modeAllowlist:
		for _, n := range names {
			delete(m.allowed, n)
		}
	case modeBlocklist:
		for _, n := range names {
			m.blocked[n] = true
		}
	}
}

// SetAllowed switches to allowlist mode. Empty list resets to open mode.
func (m *Manager) SetAllowed(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(names) == 0 {
		m.mode = modeOpen
		m.allowed = make(map[string]bool)
		m.blocked = make(map[string]bool)
		m.disabled = make(map[string]bool)
		return
	}
	m.mode = modeAllowlist
	m.allowed = make(map[string]bool)
	for _, n := range names {
		m.allowed[n] = true
	}
	m.blocked = make(map[string]bool)
	m.disabled = make(map[string]bool)
}

// SetBlocked switches to blocklist mode. Empty list resets to open mode.
func (m *Manager) SetBlocked(names []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(names) == 0 {
		m.mode = modeOpen
		m.allowed = make(map[string]bool)
		m.blocked = make(map[string]bool)
		m.disabled = make(map[string]bool)
		return
	}
	m.mode = modeBlocklist
	m.blocked = make(map[string]bool)
	for _, n := range names {
		m.blocked[n] = true
	}
	m.allowed = make(map[string]bool)
	m.disabled = make(map[string]bool)
}

// Batch performs an atomic multi-operation update. Returns an error if both allow and block are non-nil/non-empty.
// Returns true if the active set changed.
func (m *Manager) Batch(enable, disable, allow, block []string) (bool, error) {
	if len(allow) > 0 && len(block) > 0 {
		return false, fmt.Errorf("batch: cannot specify both allow and block")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	before := m.getActiveLocked()

	// Mode-setting operations first
	if len(allow) > 0 {
		m.mode = modeAllowlist
		m.allowed = make(map[string]bool)
		for _, n := range allow {
			m.allowed[n] = true
		}
		m.blocked = make(map[string]bool)
		m.disabled = make(map[string]bool)
	} else if len(block) > 0 {
		m.mode = modeBlocklist
		m.blocked = make(map[string]bool)
		for _, n := range block {
			m.blocked[n] = true
		}
		m.allowed = make(map[string]bool)
		m.disabled = make(map[string]bool)
	}

	// Delta operations after mode-setting
	if len(enable) > 0 {
		switch m.mode {
		case modeOpen:
			for _, n := range enable {
				delete(m.disabled, n)
			}
		case modeAllowlist:
			for _, n := range enable {
				m.allowed[n] = true
			}
		case modeBlocklist:
			for _, n := range enable {
				delete(m.blocked, n)
			}
		}
	}
	if len(disable) > 0 {
		switch m.mode {
		case modeOpen:
			for _, n := range disable {
				m.disabled[n] = true
			}
		case modeAllowlist:
			for _, n := range disable {
				delete(m.allowed, n)
			}
		case modeBlocklist:
			for _, n := range disable {
				m.blocked[n] = true
			}
		}
	}

	after := m.getActiveLocked()
	return !slicesEqual(before, after), nil
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
