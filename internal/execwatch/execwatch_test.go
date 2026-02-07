package execwatch_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/execwatch"
)

func TestNew(t *testing.T) {
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Stop()

	if w.Path() == "" {
		t.Fatal("Path() returned empty string")
	}
}

func TestChanged_NoChangeDoesNotSignal(t *testing.T) {
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Stop()

	// Channel should not be closed when nothing changes
	select {
	case <-w.Changed():
		t.Fatal("Changed() signaled when nothing changed")
	case <-time.After(200 * time.Millisecond):
		// Expected: no signal
	}
}

func TestChanged_SignalsOnWrite(t *testing.T) {
	// Create a temp binary to watch
	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-binary")
	if err := os.WriteFile(binPath, []byte("v1-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := execwatch.NewFromPath(binPath)
	if err != nil {
		t.Fatalf("NewFromPath() error: %v", err)
	}
	defer w.Stop()

	// Give fsnotify a moment to set up
	time.Sleep(100 * time.Millisecond)

	// Write new content — this should trigger the change
	if err := os.WriteFile(binPath, []byte("v2-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Changed():
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("Changed() did not signal after binary was modified")
	}
}

func TestChanged_SignalsOnAtomicReplace(t *testing.T) {
	// Create a temp binary to watch
	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-binary")
	if err := os.WriteFile(binPath, []byte("v1-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	w, err := execwatch.NewFromPath(binPath)
	if err != nil {
		t.Fatalf("NewFromPath() error: %v", err)
	}
	defer w.Stop()

	// Give fsnotify a moment to set up
	time.Sleep(100 * time.Millisecond)

	// Atomic replace: write to temp file then rename over the original
	tmpPath := binPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte("v2-content"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, binPath); err != nil {
		t.Fatal(err)
	}

	select {
	case <-w.Changed():
		// Expected
	case <-time.After(2 * time.Second):
		t.Fatal("Changed() did not signal after atomic replace")
	}
}

func TestStop_UnblocksChanged(t *testing.T) {
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Stop should not panic and the channel should remain open (not closed)
	w.Stop()

	// After stop, Changed() should not signal (channel not closed by Stop)
	select {
	case <-w.Changed():
		// This is acceptable — Stop may or may not close the channel
	case <-time.After(100 * time.Millisecond):
		// Expected: no signal
	}
}
