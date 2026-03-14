package cancel

import (
	"context"
	"sync"
)

type trackedCall struct {
	cancel context.CancelFunc
}

type Tracker struct {
	mu    sync.Mutex
	calls map[string]*trackedCall
}

func NewTracker() *Tracker {
	return &Tracker{calls: make(map[string]*trackedCall)}
}

// TrackCallWithContext creates a cancellable child context for the given request ID.
func (t *Tracker) TrackCallWithContext(parent context.Context, requestID string) (context.Context, string) {
	ctx, cancel := context.WithCancel(parent)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls[requestID] = &trackedCall{cancel: cancel}
	return ctx, requestID
}

// Cancel cancels the context for the given request ID.
func (t *Tracker) Cancel(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if call, exists := t.calls[requestID]; exists {
		call.cancel()
	}
}

// IsCancelled checks if a request is being tracked (for backward compat).
func (t *Tracker) IsCancelled(requestID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, exists := t.calls[requestID]
	return exists
}

// Complete removes tracking for the given request ID and releases resources.
func (t *Tracker) Complete(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if call, exists := t.calls[requestID]; exists {
		call.cancel()
		delete(t.calls, requestID)
	}
}
