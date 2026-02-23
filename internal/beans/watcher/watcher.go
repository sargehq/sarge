// Package watcher provides file system watching with debouncing for the beans directory.
package watcher

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/beans/pubsub"

	"github.com/fsnotify/fsnotify"
)

// WatcherEventType identifies the kind of watcher event.
type WatcherEventType string

const (
	// BeansChanged is emitted when a bean markdown file changes (after debounce).
	BeansChanged WatcherEventType = "beans_changed"
	// WatcherError is emitted when the watcher encounters an error (immediate, not debounced).
	WatcherError WatcherEventType = "error"
)

// WatcherEvent represents an event from the beans directory watcher.
type WatcherEvent struct {
	Type  WatcherEventType
	Error error // Non-nil for WatcherError events
}

// Watcher monitors the .beans/ directory for markdown file changes and publishes events via broker.
type Watcher struct {
	fsWatcher *fsnotify.Watcher
	beansDir  string
	debounce  time.Duration
	done      chan struct{}
	broker    *pubsub.Broker[WatcherEvent]
}

// Config holds watcher configuration options.
type Config struct {
	BeansDir    string
	DebounceDur time.Duration
}

// DefaultConfig returns sensible defaults for the watcher.
func DefaultConfig(beansDir string) Config {
	return Config{
		BeansDir:    beansDir,
		DebounceDur: 100 * time.Millisecond,
	}
}

// New creates a new beans directory watcher.
func New(cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	return &Watcher{
		fsWatcher: fsw,
		beansDir:  cfg.BeansDir,
		debounce:  cfg.DebounceDur,
		done:      make(chan struct{}),
		broker:    pubsub.NewBroker[WatcherEvent](),
	}, nil
}

// Start begins watching the beans directory.
func (w *Watcher) Start() error {
	if err := w.fsWatcher.Add(w.beansDir); err != nil {
		return fmt.Errorf("watching directory %s: %w", w.beansDir, err)
	}

	go w.loop()

	return nil
}

// Stop terminates the watcher and releases resources.
// CRITICAL SHUTDOWN SEQUENCE: broker.Close() must be called BEFORE fsWatcher.Close().
func (w *Watcher) Stop() error {
	close(w.done)
	w.broker.Close()
	return w.fsWatcher.Close()
}

// Broker returns the pub/sub broker for subscribing to watcher events.
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

			if !w.isRelevantEvent(event) {
				continue
			}

			if timer == nil {
				timer = time.NewTimer(w.debounce)
				pending = true
			} else {
				if !timer.Stop() {
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
				w.broker.Publish(pubsub.UpdatedEvent, WatcherEvent{
					Type: BeansChanged,
				})
				pending = false
			}

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
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
// Only reacts to .md file changes in the beans directory.
func (w *Watcher) isRelevantEvent(event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	base := filepath.Base(event.Name)
	return strings.HasSuffix(base, ".md")
}
