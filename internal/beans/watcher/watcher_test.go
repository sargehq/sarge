package watcher_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sargehq/sarge/internal/beans/pubsub"
	"github.com/sargehq/sarge/internal/beans/watcher"
)

func TestWatcher_DebounceMultipleWrites(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "main-abc--test-bean.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 150 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	for i := 0; i < 10; i++ {
		err := os.WriteFile(mdPath, []byte(fmt.Sprintf("test%d", i)), 0644)
		require.NoError(t, err)
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case evt := <-sub:
		require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
	case <-time.After(400 * time.Millisecond):
		require.Fail(t, "expected notification but got timeout")
	}

	select {
	case <-sub:
		require.Fail(t, "unexpected second notification")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWatcher_IgnoresIrrelevantFiles(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "main-abc--test.md")
	otherPath := filepath.Join(dir, "other.txt")
	err := os.WriteFile(mdPath, []byte("bean"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(otherPath, []byte("initial"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	err = os.WriteFile(otherPath, []byte("other content"), 0644)
	require.NoError(t, err)

	select {
	case <-sub:
		require.Fail(t, "should not notify for non-.md files")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestWatcher_Stop(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "test.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	err = w.Start()
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		err := w.Stop()
		require.NoError(t, err)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		require.Fail(t, "Stop() timed out")
	}
}

func TestWatcher_ReactsToNewMDFiles(t *testing.T) {
	dir := t.TempDir()

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	// Create a new .md file — should trigger notification
	err = os.WriteFile(filepath.Join(dir, "main-new--new-bean.md"), []byte("new bean"), 0644)
	require.NoError(t, err)

	select {
	case evt := <-sub:
		require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for new .md file")
	}
}

func TestWatcher_ReactsToDeletedMDFiles(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "main-del--delete-me.md")
	err := os.WriteFile(mdPath, []byte("to delete"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	err = os.Remove(mdPath)
	require.NoError(t, err)

	select {
	case evt := <-sub:
		require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected notification for deleted .md file")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := watcher.DefaultConfig("/test/.beans")
	require.Equal(t, "/test/.beans", cfg.BeansDir)
	require.Equal(t, 100*time.Millisecond, cfg.DebounceDur)
}

func TestWatcher_BrokerAccessor(t *testing.T) {
	dir := t.TempDir()

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	broker := w.Broker()
	require.NotNil(t, broker)
}

func TestWatcher_BrokerAccessorBeforeStart(t *testing.T) {
	dir := t.TempDir()

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	broker := w.Broker()
	require.NotNil(t, broker)
}

func TestWatcher_PublishesBeansChangedEvent(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "main-abc--test.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	err = os.WriteFile(mdPath, []byte("modified content"), 0644)
	require.NoError(t, err)

	select {
	case evt := <-sub:
		require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
		require.Equal(t, pubsub.UpdatedEvent, evt.Type)
		require.Nil(t, evt.Payload.Error)
	case <-time.After(200 * time.Millisecond):
		require.Fail(t, "expected BeansChanged event but got timeout")
	}
}

func TestWatcher_DebounceWithPubsub(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "main-abc--test.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 200 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		err := os.WriteFile(mdPath, []byte(fmt.Sprintf("content%d", i)), 0644)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case evt := <-sub:
		require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
	case <-time.After(500 * time.Millisecond):
		require.Fail(t, "expected coalesced event but got timeout")
	}

	select {
	case <-sub:
		require.Fail(t, "received unexpected second event")
	case <-time.After(250 * time.Millisecond):
	}
}

func TestWatcher_StopClosesSubscriptions(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "test.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sub := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	err = w.Stop()
	require.NoError(t, err)

	select {
	case _, ok := <-sub:
		if ok {
			select {
			case _, ok := <-sub:
				require.False(t, ok)
			case <-time.After(100 * time.Millisecond):
			}
		}
	case <-time.After(200 * time.Millisecond):
	}
}

func TestWatcher_MultipleSubscribers(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "test.md")
	err := os.WriteFile(mdPath, []byte("test"), 0644)
	require.NoError(t, err)

	w, err := watcher.New(watcher.Config{
		BeansDir:    dir,
		DebounceDur: 50 * time.Millisecond,
	})
	require.NoError(t, err)
	defer func() { _ = w.Stop() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sub1 := w.Broker().Subscribe(ctx)
	sub2 := w.Broker().Subscribe(ctx)
	sub3 := w.Broker().Subscribe(ctx)

	err = w.Start()
	require.NoError(t, err)

	err = os.WriteFile(mdPath, []byte("modified"), 0644)
	require.NoError(t, err)

	receivedCount := 0
	timeout := time.After(300 * time.Millisecond)

	for i := 0; i < 3; i++ {
		select {
		case evt := <-sub1:
			require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
			receivedCount++
			sub1 = nil
		case evt := <-sub2:
			require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
			receivedCount++
			sub2 = nil
		case evt := <-sub3:
			require.Equal(t, watcher.BeansChanged, evt.Payload.Type)
			receivedCount++
			sub3 = nil
		case <-timeout:
			require.Fail(t, "timeout waiting for events - received %d of 3", receivedCount)
		}
	}

	require.Equal(t, 3, receivedCount)
}
