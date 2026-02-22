// Package skill provides beads skill detection and installation for coding agents.
package skill

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed beads/SKILL.md beads/resources/*.md
var beadsSkillFS embed.FS

//go:embed extensions/sarge-complete.ts
var sargeCompleteExtension []byte

// PiSkillInstalled checks whether the pi beads skill exists in the given repo directory.
func PiSkillInstalled(repoDir string) bool {
	skillPath := filepath.Join(repoDir, ".pi", "skills", "beads", "SKILL.md")
	_, err := os.Stat(skillPath)
	return err == nil
}

// InstallPiSkill copies the embedded beads skill files into the repo's .pi/skills/beads/ directory.
func InstallPiSkill(repoDir string) error {
	return fs.WalkDir(beadsSkillFS, "beads", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Map "beads/..." to ".pi/skills/beads/..."
		destPath := filepath.Join(repoDir, ".pi", "skills", path)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0o750)
		}

		data, err := beadsSkillFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
			return fmt.Errorf("creating directory for %s: %w", destPath, err)
		}

		return os.WriteFile(destPath, data, 0o600)
	})
}

// PiExtensionInstalled checks whether the sarge-complete pi extension exists in the given repo directory.
func PiExtensionInstalled(repoDir string) bool {
	extPath := filepath.Join(repoDir, ".pi", "extensions", "sarge-complete.ts")
	_, err := os.Stat(extPath)
	return err == nil
}

// InstallPiExtension copies the embedded sarge-complete extension into the repo's .pi/extensions/ directory.
func InstallPiExtension(repoDir string) error {
	extDir := filepath.Join(repoDir, ".pi", "extensions")
	if err := os.MkdirAll(extDir, 0o750); err != nil {
		return fmt.Errorf("creating extensions directory: %w", err)
	}

	destPath := filepath.Join(extDir, "sarge-complete.ts")
	return os.WriteFile(destPath, sargeCompleteExtension, 0o600)
}

// ClaudePluginInstalled checks whether the beads plugin is installed for Claude Code.
// It reads ~/.claude/plugins/installed_plugins.json and looks for the "beads@beads-marketplace" key.
func ClaudePluginInstalled() bool {
	return claudePluginInstalledFromFile(claudePluginsPath())
}

// claudePluginInstalledFromFile checks a specific installed_plugins.json file for the beads plugin.
func claudePluginInstalledFromFile(path string) bool {
	if path == "" {
		return false
	}

	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath) //nolint:gosec // path is constructed from os.UserHomeDir(), not user input
	if err != nil {
		return false
	}

	var installed struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(data, &installed); err != nil {
		return false
	}

	_, ok := installed.Plugins["beads@beads-marketplace"]
	return ok
}

// ClaudeInstallInstructions returns the user-facing instructions for installing the beads plugin in Claude Code.
func ClaudeInstallInstructions() string {
	return `Open Claude Code and run:
   /plugin marketplace add steveyegge/beads
   /plugin install beads`
}

// claudePluginsPath returns the path to the Claude plugins file.
func claudePluginsPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".claude", "plugins", "installed_plugins.json")
}
