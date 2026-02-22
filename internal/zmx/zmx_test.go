package zmx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	c := New()
	require.NotNil(t, c, "New() returned nil")
}

func TestSessionName(t *testing.T) {
	tests := []struct {
		project  string
		tab      string
		expected string
	}{
		{"myproject", "editor", "sarge-myproject.editor"},
		{"my-project", "my-tab", "sarge-my-project.my-tab"},
		{"proj", "tab1", "sarge-proj.tab1"},
		// Friendly names with spaces and parens are sanitized
		{"proj", "orch-w-abc (my feature)", "sarge-proj.orch-w-abc-my-feature"},
		{"proj", "console-w-abc (stellar_zhukovsky)", "sarge-proj.console-w-abc-stellar_zhukovsky"},
		// Double quotes, single quotes, and backslashes are stripped
		{"proj", `tab-with"quotes`, "sarge-proj.tab-withquotes"},
		{"proj", `tab-with'single`, "sarge-proj.tab-withsingle"},
		{"proj", `tab-with\backslash`, "sarge-proj.tab-withbackslash"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := SessionName(tt.project, tt.tab)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestParseSessionName(t *testing.T) {
	tests := []struct {
		name        string
		wantProject string
		wantTab     string
	}{
		{"sarge-myproject.editor", "myproject", "editor"},
		{"sarge-my-project.my-tab", "my-project", "my-tab"},
		{"sarge-proj.tab1", "proj", "tab1"},
		// Dot in tab name: only splits on first dot
		{"sarge-proj.tab.extra", "proj", "tab.extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, tab := ParseSessionName(tt.name)
			require.Equal(t, tt.wantProject, project)
			require.Equal(t, tt.wantTab, tab)
		})
	}
}

func TestParseSessionName_Invalid(t *testing.T) {
	tests := []struct {
		name string
	}{
		{""},
		{"random-session"},
		{"sarge-nodot"},
		{"other-prefix.tab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, tab := ParseSessionName(tt.name)
			require.Empty(t, project)
			require.Empty(t, tab)
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-dashes_and.dots", "with-dashes_and.dots"},
		{"/path/to/bin", "/path/to/bin"},
		{"KEY=value", "KEY=value"},
		{"has space", "'has space'"},
		{"export FOO=bar && pi", "'export FOO=bar && pi'"},
		{"it's", "'it'\\''s'"},
		{"(parens)", "'(parens)'"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := shellQuote(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildGhosttyTabAppleScript(t *testing.T) {
	script := buildGhosttyTabAppleScript("sarge-myproject.editor")
	require.Contains(t, script, `tell application "System Events"`)
	require.Contains(t, script, `tell process "Ghostty"`)
	require.Contains(t, script, `click menu item "New Tab" of menu "File" of menu bar item "File" of menu bar 1`)
	require.Contains(t, script, `set frontmost to true`)
	require.Contains(t, script, `zmx attach sarge-myproject.editor`)
}

func TestBuildGhosttyTabAppleScript_SessionNameIncluded(t *testing.T) {
	script := buildGhosttyTabAppleScript("sarge-proj.console-w-abc-stellar")
	require.Contains(t, script, "zmx attach sarge-proj.console-w-abc-stellar")
}

func TestSessionNameRoundTrip(t *testing.T) {
	project := "my-project"
	tab := "my-tab"

	name := SessionName(project, tab)
	gotProject, gotTab := ParseSessionName(name)

	require.Equal(t, project, gotProject)
	require.Equal(t, tab, gotTab)
}
