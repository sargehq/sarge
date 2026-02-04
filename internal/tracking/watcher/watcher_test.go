package watcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sargehq/sarge/internal/beads/pubsub"
	"github.com/sargehq/sarge/internal/tracking/watcher"
)

func TestWatcher_DebounceMultipleWrites(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher with debounce longer than total write duration
	// to ensure all writes coalesce into a single notification.
	// Write loop: 10 writes * 5ms = 50ms, so 150ms debounce ensures coalescing.
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 150 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Rapid writes should coalesce into few notifications (not 10)
	for i := 0; i < 10; i++ {
		err := os.WriteFile(dbPath, []byte(fmt.Sprintf("test%d", i)), 0644)
		require.NoError(t, err, "failed to write file")
		time.Sleep(5 * time.Millisecond)
	}

	// Count notifications over a reasonable window.
	// With proper debouncing, 10 rapid writes should produce very few notifications
	// (typically 1-2, not 10). On CI, late file system events may cause a second
	// notification after the debounce fires, which is still correct behavior.
	var notificationCount int
	deadline := time.After(500 * time.Millisecond)
countLoop:
	for {
		select {
		case evt := <-sub:
			require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
			notificationCount++
		case <-deadline:
			break countLoop
		}
	}

	require.GreaterOrEqual(t, notificationCount, 1, "expected at least one notification")
	require.LessOrEqual(t, notificationCount, 3, "expected debouncing to coalesce most writes (got %d notifications for 10 writes)", notificationCount)
}

func TestWatcher_IgnoresIrrelevantFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	otherPath := filepath.Join(dir, "other.txt")
	err := os.WriteFile(dbPath, []byte("db"), 0644)
	require.NoError(t, err, "failed to create db file")
	// Pre-create the other file so writes to it are just Write events
	err = os.WriteFile(otherPath, []byte("initial"), 0644)
	require.NoError(t, err, "failed to create other file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to unrelated file (not Create, since it already exists)
	err = os.WriteFile(otherPath, []byte("other content"), 0644)
	require.NoError(t, err, "failed to write other file")

	select {
	case <-sub:
		require.Fail(t, "should not notify for unrelated files")
	case <-time.After(100 * time.Millisecond):
		// Expected - no notification for unrelated file
	}
}

func TestWatcher_Stop(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Stop should not hang or panic
	done := make(chan struct{})
	go func() {
		err := w.Stop()
		require.NoError(t, err, "Stop returned error")
		close(done)
	}()

	select {
	case <-done:
		// Expected - stop completed successfully
	case <-time.After(1 * time.Second):
		require.Fail(t, "Stop() timed out - possible deadlock")
	}
}

func TestWatcher_WatchesWALFile(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	walPath := filepath.Join(dir, "tracking.db-wal")

	// Create db file (watcher needs the directory to exist with db file)
	err := os.WriteFile(dbPath, []byte("db"), 0644)
	require.NoError(t, err, "failed to create db file")

	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Write to WAL file should trigger notification
	err = os.WriteFile(walPath, []byte("wal data"), 0644)
	require.NoError(t, err, "failed to write WAL file")

	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event for WAL write")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for WAL file write")
	}
}

func TestDefaultConfig(t *testing.T) {
	dbPath := "/test/tracking.db"
	cfg := watcher.DefaultConfig(dbPath)

	require.Equal(t, dbPath, cfg.DBPath)
	require.Equal(t, 100*time.Millisecond, cfg.DebounceDur)
}

func TestWatcher_BrokerAccessor(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Start watcher
	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Broker() should return non-nil broker after New()
	broker := w.Broker()
	require.NotNil(t, broker, "Broker() should return non-nil broker after New()")

	// Test that broker subscribers count is working
	ctx := context.Background()
	sub1 := broker.Subscribe(ctx)
	require.NotNil(t, sub1, "Subscribe should return non-nil channel")

	// Count should be non-zero after subscribing
	count := broker.SubscriberCount()
	require.Greater(t, count, 0, "SubscriberCount should be non-zero after subscribing")
}

func TestWatcher_MultipleWrites(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("test"), 0644)
	require.NoError(t, err, "failed to create test file")

	// Create watcher with shorter debounce to test separate events
	w, err := watcher.New(watcher.Config{
		DBPath:      dbPath,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe to broker before starting
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// First write
	err = os.WriteFile(dbPath, []byte("test1"), 0644)
	require.NoError(t, err, "failed to write file")

	// Should receive first notification
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
		require.Equal(t, pubsub.UpdatedEvent, evt.Type, "expected UpdatedEvent type")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected first notification but got timeout")
	}

	// Wait for debounce to clear
	time.Sleep(100 * time.Millisecond)

	// Second write
	err = os.WriteFile(dbPath, []byte("test2"), 0644)
	require.NoError(t, err, "failed to write file")

	// Should receive second notification
	select {
	case evt := <-sub:
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected second notification but got timeout")
	}
}
