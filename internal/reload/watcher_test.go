package reload_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/msilverblatt/protomcp/internal/reload"
)

func TestWatcherFileChange(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.py")

	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32

	w, err := reload.NewWatcher(dir, nil, func() {
		called.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Start(ctx)

	// Give the watcher time to set up
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(testFile, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce (100ms) + some buffer
	time.Sleep(300 * time.Millisecond)

	if called.Load() < 1 {
		t.Errorf("expected onChange to be called at least once, got %d", called.Load())
	}
}

func TestWatcherDebounce(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.py")

	if err := os.WriteFile(testFile, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32

	w, err := reload.NewWatcher(dir, nil, func() {
		called.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go w.Start(ctx)

	// Give the watcher time to set up
	time.Sleep(50 * time.Millisecond)

	// Make multiple rapid changes (all within the 100ms debounce window)
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(testFile, []byte("change"), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce to fire
	time.Sleep(300 * time.Millisecond)

	count := called.Load()
	if count != 1 {
		t.Errorf("expected exactly 1 debounced onChange call, got %d", count)
	}
}
