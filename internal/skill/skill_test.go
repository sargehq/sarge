package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiExtensionInstalled(t *testing.T) {
	t.Run("returns false when extension does not exist", func(t *testing.T) {
		dir := t.TempDir()
		if PiExtensionInstalled(dir) {
			t.Error("expected false for empty directory")
		}
	})

	t.Run("returns true when sarge-complete.ts exists", func(t *testing.T) {
		dir := t.TempDir()
		extDir := filepath.Join(dir, ".pi", "extensions")
		if err := os.MkdirAll(extDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(extDir, "sarge-complete.ts"), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !PiExtensionInstalled(dir) {
			t.Error("expected true when sarge-complete.ts exists")
		}
	})
}

func TestInstallPiExtension(t *testing.T) {
	dir := t.TempDir()

	if err := InstallPiExtension(dir); err != nil {
		t.Fatalf("InstallPiExtension failed: %v", err)
	}

	extPath := filepath.Join(dir, ".pi", "extensions", "sarge-complete.ts")
	if _, err := os.Stat(extPath); err != nil {
		t.Errorf("sarge-complete.ts not created: %v", err)
	}

	data, err := os.ReadFile(extPath) //nolint:gosec // test code
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("sarge-complete.ts is empty")
	}

	// Should contain expected content
	if !strings.Contains(string(data), "agent_end") {
		t.Error("sarge-complete.ts missing expected agent_end handler")
	}

	if !PiExtensionInstalled(dir) {
		t.Error("PiExtensionInstalled should return true after install")
	}
}

func TestInstallPiExtension_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := InstallPiExtension(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := InstallPiExtension(dir); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
}

func TestBeansPrimeExtensionInstalled(t *testing.T) {
	t.Run("returns false when extension does not exist", func(t *testing.T) {
		dir := t.TempDir()
		if BeansPrimeExtensionInstalled(dir) {
			t.Error("expected false for empty directory")
		}
	})

	t.Run("returns true when beans-prime.ts exists", func(t *testing.T) {
		dir := t.TempDir()
		extDir := filepath.Join(dir, ".pi", "extensions")
		if err := os.MkdirAll(extDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(extDir, "beans-prime.ts"), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !BeansPrimeExtensionInstalled(dir) {
			t.Error("expected true when beans-prime.ts exists")
		}
	})
}

func TestInstallBeansPrimeExtension(t *testing.T) {
	dir := t.TempDir()

	if err := InstallBeansPrimeExtension(dir); err != nil {
		t.Fatalf("InstallBeansPrimeExtension failed: %v", err)
	}

	extPath := filepath.Join(dir, ".pi", "extensions", "beans-prime.ts")
	if _, err := os.Stat(extPath); err != nil {
		t.Errorf("beans-prime.ts not created: %v", err)
	}

	data, err := os.ReadFile(extPath) //nolint:gosec // test code
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("beans-prime.ts is empty")
	}

	if !strings.Contains(string(data), "before_agent_start") {
		t.Error("beans-prime.ts missing expected before_agent_start handler")
	}

	if !BeansPrimeExtensionInstalled(dir) {
		t.Error("BeansPrimeExtensionInstalled should return true after install")
	}
}

func TestInstallBeansPrimeExtension_Idempotent(t *testing.T) {
	dir := t.TempDir()

	if err := InstallBeansPrimeExtension(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := InstallBeansPrimeExtension(dir); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
}

func TestClaudePluginInstalled(t *testing.T) {
	// This test checks the function doesn't panic with no home dir or missing file.
	// The actual result depends on the test environment.
	// Just verify it returns a bool without panicking.
	_ = ClaudePluginInstalled()
}

func TestClaudePluginInstalled_WithFile(t *testing.T) {
	// Create a temp dir to simulate ~/.claude/plugins/
	dir := t.TempDir()
	pluginsDir := filepath.Join(dir, ".claude", "plugins")
	if err := os.MkdirAll(pluginsDir, 0o750); err != nil {
		t.Fatal(err)
	}

	t.Run("returns true when beans plugin present", func(t *testing.T) {
		data := map[string]any{
			"version": 2,
			"plugins": map[string]any{
				"beans@beans-marketplace": []any{
					map[string]any{"scope": "project"},
				},
			},
		}
		b, _ := json.Marshal(data)
		path := filepath.Join(pluginsDir, "installed_plugins.json")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}

		installed := claudePluginInstalledFromFile(path)
		if !installed {
			t.Error("expected true when beans plugin is present")
		}
	})

	t.Run("returns false when beans plugin absent", func(t *testing.T) {
		data := map[string]any{
			"version": 2,
			"plugins": map[string]any{
				"other-plugin@marketplace": []any{},
			},
		}
		b, _ := json.Marshal(data)
		path := filepath.Join(pluginsDir, "installed_plugins.json")
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}

		installed := claudePluginInstalledFromFile(path)
		if installed {
			t.Error("expected false when beans plugin is absent")
		}
	})
}
