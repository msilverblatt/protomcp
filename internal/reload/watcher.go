package reload

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a file or directory for changes and calls onChange
// when a relevant file is modified, with debouncing.
type Watcher struct {
	path       string
	extensions []string
	onChange   func()
	watcher    *fsnotify.Watcher
	mu         sync.Mutex
}

// NewWatcher creates a new file watcher. If extensions is nil or empty,
// all file changes are reported.
func NewWatcher(path string, extensions []string, onChange func()) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := fsw.Add(path); err != nil {
		fsw.Close()
		return nil, err
	}

	return &Watcher{
		path:       path,
		extensions: extensions,
		onChange:   onChange,
		watcher:    fsw,
	}, nil
}

// Start blocks and watches for file changes until the context is cancelled.
// It debounces rapid changes with a 100ms window.
func (w *Watcher) Start(ctx context.Context) error {
	var debounceTimer *time.Timer
	var debounceMu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.watcher.Events:
			if !ok {
				return nil
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			if !w.matchesExtension(event.Name) {
				continue
			}

			debounceMu.Lock()
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(100*time.Millisecond, func() {
				w.onChange()
			})
			debounceMu.Unlock()

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return nil
			}
			_ = err // Log errors could be added here
		}
	}
}

// Stop closes the underlying fsnotify watcher.
func (w *Watcher) Stop() error {
	return w.watcher.Close()
}

func (w *Watcher) matchesExtension(name string) bool {
	if len(w.extensions) == 0 {
		return true
	}
	ext := filepath.Ext(name)
	for _, e := range w.extensions {
		if ext == e {
			return true
		}
	}
	return false
}
