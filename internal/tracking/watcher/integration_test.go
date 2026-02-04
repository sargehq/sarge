// +build integration

package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sargehq/sarge/internal/tracking/watcher"
)

// TestIntegration_ImmediateUpdates verifies that database changes trigger
// immediate updates through the watcher system.
func TestIntegration_ImmediateUpdates(t *testing.T) {
	// Create temp database file that simulates tracking.db
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("initial"), 0644)
	require.NoError(t, err, "failed to create test database")

	// Create and start watcher with standard debounce
	w, err := watcher.New(watcher.DefaultConfig(dbPath))
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe before starting
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Record start time
	startTime := time.Now()

	// Simulate database update (like when a task status changes)
	err = os.WriteFile(dbPath, []byte("updated"), 0644)
	require.NoError(t, err, "failed to update database")

	// Wait for notification
	select {
	case evt := <-sub:
		elapsed := time.Since(startTime)
		require.Equal(t, watcher.DBChanged, evt.Payload.Type, "expected DBChanged event")

		// Verify update was received quickly (within debounce + small buffer)
		// Default debounce is 100ms, allow 200ms total for processing
		require.Less(t, elapsed, 200*time.Millisecond,
			"update should be received within 200ms (got %v)", elapsed)

		t.Logf("Update received in %v", elapsed)
	case <-ctx.Done():
		require.Fail(t, "timeout waiting for database change notification")
	}
}

// TestIntegration_NoPollingRequired verifies that updates work without
// any polling mechanism when watcher is available.
func TestIntegration_NoPollingRequired(t *testing.T) {
	// Create temp database file
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "tracking.db")
	err := os.WriteFile(dbPath, []byte("initial"), 0644)
	require.NoError(t, err, "failed to create test database")

	// Create and start watcher
	w, err := watcher.New(watcher.DefaultConfig(dbPath))
	require.NoError(t, err, "failed to create watcher")
	defer func() { _ = w.Stop() }()

	// Subscribe before starting
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err, "failed to start watcher")

	// Perform multiple updates and verify each triggers a notification
	// without any polling delay
	updateCount := 5
	for i := 0; i < updateCount; i++ {
		// Write update
		data := []byte(string(rune('a' + i)))
		err := os.WriteFile(dbPath, data, 0644)
		require.NoError(t, err, "failed to write update %d", i)

		// Should receive notification quickly (no 2-second poll delay)
		select {
		case evt := <-sub:
			require.Equal(t, watcher.DBChanged, evt.Payload.Type,
				"expected DBChanged event for update %d", i)
		case <-time.After(500 * time.Millisecond):
			require.Fail(t, "update %d not received within 500ms (no polling should be needed)", i)
		}

		// Wait a bit to ensure debounce clears for next update
		time.Sleep(150 * time.Millisecond)
	}
}