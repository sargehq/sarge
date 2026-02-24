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

	s := string(readMiseConfig(t, dir))
	// Default: claude present but commented out, gh and zellij active
	assert.Contains(t, s, "# claude = \"latest\"")
	assert.NotContains(t, s, "pi-coding-agent")
	assert.Contains(t, s, "gh = \"latest\"")
	assert.NotContains(t, s, "# gh = \"latest\"")
	assert.Contains(t, s, "zellij = \"latest\"")
	assert.NotContains(t, s, "# zellij = \"latest\"")
	assert.Contains(t, s, "beans")
}

func TestGenerateConfigWithSelections_AgentClaudeActive(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		AgentInMise:   true,
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "claude = \"latest\"")
	assert.NotContains(t, s, "# claude = \"latest\"")
	assert.NotContains(t, s, "pi-coding-agent")
}

func TestGenerateConfigWithSelections_AgentClaudeCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		AgentInMise:   false,
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "# claude = \"latest\"")
	// Make sure the uncommented version is NOT present
	// (the commented line contains "claude" so we check for the exact uncommented pattern)
	assert.NotRegexp(t, `(?m)^claude = "latest"`, s)
}

func TestGenerateConfigWithSelections_AgentPiActive(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "pi",
		AgentInMise:   true,
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "\"npm:@mariozechner/pi-coding-agent\" = \"latest\"")
	assert.NotContains(t, s, "# \"npm:@mariozechner/pi-coding-agent\"")
	assert.NotContains(t, s, "claude")
}

func TestGenerateConfigWithSelections_AgentPiCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "pi",
		AgentInMise:   false,
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "# \"npm:@mariozechner/pi-coding-agent\" = \"latest\"")
	assert.NotRegexp(t, `(?m)^"npm:@mariozechner/pi-coding-agent"`, s)
}

func TestGenerateConfigWithSelections_GHCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		IncludeGH:     false,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "# gh = \"latest\"")
	assert.NotRegexp(t, `(?m)^gh = "latest"`, s)
	assert.Contains(t, s, "zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_ZellijCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		IncludeGH:     true,
		IncludeZellij: false,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "gh = \"latest\"")
	assert.Contains(t, s, "# zellij = \"latest\"")
	assert.NotRegexp(t, `(?m)^zellij = "latest"`, s)
}

func TestGenerateConfigWithSelections_AllCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "claude",
		AgentInMise:   false,
		IncludeGH:     false,
		IncludeZellij: false,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "beans")
	assert.Contains(t, s, "# claude = \"latest\"")
	assert.Contains(t, s, "# gh = \"latest\"")
	assert.Contains(t, s, "# zellij = \"latest\"")
}

func TestGenerateConfigWithSelections_SkipsExistingConfig(t *testing.T) {
	dir := t.TempDir()

	existingContent := []byte("# existing config")
	err := os.WriteFile(filepath.Join(dir, ".mise.toml"), existingContent, 0600)
	require.NoError(t, err)

	sel := DefaultToolSelections()
	err = GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	content := readMiseConfig(t, dir)
	assert.Equal(t, existingContent, content)
}

func TestDefaultToolSelections(t *testing.T) {
	sel := DefaultToolSelections()
	assert.Equal(t, "claude", sel.AgentType)
	assert.False(t, sel.AgentInMise)
	assert.True(t, sel.IncludeGH)
	assert.True(t, sel.IncludeZellij)
}

func TestRegenerateConfigWithSelections_CreatesBackupOfExistingConfig(t *testing.T) {
	dir := t.TempDir()

	// Write an initial config that simulates a user-customized .mise.toml.
	originalContent := []byte("# user customized config\n[tools]\nmy-custom-tool = \"1.0.0\"\n")
	configPath := filepath.Join(dir, ".mise.toml")
	err := os.WriteFile(configPath, originalContent, 0600)
	require.NoError(t, err)

	sel := DefaultToolSelections()
	err = RegenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	// The backup should contain the original content.
	backupContent, err := os.ReadFile(configPath + ".bak") //nolint:gosec // path constructed from t.TempDir()
	require.NoError(t, err, "backup file should have been created")
	assert.Equal(t, originalContent, backupContent, "backup should preserve original content")

	// The config file itself should be overwritten with new content.
	newContent := readMiseConfig(t, dir)
	assert.NotEqual(t, originalContent, newContent, "config should have been overwritten")
	assert.Contains(t, string(newContent), "beans", "new config should contain sarge defaults")
}

func TestRegenerateConfigWithSelections_NoBackupWhenNoExistingConfig(t *testing.T) {
	dir := t.TempDir()

	sel := DefaultToolSelections()
	err := RegenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	// No backup file should exist when there was no pre-existing config.
	_, err = os.Stat(filepath.Join(dir, ".mise.toml.bak"))
	assert.True(t, os.IsNotExist(err), "no backup should be created when no existing config")

	// The config should have been generated.
	s := string(readMiseConfig(t, dir))
	assert.Contains(t, s, "beans")
}
