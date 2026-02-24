package mise

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequiredBeansVersion(t *testing.T) {
	v := RequiredBeansVersion()
	assert.NotEmpty(t, v, "should extract beans version from template")
	assert.True(t, len(v) > 0, "version should not be empty: got %q", v)
}

func TestReadBeansVersion(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantVer   string
		wantCom   bool
		wantFound bool
	}{
		{
			name:      "active beans line",
			content:   "[tools]\n\"go:github.com/hmans/beans\" = \"0.4.0\"\ngh = \"latest\"\n",
			wantVer:   "0.4.0",
			wantCom:   false,
			wantFound: true,
		},
		{
			name:      "commented beans line",
			content:   "[tools]\n# \"go:github.com/hmans/beans\" = \"0.3.0\"\n",
			wantVer:   "0.3.0",
			wantCom:   true,
			wantFound: true,
		},
		{
			name:      "no beans line",
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

			ver, commented, err := ReadBeansVersion(path)
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

func TestUpdateBeansVersion(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		newVersion string
		wantMod    bool
		wantLine   string
	}{
		{
			name:       "update version",
			content:    "[tools]\n\"go:github.com/hmans/beans\" = \"0.3.0\"\ngh = \"latest\"\n",
			newVersion: "0.4.0",
			wantMod:    true,
			wantLine:   "\"go:github.com/hmans/beans\" = \"0.4.0\"",
		},
		{
			name:       "already correct",
			content:    "[tools]\n\"go:github.com/hmans/beans\" = \"0.4.0\"\n",
			newVersion: "0.4.0",
			wantMod:    false,
		},
		{
			name:       "no beans line",
			content:    "[tools]\ngh = \"latest\"\n",
			newVersion: "0.4.0",
			wantMod:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".mise.toml")
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0600))

			modified, err := UpdateBeansVersion(path, tt.newVersion)
			require.NoError(t, err)
			assert.Equal(t, tt.wantMod, modified)

			if tt.wantMod {
				data, err := os.ReadFile(path) //nolint:gosec // G304: path is a test temp file we created
				require.NoError(t, err)
				assert.Contains(t, string(data), tt.wantLine)
				// Ensure other lines are preserved
				assert.Contains(t, string(data), "[tools]")
			}
		})
	}
}
