// Package profiles provides hot-reload support for JSON-based profiles.
//
// The Watcher monitors a profiles JSON file and reloads it when the file
// changes. Callers receive updates via a channel.
//
// Usage:
//
//	watcher, err := profiles.NewWatcher("profiles.json", 30*time.Second)
//	go watcher.Start(ctx)
//	for range watcher.Updates() {
//	    log.Println("profiles reloaded")
//	}
package profiles

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Watcher monitors a profiles JSON file for changes and reloads the
// registry when the file is modified.
type Watcher struct {
	path       string
	interval   time.Duration
	lastMod    time.Time
	lastSize   int64
	updates    chan struct{}
	closed     atomic.Bool     // guards send on closed channel
	mu         sync.Mutex
	reloadLock sync.Mutex      // prevents concurrent reloads
}

// NewWatcher creates a new Watcher for the given profiles JSON file.
// The interval controls how often the file is checked for changes.
// A zero or negative interval defaults to 30 seconds.
func NewWatcher(path string, interval time.Duration) (*Watcher, error) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat profiles file: %w", err)
	}

	return &Watcher{
		path:     path,
		interval: interval,
		lastMod:  info.ModTime(),
		lastSize: info.Size(),
		updates:  make(chan struct{}, 1),
		closed:   atomic.Bool{},
	}, nil
}

// Updates returns a channel that receives a value each time the profiles
// file is successfully reloaded. The channel is closed when the watcher
// stops. Non-blocking send avoids backpressure if the consumer is slow.
func (w *Watcher) Updates() <-chan struct{} {
	return w.updates
}

// Stopped returns true after Start() has returned and the watcher is
// fully shut down. After this point ReloadNow() is a no-op.
func (w *Watcher) Stopped() bool {
	return w.closed.Load()
}

// Start begins the watch loop. It blocks until ctx is cancelled.
// The first reload happens immediately. When Start returns, the
// Updates channel is closed and ReloadNow becomes a no-op.
func (w *Watcher) Start(ctx context.Context) {
	defer w.shutdown()

	// Initial load.
	if err := w.reload(); err != nil {
		// Log but continue — polling may succeed later.
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkAndReload()
		}
	}
}

// shutdown closes the updates channel and marks the watcher as stopped.
// It is safe to call multiple times.
func (w *Watcher) shutdown() {
	if w.closed.Swap(true) {
		return // already closed
	}
	close(w.updates)
}

// ReloadNow forces an immediate reload. Returns nil if the reload
// succeeds, an error if it fails. After the watcher has stopped
// (Start() returned), ReloadNow is a no-op and returns nil.
func (w *Watcher) ReloadNow() error {
	if w.closed.Load() {
		return nil // watcher stopped, silently skip
	}
	return w.reload()
}

func (w *Watcher) checkAndReload() {
	info, err := os.Stat(w.path)
	if err != nil {
		return
	}

	w.mu.Lock()
	changed := !info.ModTime().Equal(w.lastMod) || info.Size() != w.lastSize
	if changed {
		w.lastMod = info.ModTime()
		w.lastSize = info.Size()
	}
	w.mu.Unlock()

	if changed {
		w.reload()
	}
}

func (w *Watcher) reload() error {
	// If the watcher has already stopped, don't try to load or notify.
	if w.closed.Load() {
		return nil
	}

	w.reloadLock.Lock()
	defer w.reloadLock.Unlock()

	file, err := LoadProfilesFromJSONFile(w.path)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}

	if err := MergeJSONProfilesIntoRegistry(file); err != nil {
		return fmt.Errorf("merge profiles: %w", err)
	}

	// Non-blocking notify. Guard against closed channel: if stopped
	// between the closed.Load() check above and here, skip silently.
	if w.closed.Load() {
		return nil
	}
	select {
	case w.updates <- struct{}{}:
	default:
	}

	return nil
}
