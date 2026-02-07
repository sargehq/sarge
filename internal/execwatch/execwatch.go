// Package execwatch detects when the running executable has been replaced on disk.
//
// On startup, it records the path and modification time of the current binary.
// The Check method compares the current state against the recorded values to
// detect when a new binary has been installed.
package execwatch

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Watcher monitors the running executable for changes.
type Watcher struct {
	mu      sync.Mutex
	path    string
	modTime time.Time
	hash    []byte // optional, computed only if WithHash is used
}

// Option configures a Watcher.
type Option func(*Watcher) error

// WithHash records an initial SHA-256 hash of the executable for more robust
// change detection. This is more expensive at startup but handles edge cases
// where mtime is not updated (e.g., some build tools).
func WithHash() Option {
	return func(w *Watcher) error {
		h, err := hashFile(w.path)
		if err != nil {
			return fmt.Errorf("execwatch: hash executable: %w", err)
		}
		w.hash = h
		return nil
	}
}

// New creates a Watcher that tracks the currently running executable.
// It resolves the executable path via os.Executable and records its initial
// modification time.
func New(opts ...Option) (*Watcher, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("execwatch: resolve executable: %w", err)
	}

	info, err := os.Stat(exePath)
	if err != nil {
		return nil, fmt.Errorf("execwatch: stat executable: %w", err)
	}

	w := &Watcher{
		path:    exePath,
		modTime: info.ModTime(),
	}

	for _, opt := range opts {
		if err := opt(w); err != nil {
			return nil, err
		}
	}

	return w, nil
}

// Result describes what changed about the executable.
type Result struct {
	Changed bool
	// Reason describes why the binary is considered changed (e.g., "mtime changed",
	// "hash changed", "binary missing").
	Reason string
}

// Check tests whether the executable on disk has changed since the Watcher was
// created. It is safe to call from multiple goroutines.
//
// The check is stat-based by default (cheap). If the Watcher was created with
// WithHash and the mtime has changed, a hash comparison is also performed.
func (w *Watcher) Check() (Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, err := os.Stat(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Changed: true, Reason: "binary missing"}, nil
		}
		return Result{}, fmt.Errorf("execwatch: stat executable: %w", err)
	}

	if !info.ModTime().Equal(w.modTime) {
		// If we have a hash, do a deeper comparison — the mtime change may be
		// cosmetic (e.g., touch without content change).
		if w.hash != nil {
			h, err := hashFile(w.path)
			if err != nil {
				return Result{}, fmt.Errorf("execwatch: hash executable: %w", err)
			}
			if !equalBytes(w.hash, h) {
				return Result{Changed: true, Reason: "hash changed"}, nil
			}
			// mtime changed but hash is the same — update stored mtime so we
			// don't keep re-hashing.
			w.modTime = info.ModTime()
			return Result{Changed: false}, nil
		}
		return Result{Changed: true, Reason: "mtime changed"}, nil
	}

	return Result{Changed: false}, nil
}

// Path returns the resolved path to the watched executable.
func (w *Watcher) Path() string {
	return w.path
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
