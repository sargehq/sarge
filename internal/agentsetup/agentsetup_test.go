package agentsetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPiExtensionInstalled(t *testing.T) {
	t.Run("false when missing", func(t *testing.T) {
		assert.False(t, PiExtensionInstalled(t.TempDir()))
	})

	t.Run("true when present", func(t *testing.T) {
		dir := t.TempDir()
		extDir := filepath.Join(dir, ".pi", "extensions")
		require.NoError(t, os.MkdirAll(extDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(extDir, "sarge-complete.ts"), []byte("test"), 0o644))
		assert.True(t, PiExtensionInstalled(dir))
	})
}

func TestInstallPiExtension(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InstallPiExtension(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".pi", "extensions", "sarge-complete.ts")) //nolint:gosec
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.True(t, strings.Contains(string(data), "agent_end"), "missing agent_end handler")
	assert.True(t, PiExtensionInstalled(dir))
}

func TestInstallPiExtension_Idempotent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InstallPiExtension(dir))
	require.NoError(t, InstallPiExtension(dir))
}

func TestBeansPrimeExtensionInstalled(t *testing.T) {
	t.Run("false when missing", func(t *testing.T) {
		assert.False(t, BeansPrimeExtensionInstalled(t.TempDir()))
	})

	t.Run("true when present", func(t *testing.T) {
		dir := t.TempDir()
		extDir := filepath.Join(dir, ".pi", "extensions")
		require.NoError(t, os.MkdirAll(extDir, 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(extDir, "beans-prime.ts"), []byte("test"), 0o644))
		assert.True(t, BeansPrimeExtensionInstalled(dir))
	})
}

func TestInstallBeansPrimeExtension(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InstallBeansPrimeExtension(dir))

	data, err := os.ReadFile(filepath.Join(dir, ".pi", "extensions", "beans-prime.ts")) //nolint:gosec
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	assert.True(t, strings.Contains(string(data), "before_agent_start"), "missing before_agent_start handler")
	assert.True(t, BeansPrimeExtensionInstalled(dir))
}

func TestInstallBeansPrimeExtension_Idempotent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, InstallBeansPrimeExtension(dir))
	require.NoError(t, InstallBeansPrimeExtension(dir))
}

func TestClaudePluginInstalled(t *testing.T) {
	// Just verify it doesn't panic in any environment.
	_ = ClaudePluginInstalled()
}

func TestClaudePluginInstalledFromFile(t *testing.T) {
	writePlugins := func(t *testing.T, dir string, plugins map[string]any) string {
		t.Helper()
		b, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
		require.NoError(t, err)
		path := filepath.Join(dir, "installed_plugins.json")
		require.NoError(t, os.WriteFile(path, b, 0o644))
		return path
	}

	t.Run("true when beans plugin present", func(t *testing.T) {
		path := writePlugins(t, t.TempDir(), map[string]any{
			"beans@beans-marketplace": []any{map[string]any{"scope": "project"}},
		})
		assert.True(t, claudePluginInstalledFromFile(path))
	})

	t.Run("false when beans plugin absent", func(t *testing.T) {
		path := writePlugins(t, t.TempDir(), map[string]any{
			"other-plugin@marketplace": []any{},
		})
		assert.False(t, claudePluginInstalledFromFile(path))
	})

	t.Run("false for empty path", func(t *testing.T) {
		assert.False(t, claudePluginInstalledFromFile(""))
	})

	t.Run("false for missing file", func(t *testing.T) {
		assert.False(t, claudePluginInstalledFromFile("/nonexistent/path.json"))
	})
}
