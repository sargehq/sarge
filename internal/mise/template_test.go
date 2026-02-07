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
	// Default: no agent, gh and zellij active
	assert.NotContains(t, s, "claude")
	assert.NotContains(t, s, "pi-coding-agent")
	assert.Contains(t, s, "gh = \"latest\"")
	assert.NotContains(t, s, "# gh = \"latest\"")
	assert.Contains(t, s, "zellij = \"latest\"")
	assert.NotContains(t, s, "# zellij = \"latest\"")
	assert.Contains(t, s, "beads")
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

func TestGenerateConfigWithSelections_AgentNone(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "none",
		AgentInMise:   false,
		IncludeGH:     true,
		IncludeZellij: true,
	}

	err := GenerateConfigWithSelections(dir, sel)
	require.NoError(t, err)

	s := string(readMiseConfig(t, dir))
	assert.NotContains(t, s, "claude")
	assert.NotContains(t, s, "pi-coding-agent")
}

func TestGenerateConfigWithSelections_GHCommented(t *testing.T) {
	dir := t.TempDir()
	sel := ToolSelections{
		AgentType:     "none",
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
		AgentType:     "none",
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
	assert.Contains(t, s, "beads")
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
	assert.Equal(t, "", sel.AgentType)
	assert.False(t, sel.AgentInMise)
	assert.True(t, sel.IncludeGH)
	assert.True(t, sel.IncludeZellij)
}
