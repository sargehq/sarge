package worktree

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewOperations(t *testing.T) {
	ops := NewOperations()
	require.NotNil(t, ops, "NewOperations returned nil")

	// Verify it returns a CLIOperations
	_, ok := ops.(*CLIOperations)
	require.True(t, ok, "NewOperations should return *CLIOperations")
}

func TestCLIOperationsImplementsInterface(t *testing.T) {
	// Compile-time check that CLIOperations implements Operations
	var _ Operations = (*CLIOperations)(nil)
}

func TestSetMiseTrustedPaths(t *testing.T) {
	t.Run("sets project root as trusted path", func(t *testing.T) {
		cmd := exec.Command("echo")
		setMiseTrustedPaths(cmd, "/home/user/myproj/w-abc/tree")

		var miseTrust string
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=") {
				miseTrust = strings.TrimPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=")
			}
		}
		require.Equal(t, "/home/user/myproj", miseTrust)
	})

	t.Run("preserves existing trusted paths", func(t *testing.T) {
		t.Setenv("MISE_TRUSTED_CONFIG_PATHS", "/existing/path")

		cmd := exec.Command("echo")
		setMiseTrustedPaths(cmd, "/home/user/myproj/w-abc/tree")

		var miseTrust string
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=") {
				miseTrust = strings.TrimPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=")
			}
		}
		require.Equal(t, "/existing/path:/home/user/myproj", miseTrust)
	})

	t.Run("no existing env var", func(t *testing.T) {
		// Ensure env var is not set
		os.Unsetenv("MISE_TRUSTED_CONFIG_PATHS")

		cmd := exec.Command("echo")
		setMiseTrustedPaths(cmd, "/projects/foo/w-123/tree")

		var miseTrust string
		for _, env := range cmd.Env {
			if strings.HasPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=") {
				miseTrust = strings.TrimPrefix(env, "MISE_TRUSTED_CONFIG_PATHS=")
			}
		}
		require.Equal(t, "/projects/foo", miseTrust)
	})
}

func TestParseWorktreeList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Worktree
	}{
		{
			name:     "empty output",
			input:    "",
			expected: nil,
		},
		{
			name: "single worktree",
			input: `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main
`,
			expected: []Worktree{
				{Path: "/path/to/main", HEAD: "abc123def456", Branch: "main"},
			},
		},
		{
			name: "multiple worktrees",
			input: `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main

worktree /path/to/feature
HEAD def789ghi012
branch refs/heads/feature-branch

`,
			expected: []Worktree{
				{Path: "/path/to/main", HEAD: "abc123def456", Branch: "main"},
				{Path: "/path/to/feature", HEAD: "def789ghi012", Branch: "feature-branch"},
			},
		},
		{
			name: "detached HEAD worktree",
			input: `worktree /path/to/detached
HEAD abc123def456
detached

`,
			expected: []Worktree{
				{Path: "/path/to/detached", HEAD: "abc123def456", Branch: ""},
			},
		},
		{
			name: "no trailing newline",
			input: `worktree /path/to/main
HEAD abc123def456
branch refs/heads/main`,
			expected: []Worktree{
				{Path: "/path/to/main", HEAD: "abc123def456", Branch: "main"},
			},
		},
		{
			name: "path with spaces",
			input: `worktree /path/with spaces/to/main
HEAD abc123def456
branch refs/heads/main
`,
			expected: []Worktree{
				{Path: "/path/with spaces/to/main", HEAD: "abc123def456", Branch: "main"},
			},
		},
		{
			name: "branch with slashes",
			input: `worktree /path/to/feature
HEAD abc123def456
branch refs/heads/feature/sub-feature/deep
`,
			expected: []Worktree{
				{Path: "/path/to/feature", HEAD: "abc123def456", Branch: "feature/sub-feature/deep"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseWorktreeList(tt.input)
			require.NoError(t, err)

			require.Len(t, result, len(tt.expected))

			for i, expected := range tt.expected {
				require.Equal(t, expected.Path, result[i].Path, "worktree[%d] path mismatch", i)
				require.Equal(t, expected.HEAD, result[i].HEAD, "worktree[%d] HEAD mismatch", i)
				require.Equal(t, expected.Branch, result[i].Branch, "worktree[%d] branch mismatch", i)
			}
		})
	}
}
