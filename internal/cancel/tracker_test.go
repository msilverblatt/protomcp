package cancel

import "testing"

func TestTracker_SetAndCheck(t *testing.T) {
	tracker := NewTracker()
	tracker.TrackCall("req-1")
	if tracker.IsCancelled("req-1") {
		t.Fatal("should not be cancelled yet")
	}
	tracker.Cancel("req-1")
	if !tracker.IsCancelled("req-1") {
		t.Fatal("should be cancelled after Cancel()")
	}
}

func TestTracker_CancelUnknownRequestIsNoop(t *testing.T) {
	tracker := NewTracker()
	tracker.Cancel("nonexistent")
}

func TestTracker_CompleteRemovesTracking(t *testing.T) {
	tracker := NewTracker()
	tracker.TrackCall("req-1")
	tracker.Complete("req-1")
	if tracker.IsCancelled("req-1") {
		t.Fatal("completed call should not report cancelled")
	}
}
