package work_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sargehq/sarge/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDestroyWork_CleansUpAllResources(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with beads
	h.CreateBead("bead-1", "Test Bead 1")
	h.CreateBead("bead-2", "Test Bead 2")

	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")

	// Create a task associated with the work
	h.CreateTask("w-test.1", "w-test", []string{"bead-1", "bead-2"})

	// Verify work exists before destruction
	workBefore, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	require.NotNil(t, workBefore)

	// Verify beads are associated
	beadsBefore, err := h.DB.GetWorkBeads(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, beadsBefore, 2)

	// Track worktree removal
	worktreeRemoved := false
	h.Worktree.RemoveForceFunc = func(ctx context.Context, repoPath string, worktreePath string) error {
		worktreeRemoved = true
		assert.Equal(t, workRecord.WorktreePath, worktreePath)
		return nil
	}

	// Destroy the work
	var output bytes.Buffer
	err = h.WorkService.DestroyWork(ctx, "w-test", &output)
	require.NoError(t, err)

	// Verify worktree was removed
	assert.True(t, worktreeRemoved, "worktree should have been removed")

	// Verify database records deleted
	workAfter, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Nil(t, workAfter, "work record should be deleted")

	// Verify work_beads cleaned up
	beadsAfter, err := h.DB.GetWorkBeads(ctx, "w-test")
	require.NoError(t, err)
	assert.Empty(t, beadsAfter, "work beads should be deleted")

	// Verify task was deleted
	task, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Nil(t, task, "task should be deleted")
}

func TestDestroyWork_ClosesRootIssue(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create bead that will be the root issue
	h.CreateBead("root-bead", "Root Issue")

	// Create work with root issue
	err := h.DB.CreateWork(ctx, "w-test", "Test Work", "/test/worktree", "feat/test", "main", "root-bead", false)
	require.NoError(t, err)

	// Track if close was called
	closeCalled := false
	var closedBeadID string
	h.Beads.CloseFunc = func(ctx context.Context, beadID string) error {
		closeCalled = true
		closedBeadID = beadID
		return nil
	}

	// Destroy the work
	err = h.WorkService.DestroyWork(ctx, "w-test", io.Discard)
	require.NoError(t, err)

	// Verify root issue was closed
	assert.True(t, closeCalled, "beads.Close should have been called")
	assert.Equal(t, "root-bead", closedBeadID, "should close the root issue")
}

func TestDestroyWork_TerminatesZellijTabs(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work
	h.CreateWork("w-test", "feat/test")

	// Track if tab termination was called
	terminateCalled := false
	var terminatedWorkID string
	h.OrchestratorManager.TerminateWorkTabsFunc = func(ctx context.Context, workID string, projName string, w io.Writer) error {
		terminateCalled = true
		terminatedWorkID = workID
		return nil
	}

	// Destroy the work
	err := h.WorkService.DestroyWork(ctx, "w-test", io.Discard)
	require.NoError(t, err)

	// Verify zellij tabs were terminated
	assert.True(t, terminateCalled, "TerminateWorkTabs should have been called")
	assert.Equal(t, "w-test", terminatedWorkID)
}

func TestDestroyWork_HandlesPartialFailures(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with root issue
	h.CreateBead("root-bead", "Root Issue")
	err := h.DB.CreateWork(ctx, "w-test", "Test Work", "/test/worktree", "feat/test", "main", "root-bead", false)
	require.NoError(t, err)

	// Configure beads close to fail
	h.Beads.CloseFunc = func(ctx context.Context, beadID string) error {
		return errors.New("beads service unavailable")
	}

	// Configure tab termination to fail
	h.OrchestratorManager.TerminateWorkTabsFunc = func(ctx context.Context, workID string, projName string, w io.Writer) error {
		return errors.New("zellij not running")
	}

	// Configure worktree removal to fail
	h.Worktree.RemoveForceFunc = func(ctx context.Context, repoPath string, worktreePath string) error {
		return errors.New("worktree in use")
	}

	// Destroy should succeed despite partial failures (warnings only)
	var output bytes.Buffer
	err = h.WorkService.DestroyWork(ctx, "w-test", &output)
	require.NoError(t, err)

	// Verify warnings were logged
	outputStr := output.String()
	assert.Contains(t, outputStr, "Warning")

	// Database cleanup should still have happened
	workAfter, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Nil(t, workAfter, "work record should be deleted even with partial failures")
}

