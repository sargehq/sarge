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

// Watcher monitors the beads database for changes and publishes events via broker.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	dbPath    string
	debounce  time.Duration
	done      chan struct{}
	broker    *pubsub.Broker[WatcherEvent]
}

// Config holds watcher configuration options.
type Config struct {
	DBPath      string
	DebounceDur time.Duration
}

// DefaultConfig returns sensible defaults for the watcher.
func DefaultConfig(dbPath string) Config {
	return Config{
		DBPath:      dbPath,
		DebounceDur: 100 * time.Millisecond,
	}
}

// New creates a new database watcher.
func New(cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fsw,
		dbPath:    cfg.DBPath,
		debounce:  cfg.DebounceDur,
		done:      make(chan struct{}),
		broker:    pubsub.NewBroker[WatcherEvent](),
	}, nil
}

// Start begins watching the database directory.
// Subscribe to watcher events using Broker().Subscribe(ctx) instead of the old channel return.
func (w *Watcher) Start() error {
	// Watch the directory containing the database
	dir := filepath.Dir(w.dbPath)
	if err := w.fsWatcher.Add(dir); err != nil {
		return fmt.Errorf("watching directory %s: %w", dir, err)
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
	// Only care about write or create operations (WAL file may be created fresh)
	if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return false
	}

	base := filepath.Base(event.Name)
	return base == "beads.db" || base == "beads.db-wal"
}
