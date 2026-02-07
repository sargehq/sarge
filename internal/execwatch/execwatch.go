// Package execwatch detects when the running executable has been replaced on disk.
//
// It uses fsnotify to watch the directory containing the binary for changes,
// and signals via a channel when the executable has been modified or replaced.
package execwatch

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors the running executable for changes using filesystem events.
type Watcher struct {
	path    string
	modTime time.Time
	changed chan struct{} // closed when a change is detected
	done    chan struct{} // closed to stop the watcher
	fsw     *fsnotify.Watcher
}

// New creates a Watcher that tracks the currently running executable.
// It resolves the executable path via os.Executable and begins watching
// its parent directory for changes. When the binary is modified or replaced,
// the channel returned by Changed() is closed.
func New() (*Watcher, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("execwatch: resolve executable: %w", err)
	}
	return NewFromPath(exePath)
}

// NewFromPath creates a Watcher that tracks the file at the given path.
// This is useful for testing with a specific file path.
func NewFromPath(path string) (*Watcher, error) {
	// Resolve symlinks so we watch the real file's directory
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("execwatch: eval symlinks: %w", err)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("execwatch: stat executable: %w", err)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("execwatch: create fsnotify watcher: %w", err)
	}

	// Watch the directory (not the file directly) because installers often
	// do an atomic rename, which replaces the inode. Watching the directory
	// catches both in-place writes and atomic replacements.
	dir := filepath.Dir(resolved)
	if err := fsw.Add(dir); err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("execwatch: watch directory %s: %w", dir, err)
	}

	w := &Watcher{
		path:    resolved,
		modTime: info.ModTime(),
		changed: make(chan struct{}),
		done:    make(chan struct{}),
		fsw:     fsw,
	}

	go w.loop()

	return w, nil
}

// Changed returns a channel that is closed when the executable has been
// modified or replaced on disk. This is safe to use in a select statement.
func (w *Watcher) Changed() <-chan struct{} {
	return w.changed
}

// Path returns the resolved path to the watched executable.
func (w *Watcher) Path() string {
	return w.path
}

// Stop terminates the watcher and releases resources.
func (w *Watcher) Stop() {
	close(w.done)
	_ = w.fsw.Close()
}

// loop processes fsnotify events and signals when the binary changes.
func (w *Watcher) loop() {
	baseName := filepath.Base(w.path)

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}

			// Only care about our binary
			if filepath.Base(event.Name) != baseName {
				continue
			}

			// Any event on the binary is worth checking (macOS/kqueue reports
			// CHMOD for atomic replaces like `go build -o`, not Write/Create).
			info, err := os.Stat(w.path)
			if err != nil {
				if os.IsNotExist(err) {
					// Binary was removed (possibly mid atomic-replace) — wait
					// briefly for the new file to appear
					time.Sleep(200 * time.Millisecond)
					info, err = os.Stat(w.path)
					if err != nil {
						// Still gone — signal change
						close(w.changed)
						return
					}
				} else {
					continue
				}
			}

			if !info.ModTime().Equal(w.modTime) {
				close(w.changed)
				return
			}

		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Ignore errors — the watcher will keep trying

		case <-w.done:
			return
		}
	}
}
