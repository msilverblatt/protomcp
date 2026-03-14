package cancel

import (
	"context"
	"testing"
	"time"
)

func TestTracker_CancelContext(t *testing.T) {
	tr := NewTracker()
	ctx, _ := tr.TrackCallWithContext(context.Background(), "req-1")

	select {
	case <-ctx.Done():
		t.Fatal("context should not be cancelled yet")
	default:
	}

	tr.Cancel("req-1")

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled after Cancel()")
	}
}

func TestTracker_CancelUnknownRequestIsNoop(t *testing.T) {
	tr := NewTracker()
	tr.Cancel("nonexistent") // should not panic
}

func TestTracker_CompleteRemovesTracking(t *testing.T) {
	tr := NewTracker()
	_, reqID := tr.TrackCallWithContext(context.Background(), "req-2")
	tr.Complete(reqID)

	if tr.IsCancelled("req-2") {
		t.Fatal("completed call should not report tracked")
	}

	// Cancel after complete should be a no-op
	tr.Cancel("req-2")
}

func TestTracker_CompleteReleasesContext(t *testing.T) {
	tr := NewTracker()
	ctx, reqID := tr.TrackCallWithContext(context.Background(), "req-3")
	tr.Complete(reqID)

	// Context should be cancelled after Complete (releases resources)
	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be cancelled after Complete()")
	}
}
