package profiles

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWatcherInitialLoad verifies the watcher loads profiles on start.
func TestWatcherInitialLoad(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	origProfiles := mapsClone(canonicalTLSClients)
	defer func() {
		LockRegistry()
		canonicalTLSClients = origProfiles
		for k, v := range origProfiles {
			MappedTLSClients[k] = v
		}
		rebuildProfileIndexes()
		UnlockRegistry()
	}()

	// Clear registry under lock.
	LockRegistry()
	canonicalTLSClients = make(map[string]ClientProfile)
	for k := range MappedTLSClients {
		delete(MappedTLSClients, k)
	}
	rebuildProfileIndexes()
	UnlockRegistry()

	RLockRegistry()
	initialCount := len(canonicalTLSClients)
	RUnlockRegistry()
	require.Equal(t, 0, initialCount, "registry should be empty")

	watcher, err := NewWatcher(jsonPath, 100*time.Millisecond)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go watcher.Start(ctx)

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-ctx.Done():
		t.Fatal("timeout waiting for initial load")
	}

	RLockRegistry()
	count := len(canonicalTLSClients)
	RUnlockRegistry()
	require.Greater(t, count, 0, "registry should be populated")
	t.Logf("watcher loaded %d profiles", count)

	cancel()
	// Wait for watcher to stop (channel closed).
	waitWatcherStopped(t, watcher, 3*time.Second)
}

// TestWatcherReloadOnChange verifies file modification triggers reload.
func TestWatcherReloadOnChange(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	origProfiles := mapsClone(canonicalTLSClients)
	defer func() {
		LockRegistry()
		canonicalTLSClients = origProfiles
		for k, v := range origProfiles {
			MappedTLSClients[k] = v
		}
		rebuildProfileIndexes()
		UnlockRegistry()
	}()

	// Clear registry.
	LockRegistry()
	canonicalTLSClients = make(map[string]ClientProfile)
	for k := range MappedTLSClients {
		delete(MappedTLSClients, k)
	}
	rebuildProfileIndexes()
	UnlockRegistry()

	watcher, err := NewWatcher(jsonPath, 100*time.Millisecond)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	go watcher.Start(ctx)

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-ctx.Done():
		cancel()
		t.Fatal("timeout waiting for initial load")
	}

	// Touch the file to trigger reload.
	time.Sleep(200 * time.Millisecond)
	err = os.Chtimes(jsonPath, time.Now(), time.Now())
	require.NoError(t, err)

	// Wait for reload notification.
	select {
	case <-watcher.Updates():
		t.Log("reload notification received")
	case <-ctx.Done():
		cancel()
		t.Fatal("timeout waiting for reload")
	}

	// Stop the watcher before reading registry.
	cancel()
	waitWatcherStopped(t, watcher, 3*time.Second)

	RLockRegistry()
	countAfter := len(canonicalTLSClients)
	RUnlockRegistry()
	require.Greater(t, countAfter, 0, "no profiles loaded")
	t.Logf("after reload: %d profiles", countAfter)
}

// TestWatcherReloadNowAfterStop verifies ReloadNow is a no-op after Stop.
// This is the key fix — previously it would panic on closed channel.
func TestWatcherReloadNowAfterStop(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	watcher, err := NewWatcher(jsonPath, time.Second)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go watcher.Start(ctx)

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-ctx.Done():
		cancel()
		t.Fatal("timeout waiting for initial load")
	}

	// Stop the watcher.
	cancel()
	waitWatcherStopped(t, watcher, 3*time.Second)

	require.True(t, watcher.Stopped(), "watcher should report stopped")

	// ReloadNow after stop should NOT panic — it should return nil.
	err = watcher.ReloadNow()
	require.NoError(t, err, "ReloadNow after stop should not error")
	t.Log("ReloadNow after stop: OK (no panic)")
}

// TestWatcherDoubleStart verifies calling Start twice doesn't double-close channel.
func TestWatcherDoubleStart(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	watcher, err := NewWatcher(jsonPath, time.Hour)
	require.NoError(t, err)

	ctx1, cancel1 := context.WithCancel(context.Background())
	go watcher.Start(ctx1)

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-time.After(5 * time.Second):
		cancel1()
		t.Fatal("timeout")
	}

	// First stop.
	cancel1()
	waitWatcherStopped(t, watcher, 3*time.Second)
	require.True(t, watcher.Stopped())

	// Second Start (no-op for reloads, channel already closed).
	// This tests that shutdown() is idempotent.
	ctx2, cancel2 := context.WithCancel(context.Background())
	go watcher.Start(ctx2)
	cancel2()
	waitWatcherStopped(t, watcher, 1*time.Second)
	t.Log("double start: OK")
}

// TestWatcherContextCancellation verifies the watcher stops on cancel.
func TestWatcherContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	watcher, err := NewWatcher(jsonPath, 10*time.Second)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{})
	go func() {
		close(started)
		watcher.Start(ctx)
	}()

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout waiting for initial load")
	}

	// Cancel and verify watcher stops.
	cancel()

	waitWatcherStopped(t, watcher, 5*time.Second)
	t.Log("watcher stopped after context cancellation")
}

// TestWatcherFileNotFound verifies error on missing file.
func TestWatcherFileNotFound(t *testing.T) {
	_, err := NewWatcher("/nonexistent/path/profiles.json", time.Second)
	require.Error(t, err)
}

// TestWatcherConcurrentReloadAndResolve verifies no race between
// watcher reload (write lock) and profile resolution (read lock).
func TestWatcherConcurrentReloadAndResolve(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "profiles.json")

	err := ExportProfilesToJSON(jsonPath)
	require.NoError(t, err)

	watcher, err := NewWatcher(jsonPath, 50*time.Millisecond)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go watcher.Start(ctx)

	// Wait for initial load.
	select {
	case <-watcher.Updates():
	case <-ctx.Done():
		t.Fatal("timeout")
	}

	// Fire concurrent resolve + reload.
	done := make(chan struct{})
	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = ResolveClientProfileStrict("chrome_150")
				_, _ = ResolveClientProfileStrict("firefox_148")
				_ = RandomBrowserProfileKeys()
			}
			done <- struct{}{}
		}()
	}

	// Trigger reloads while resolves are happening.
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		os.Chtimes(jsonPath, time.Now(), time.Now())
	}

	// Wait for all resolvers.
	for i := 0; i < goroutines; i++ {
		<-done
	}

	t.Log("concurrent reload + resolve: OK")
}

// waitWatcherStopped blocks until the watcher's Updates channel is closed
// or the timeout expires.
func waitWatcherStopped(t *testing.T, w *Watcher, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case _, ok := <-w.Updates():
			if !ok {
				return // channel closed
			}
		case <-deadline:
			t.Fatal("timeout waiting for watcher to stop")
			return
		}
	}
}
