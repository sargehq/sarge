// Package watcher provides file system watching with debouncing for the beads database.
package watcher

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/sargehq/sarge/internal/beads/pubsub"

	"github.com/fsnotify/fsnotify"
)

// WatcherEventType identifies the kind of watcher event.
type WatcherEventType string

const (
	// DBChanged is emitted when the database file changes (after debounce).
	DBChanged WatcherEventType = "db_changed"
	// WatcherError is emitted when the watcher encounters an error (immediate, not debounced).
	WatcherError WatcherEventType = "error"
)

// WatcherEvent represents an event from the database watcher.
type WatcherEvent struct {
	Type  WatcherEventType
	Error error // Non-nil for WatcherError events
}

// Watcher monitors the beads directory for changes and publishes events via broker.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	watchDir  string
	debounce  time.Duration
	done      chan struct{}
	broker    *pubsub.Broker[WatcherEvent]
}

// Config holds watcher configuration options.
type Config struct {
	// WatchDir is the .beads directory to watch for changes.
	WatchDir    string
	DebounceDur time.Duration
}

// DefaultConfig returns sensible defaults for the watcher.
// beadsDir should be the .beads directory path.
func DefaultConfig(beadsDir string) Config {
	return Config{
		WatchDir:    beadsDir,
		DebounceDur: 100 * time.Millisecond,
	}
}

// New creates a new beads watcher.
func New(cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fsw,
		watchDir:  cfg.WatchDir,
		debounce:  cfg.DebounceDur,
		done:      make(chan struct{}),
		broker:    pubsub.NewBroker[WatcherEvent](),
	}, nil
}

// Start begins watching the beads directory.
// Subscribe to watcher events using Broker().Subscribe(ctx) instead of the old channel return.
func (w *Watcher) Start() error {
	if err := w.fsWatcher.Add(w.watchDir); err != nil {
		return fmt.Errorf("watching directory %s: %w", w.watchDir, err)
	}

	go w.loop()

	return nil
}

// Stop terminates the watcher and releases resources.
// CRITICAL SHUTDOWN SEQUENCE: broker.Close() must be called BEFORE fsWatcher.Close().
// This ensures subscribers receive clean channel close notifications before the underlying
// fsnotify watcher is destroyed. Reversing this order could leave subscribers hanging.
func (w *Watcher) Stop() error {
	close(w.done)
	w.broker.Close() // Close broker first to notify subscribers
	return w.fsWatcher.Close()
}

// Broker returns the pub/sub broker for subscribing to watcher events.
// The broker is created in New(), so it is always valid even before Start() is called.
func (w *Watcher) Broker() *pubsub.Broker[WatcherEvent] {
	return w.broker
}

// loop processes file system events with debouncing.
func (w *Watcher) loop() {
	var (
		timer   *time.Timer
		pending bool
	)

	for {
		select {
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}

			// Only react to writes on database files
			if !w.isRelevantEvent(event) {
				continue
			}

			// Reset or start debounce timer
			if timer == nil {
				timer = time.NewTimer(w.debounce)
				pending = true
			} else {
				if !timer.Stop() {
					// Drain the timer channel if it already fired
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.debounce)
				pending = true
			}

		case <-func() <-chan time.Time {
			if timer != nil {
				return timer.C
			}
			return nil
		}():
			if pending {
				// Publish DBChanged event to broker (non-blocking by design)
				w.broker.Publish(pubsub.UpdatedEvent, WatcherEvent{
					Type: DBChanged,
				})
				pending = false
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// Publish error event (immediate, not debounced)
			w.broker.Publish(pubsub.UpdatedEvent, WatcherEvent{
				Type:  WatcherError,
				Error: err,
			})

		case <-w.done:
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// isRelevantEvent checks if the event should trigger a refresh.
func (w *Watcher) isRelevantEvent(event fsnotify.Event) bool {
	// Only care about write or create operations
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	base := filepath.Base(event.Name)
	// Watch for JSONL changes (primary source of truth for bd CLI),
	// SQLite changes (legacy), and Dolt WAL changes.
	return base == "issues.jsonl" ||
		base == "beads.db" || base == "beads.db-wal" ||
		base == "interactions.jsonl"
}
