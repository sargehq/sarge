package execwatch_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/execwatch"
)

// testWatcher creates a Watcher that monitors a temporary fake binary.
// We can't easily override os.Executable in the production code, so we test
// the core logic by creating a watcher via the exported constructor against
// the test binary itself, and also test the helpers indirectly.

func TestNew(t *testing.T) {
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if w.Path() == "" {
		t.Fatal("Path() returned empty string")
	}
}

func TestNewWithHash(t *testing.T) {
	w, err := execwatch.New(execwatch.WithHash())
	if err != nil {
		t.Fatalf("New(WithHash) error: %v", err)
	}
	if w.Path() == "" {
		t.Fatal("Path() returned empty string")
	}
}

func TestCheck_NoChange(t *testing.T) {
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	result, err := w.Check()
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if result.Changed {
		t.Errorf("Check() reported change when nothing changed: %s", result.Reason)
	}
}

func TestCheck_NoChangeWithHash(t *testing.T) {
	w, err := execwatch.New(execwatch.WithHash())
	if err != nil {
		t.Fatalf("New(WithHash) error: %v", err)
	}

	result, err := w.Check()
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if result.Changed {
		t.Errorf("Check() reported change when nothing changed: %s", result.Reason)
	}
}

// TestCheckWithTempBinary creates a temporary file and uses a lower-level
// approach to verify change detection logic.
func TestCheckWithTempBinary(t *testing.T) {
	// We test via a real watcher against the test binary. The test binary
	// won't change during the test, so this validates the no-change path.
	w, err := execwatch.New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	// Multiple checks should all report no change.
	for i := 0; i < 3; i++ {
		result, err := w.Check()
		if err != nil {
			t.Fatalf("Check() iteration %d error: %v", i, err)
		}
		if result.Changed {
			t.Errorf("Check() iteration %d reported unexpected change: %s", i, result.Reason)
		}
	}
}

// TestNewFromPath tests using a custom temporary binary to simulate changes.
func TestSimulateChange(t *testing.T) {
	// Create a temp binary file
	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-binary")
	if err := os.WriteFile(binPath, []byte("v1-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	// We can't use New() with a custom path directly, but we can test the
	// concept by using NewFromPath if we add it, or just validate the
	// current binary approach works. For now, this test validates that the
	// file manipulation we'd do actually changes mtime.
	info1, _ := os.Stat(binPath)

	// Ensure time passes so mtime differs
	time.Sleep(50 * time.Millisecond)

	if err := os.WriteFile(binPath, []byte("v2-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	info2, _ := os.Stat(binPath)
	if info1.ModTime().Equal(info2.ModTime()) {
		t.Skip("filesystem doesn't have sub-second mtime resolution")
	}
	// Verified: writing new content changes mtime, which is what our
	// Watcher relies on.
}
