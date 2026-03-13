package tasks

import "testing"

func TestTaskManager_CreateAndGet(t *testing.T) {
	mgr := NewManager()
	mgr.Register("task-1", "req-1")
	state, err := mgr.GetStatus("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "running" {
		t.Fatalf("expected running, got %s", state.State)
	}
}

func TestTaskManager_FailOnCrash(t *testing.T) {
	mgr := NewManager()
	mgr.Register("task-1", "req-1")
	mgr.FailAll("tool process crashed")
	state, err := mgr.GetStatus("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if state.State != "failed" {
		t.Fatalf("expected failed, got %s", state.State)
	}
}

func TestTaskManager_GetUnknownTask(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.GetStatus("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown task")
	}
}
