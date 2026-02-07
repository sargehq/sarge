package mise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readMiseConfig reads the .mise.toml file from the given directory.
// This helper avoids gosec G304 warnings in tests since the path is constructed from t.TempDir().
func readMiseConfig(t *testing.T, dir string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, ".mise.toml")) //nolint:gosec // path constructed from t.TempDir()
	require.NoError(t, err)
	return content
}

func TestGenerateConfigWithSelections_DefaultSelections(t *testing.T) {
	dir := t.TempDir()
	sel := DefaultToolSelections()

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	assert.Contains(t, s, "claude = \"latest\"")
	assert.Contains(t, s, "gh = \"latest\"")
	assert.Contains(t, s, "zellij = \"latest\"")
	assert.Contains(t, s, "beads")
}

func TestGenerateConfigWithSelections_AgentNone(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "none",
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	assert.NotContains(t, s, "claude")
	assert.NotContains(t, s, "pi-coding-agent")
	assert.Contains(t, s, "gh = \"latest\"")
	assert.Contains(t, s, "zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_AgentPi(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "pi",
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	assert.Contains(t, s, "pi-coding-agent")
	assert.NotContains(t, s, "claude")
}

func TestGenerateConfigWithSelections_NoGH(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		IncludeGH:     false,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	assert.Contains(t, s, "claude = \"latest\"")
	assert.NotContains(t, s, "gh = \"latest\"")
	assert.Contains(t, s, "zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_NoZellij(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		IncludeGH:     true,
		IncludeZellij: false,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	assert.Contains(t, s, "claude = \"latest\"")
	assert.Contains(t, s, "gh = \"latest\"")
	assert.NotContains(t, s, "zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_AllToolsDisabled(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "none",
		IncludeGH:     false,
		IncludeZellij: false,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	s := string(content)
	// Only beads should remain
	assert.Contains(t, s, "beads")
	assert.NotContains(t, s, "claude")
	assert.NotContains(t, s, "pi-coding-agent")
	assert.NotContains(t, s, "gh = \"latest\"")
	assert.NotContains(t, s, "zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_SkipsExistingConfig(t *testing.T) {
	dir := t.TempDir()

	// Create an existing config
	existingContent := []byte("# existing config")
	err := os.WriteFile(filepath.Join(dir, ".mise.toml"), existingContent, 0600)
	require.NoError(t, err)

	sel := DefaultToolSelections()
	err = GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	// Should not have overwritten
	content := readMiseConfig(t, dir)
	assert.Equal(t, existingContent, content)
}

func TestGenerateConfigWithSelections_DefaultsEmptyAgentToClaude(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "", // empty
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)

	assert.Contains(t, string(content), "claude = \"latest\"")
}

func TestDefaultToolSelections(t *testing.T) {
	sel := DefaultToolSelections()
	assert.Equal(t, "claude", sel.AgentType)
	assert.True(t, sel.IncludeGH)
	assert.True(t, sel.IncludeZellij)
}
