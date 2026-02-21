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
		{"myproject", "editor", "sarge-myproject--editor"},
		{"my-project", "my-tab", "sarge-my-project--my-tab"},
		{"proj", "tab1", "sarge-proj--tab1"},
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
		{"sarge-myproject--editor", "myproject", "editor"},
		{"sarge-my-project--my-tab", "my-project", "my-tab"},
		{"sarge-proj--tab1", "proj", "tab1"},
		// Double-dash in tab name: only splits on first --
		{"sarge-proj--tab--extra", "proj", "tab--extra"},
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
		{"sarge-nodashdash"},
		{"other-prefix--tab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, tab := ParseSessionName(tt.name)
			require.Empty(t, project)
			require.Empty(t, tab)
		})
	}
}

func TestSessionNameRoundTrip(t *testing.T) {
	project := "my-project"
	tab := "my-tab"

	name := SessionName(project, tab)
	gotProject, gotTab := ParseSessionName(name)

	require.Equal(t, project, gotProject)
	require.Equal(t, tab, gotTab)
}