func TestDestroyWork_WorktreeRemovalFailure(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with worktree path
	workRecord := h.CreateWork("w-test", "feat/test")
	require.NotEmpty(t, workRecord.WorktreePath)

	// Configure worktree removal to fail
	worktreeRemoveCalled := false
	h.Worktree.RemoveForceFunc = func(ctx context.Context, repoPath string, worktreePath string) error {
		worktreeRemoveCalled = true
		return errors.New("permission denied: worktree locked")
	}

	// Destroy should succeed (worktree failure is non-fatal)
	var output bytes.Buffer
	err := h.WorkService.DestroyWork(ctx, "w-test", &output)
	require.NoError(t, err)

	// Verify removal was attempted
	assert.True(t, worktreeRemoveCalled, "worktree removal should have been attempted")

	// Verify warning in output
	assert.Contains(t, output.String(), "Warning")
	assert.Contains(t, output.String(), "worktree")

	// Database should still be cleaned up
	workAfter, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Nil(t, workAfter, "work should be deleted from DB")
}

func TestDestroyWork_WorkNotFound(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Try to destroy non-existent work
	err := h.WorkService.DestroyWork(ctx, "non-existent-work", io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDestroyWork_NoWorktreePath(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work without worktree path (e.g., still pending)
	err := h.DB.CreateWork(ctx, "w-pending", "Pending Work", "", "feat/pending", "main", "", false)
	require.NoError(t, err)

	// Track worktree removal calls
	removeCalled := false
	h.Worktree.RemoveForceFunc = func(ctx context.Context, repoPath string, worktreePath string) error {
		removeCalled = true
		return nil
	}

	// Destroy should succeed without trying to remove worktree
	err = h.WorkService.DestroyWork(ctx, "w-pending", io.Discard)
	require.NoError(t, err)

	// Worktree removal should NOT have been called
	assert.False(t, removeCalled, "should not try to remove empty worktree path")

	// Work should be deleted
	workAfter, err := h.DB.GetWork(ctx, "w-pending")
	require.NoError(t, err)
	assert.Nil(t, workAfter)
}

func TestDestroyWork_NoRootIssue(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work without root issue
	h.CreateWork("w-no-root", "feat/no-root")

	// Track if close was called
	closeCalled := false
	h.Beads.CloseFunc = func(ctx context.Context, beadID string) error {
		closeCalled = true
		return nil
	}

	// Destroy should succeed without trying to close root issue
	err := h.WorkService.DestroyWork(ctx, "w-no-root", io.Discard)
	require.NoError(t, err)

	// Close should NOT have been called
	assert.False(t, closeCalled, "should not try to close non-existent root issue")
}

func TestDestroyWork_KillTabsDisabled(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Configure zellij to not kill tabs on destroy
	h.Config.Zellij.KillTabsOnDestroy = boolPtr(false)

	// Create work
	h.CreateWork("w-test", "feat/test")

	// Track if tab termination was called
	terminateCalled := false
	h.OrchestratorManager.TerminateWorkTabsFunc = func(ctx context.Context, workID string, projName string, w io.Writer) error {
		terminateCalled = true
		return nil
	}

	// Destroy the work
	err := h.WorkService.DestroyWork(ctx, "w-test", io.Discard)
	require.NoError(t, err)

	// Tab termination should NOT have been called
	assert.False(t, terminateCalled, "TerminateWorkTabs should not be called when disabled")
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(b bool) *bool {
	return &b
}
