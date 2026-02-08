package mise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredBeadsVersion(t *testing.T) {
	v := RequiredBeadsVersion()
	assert.NotEmpty(t, v, "should extract beads version from template")
	assert.True(t, len(v) > 1 && v[0] == 'v', "version should start with 'v': got %q", v)
}

func TestReadBeadsVersion(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantVer   string
		wantCom   bool
		wantFound bool
	}{
		{
			name:      "active beads line",
			content:   "[tools]\n\"aqua:steveyegge/beads\" = \"v0.49.2\"\ngh = \"latest\"\n",
			wantVer:   "v0.49.2",
			wantCom:   false,
			wantFound: true,
		},
		{
			name:      "commented beads line",
			content:   "[tools]\n# \"aqua:steveyegge/beads\" = \"v0.48.0\"\n",
			wantVer:   "v0.48.0",
			wantCom:   true,
			wantFound: true,
		},
		{
			name:      "no beads line",
			content:   "[tools]\ngh = \"latest\"\n",
			wantVer:   "",
			wantCom:   false,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".mise.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0600))

			ver, commented, err := ReadBeadsVersion(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantVer, ver)
			assert.Equal(t, tt.wantCom, commented)
			if tt.wantFound {
				assert.NotEmpty(t, ver)
			} else {
				assert.Empty(t, ver)
			}
		})
	}
}

func TestUpdateBeadsVersion(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		newVersion string
		wantMod    bool
		wantLine   string
	}{
		{
			name:       "update version",
			content:    "[tools]\n\"aqua:steveyegge/beads\" = \"v0.48.0\"\ngh = \"latest\"\n",
			newVersion: "v0.49.2",
			wantMod:    true,
			wantLine:   "\"aqua:steveyegge/beads\" = \"v0.49.2\"",
		},
		{
			name:       "already correct",
			content:    "[tools]\n\"aqua:steveyegge/beads\" = \"v0.49.2\"\n",
			newVersion: "v0.49.2",
			wantMod:    false,
		},
		{
			name:       "no beads line",
			content:    "[tools]\ngh = \"latest\"\n",
			newVersion: "v0.49.2",
			wantMod:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".mise.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0600))

			modified, err := UpdateBeadsVersion(path, tt.newVersion)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMod, modified)

			if tt.wantMod {
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Contains(t, string(data), tt.wantLine)
				// Ensure other lines are preserved
				assert.Contains(t, string(data), "[tools]")
			}
		})
	}
}
