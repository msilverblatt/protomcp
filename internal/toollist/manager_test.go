package toollist_test

import (
	"testing"

	"github.com/protomcp/protomcp/internal/toollist"
)

func TestNewManager_AllToolsActive(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	active := m.GetActive()
	if len(active) != 3 {
		t.Fatalf("expected 3 active tools, got %d", len(active))
	}
}

func TestEnable_AddsToActive(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})
	m.Disable([]string{"b"})

	active := m.GetActive()
	if contains(active, "b") {
		t.Error("b should not be active after disable")
	}

	m.Enable([]string{"b"})
	active = m.GetActive()
	if !contains(active, "b") {
		t.Error("b should be active after enable")
	}
}

func TestDisable_RemovesFromActive(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.Disable([]string{"a", "c"})
	active := m.GetActive()
	if len(active) != 1 || active[0] != "b" {
		t.Errorf("expected [b], got %v", active)
	}
}

func TestSetAllowed_OnlyListedVisible(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c", "d"})

	m.SetAllowed([]string{"a", "c"})
	active := m.GetActive()
	if len(active) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(active), active)
	}
	if !contains(active, "a") || !contains(active, "c") {
		t.Errorf("expected [a,c], got %v", active)
	}
}

func TestSetBlocked_AllExceptListedVisible(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c", "d"})

	m.SetBlocked([]string{"b", "d"})
	active := m.GetActive()
	if len(active) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(active), active)
	}
	if !contains(active, "a") || !contains(active, "c") {
		t.Errorf("expected [a,c], got %v", active)
	}
}

func TestAllowlist_EnableAddsToAllowlist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetAllowed([]string{"a"})
	m.Enable([]string{"b"})
	active := m.GetActive()
	if !contains(active, "a") || !contains(active, "b") {
		t.Errorf("expected [a,b], got %v", active)
	}
	if contains(active, "c") {
		t.Error("c should not be in allowlist")
	}
}

func TestAllowlist_DisableRemovesFromAllowlist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetAllowed([]string{"a", "b"})
	m.Disable([]string{"a"})
	active := m.GetActive()
	if len(active) != 1 || active[0] != "b" {
		t.Errorf("expected [b], got %v", active)
	}
}

func TestBlocklist_EnableRemovesFromBlocklist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetBlocked([]string{"a", "b"})
	m.Enable([]string{"a"})
	active := m.GetActive()
	if !contains(active, "a") {
		t.Error("a should be active after unblocking")
	}
}

func TestBlocklist_DisableAddsToBlocklist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetBlocked([]string{"a"})
	m.Disable([]string{"b"})
	active := m.GetActive()
	if len(active) != 1 || active[0] != "c" {
		t.Errorf("expected [c], got %v", active)
	}
}

func TestSetAllowed_SwitchesFromBlocklist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetBlocked([]string{"a"})
	m.SetAllowed([]string{"b"})
	active := m.GetActive()
	if len(active) != 1 || active[0] != "b" {
		t.Errorf("expected [b], got %v", active)
	}
}

func TestSetBlocked_SwitchesFromAllowlist(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetAllowed([]string{"a"})
	m.SetBlocked([]string{"a"})
	active := m.GetActive()
	if !contains(active, "b") || !contains(active, "c") {
		t.Errorf("expected [b,c], got %v", active)
	}
}

func TestSetAllowed_EmptyResetsToOpen(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetAllowed([]string{"a"})
	m.SetAllowed([]string{})
	active := m.GetActive()
	if len(active) != 3 {
		t.Errorf("expected 3 (open mode), got %d: %v", len(active), active)
	}
}

func TestSetBlocked_EmptyResetsToOpen(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	m.SetBlocked([]string{"a"})
	m.SetBlocked([]string{})
	active := m.GetActive()
	if len(active) != 3 {
		t.Errorf("expected 3 (open mode), got %d: %v", len(active), active)
	}
}

func TestBatch_AtomicUpdate(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c", "d"})

	changed := m.Batch(nil, nil, []string{"a", "b"}, nil)
	if !changed {
		t.Error("expected changed=true")
	}
	active := m.GetActive()
	if len(active) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(active), active)
	}
}

func TestBatch_RejectsMixedAllowAndBlock(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b", "c"})

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for mixed allow+block")
		}
	}()
	m.Batch(nil, nil, []string{"a"}, []string{"b"})
}

func TestChanged_DetectsAddedTools(t *testing.T) {
	m := toollist.New()
	m.SetRegistered([]string{"a", "b"})

	before := m.GetActive()
	m.SetRegistered([]string{"a", "b", "c"})
	after := m.GetActive()

	if len(before) == len(after) {
		t.Error("expected different active sets after adding tool")
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
