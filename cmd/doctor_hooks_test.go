package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const beadsPostCheckoutHook = `#!/usr/bin/env bash
# bd-shim v1
# bd-hooks-version: abc123
exec bd hook post-checkout "$@"
`

const beadsPrepareMsgHook = `#!/usr/bin/env bash
# bd-shim v1
# bd-hooks-version: abc123
exec bd hooks run prepare-commit-msg "$@"
`

const nonBeadsHook = `#!/usr/bin/env bash
# Custom hook for CI checks
echo "Running lint..."
npm run lint
`

func writeHook(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755)
	require.NoError(t, err)
}

func TestCheckBeadsHooksInDir_NoHooksDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	issues, err := checkBeadsHooksInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, issues)
}

func TestCheckBeadsHooksInDir_NoBeadsHooks(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "pre-commit", nonBeadsHook)
	writeHook(t, dir, "post-merge", nonBeadsHook)

	issues, err := checkBeadsHooksInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, issues)
}

func TestCheckBeadsHooksInDir_ApplyRemovesBeadsHooks(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "post-checkout", beadsPostCheckoutHook)
	writeHook(t, dir, "prepare-commit-msg", beadsPrepareMsgHook)

	// Ensure apply mode (not dry-run)
	oldDryRun := doctorDryRun
	doctorDryRun = false
	defer func() { doctorDryRun = oldDryRun }()

	issues, err := checkBeadsHooksInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, issues)

	// Both files should be removed
	_, err = os.Stat(filepath.Join(dir, "post-checkout"))
	assert.True(t, os.IsNotExist(err), "post-checkout should be removed")
	_, err = os.Stat(filepath.Join(dir, "prepare-commit-msg"))
	assert.True(t, os.IsNotExist(err), "prepare-commit-msg should be removed")
}

func TestCheckBeadsHooksInDir_DryRunDoesNotRemove(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "post-checkout", beadsPostCheckoutHook)

	oldDryRun := doctorDryRun
	doctorDryRun = true
	defer func() { doctorDryRun = oldDryRun }()

	issues, err := checkBeadsHooksInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, issues)

	// File should still exist
	_, err = os.Stat(filepath.Join(dir, "post-checkout"))
	assert.NoError(t, err, "post-checkout should NOT be removed in dry-run")
}

func TestCheckBeadsHooksInDir_MixedHooks(t *testing.T) {
	dir := t.TempDir()
	writeHook(t, dir, "post-checkout", beadsPostCheckoutHook)
	writeHook(t, dir, "pre-commit", nonBeadsHook)

	oldDryRun := doctorDryRun
	doctorDryRun = false
	defer func() { doctorDryRun = oldDryRun }()

	issues, err := checkBeadsHooksInDir(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, issues)

	// Beads hook removed
	_, err = os.Stat(filepath.Join(dir, "post-checkout"))
	assert.True(t, os.IsNotExist(err), "beads hook should be removed")

	// Non-beads hook preserved
	_, err = os.Stat(filepath.Join(dir, "pre-commit"))
	assert.NoError(t, err, "non-beads hook should be preserved")
}

func TestIsBeadsHook(t *testing.T) {
	dir := t.TempDir()

	writeHook(t, dir, "beads-hook", beadsPostCheckoutHook)
	writeHook(t, dir, "normal-hook", nonBeadsHook)

	assert.True(t, isBeadsHook(filepath.Join(dir, "beads-hook")))
	assert.False(t, isBeadsHook(filepath.Join(dir, "normal-hook")))
	assert.False(t, isBeadsHook(filepath.Join(dir, "nonexistent")))
}
