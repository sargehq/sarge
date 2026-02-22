package skill

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiSkillInstalled(t *testing.T) {
	t.Run("returns false when skill dir does not exist", func(t *testing.T) {
		dir := t.TempDir()
		if PiSkillInstalled(dir) {
			t.Error("expected false for empty directory")
		}
	})

	t.Run("returns true when SKILL.md exists", func(t *testing.T) {
		dir := t.TempDir()
		skillDir := filepath.Join(dir, ".pi", "skills", "beads")
		if err := os.MkdirAll(skillDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !PiSkillInstalled(dir) {
			t.Error("expected true when SKILL.md exists")
		}
	})
}

func TestInstallPiSkill(t *testing.T) {
	dir := t.TempDir()

	if err := InstallPiSkill(dir); err != nil {
		t.Fatalf("InstallPiSkill failed: %v", err)
	}

	// Check that SKILL.md was created
	skillPath := filepath.Join(dir, ".pi", "skills", "beads", "SKILL.md")
	if _, err := os.Stat(skillPath); err != nil {
		t.Errorf("SKILL.md not created: %v", err)
	}

	// Check that resource files were created
	resourceFiles := []string{
		"CLI_REFERENCE.md",
		"DEPENDENCIES.md",
		"PITFALLS.md",
		"WORKFLOWS.md",
	}
	for _, name := range resourceFiles {
		path := filepath.Join(dir, ".pi", "skills", "beads", "resources", name)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("resource file %s not created: %v", name, err)
		}
	}

	// Verify SKILL.md has expected content
	data, err := os.ReadFile(skillPath) //nolint:gosec // test code, path from t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("SKILL.md is empty")
	}

	// Verify it's now detected as installed
	if !PiSkillInstalled(dir) {
		t.Error("PiSkillInstalled should return true after install")
	}
}

func TestInstallPiSkill_Idempotent(t *testing.T) {
	dir := t.TempDir()

	// Install twice — should not error
	if err := InstallPiSkill(dir); err != nil {
		t.Fatalf("first install failed: %v", err)
	}
	if err := InstallPiSkill(dir); err != nil {
		t.Fatalf("second install failed: %v", err)
	}
}

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

	t.Run("returns true when beads plugin present", func(t *testing.T) {
		data := map[string]any{
			"version": 2,
			"plugins": map[string]any{
				"beads@beads-marketplace": []any{
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
			t.Error("expected true when beads plugin is present")
		}
	})

	t.Run("returns false when beads plugin absent", func(t *testing.T) {
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
			t.Error("expected false when beads plugin is absent")
		}
	})
}
