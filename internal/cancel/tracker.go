package cancel

import "sync"

type Tracker struct {
	mu        sync.RWMutex
	cancelled map[string]bool
}

func NewTracker() *Tracker {
	return &Tracker{cancelled: make(map[string]bool)}
}

func (t *Tracker) TrackCall(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.cancelled[requestID] = false
}

func (t *Tracker) Cancel(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.cancelled[requestID]; exists {
		t.cancelled[requestID] = true
	}
}

func (t *Tracker) IsCancelled(requestID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cancelled[requestID]
}

func (t *Tracker) Complete(requestID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.cancelled, requestID)
}
