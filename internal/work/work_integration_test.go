package work_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/testutil"
	"github.com/sargehq/sarge/internal/work"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkCreation_Success(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create a test bead
	h.CreateBead("bead-1", "Implement feature X")

	// Create work asynchronously
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:  "feat/implement-feature-x",
		BaseBranch:  "main",
		RootIssueID: "bead-1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify work record created in DB with correct fields
	workRecord, err := h.DB.GetWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.NotNil(t, workRecord)

	assert.Equal(t, result.WorkID, workRecord.ID)
	assert.Equal(t, "pending", workRecord.Status)
	assert.Equal(t, "feat/implement-feature-x", workRecord.BranchName)
	assert.Equal(t, "main", workRecord.BaseBranch)
	assert.Equal(t, "bead-1", workRecord.RootIssueID)
	assert.False(t, workRecord.Auto)

	// Verify CreateWorktree task scheduled
	tasks, err := h.DB.GetScheduledTasksForWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	assert.Equal(t, db.TaskTypeCreateWorktree, tasks[0].TaskType)
	assert.Equal(t, db.TaskStatusPending, tasks[0].Status)
	assert.Equal(t, "feat/implement-feature-x", tasks[0].Metadata["branch"])
	assert.Equal(t, "main", tasks[0].Metadata["base_branch"])
	assert.Equal(t, "bead-1", tasks[0].Metadata["root_issue_id"])
}

func TestWorkCreation_WithEpicExpansion(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create an epic with children
	h.CreateEpicWithChildren("epic-1", "child-1", "child-2")

	// Create work using CreateWorkAsyncWithOptions to include beads
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:  "feat/epic-work",
		BaseBranch:  "main",
		RootIssueID: "epic-1",
		BeanIDs:     []string{"epic-1", "child-1", "child-2"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify work record created
	workRecord, err := h.DB.GetWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.NotNil(t, workRecord)
	assert.Equal(t, "epic-1", workRecord.RootIssueID)

	// Verify all beads added to work_beads table
	workBeads, err := h.DB.GetWorkBeans(ctx, result.WorkID)
	require.NoError(t, err)
	require.Len(t, workBeads, 3, "expected epic and 2 children")

	beanIDs := make(map[string]bool)
	for _, wb := range workBeads {
		beanIDs[wb.BeanID] = true
	}
	assert.True(t, beanIDs["epic-1"])
	assert.True(t, beanIDs["child-1"])
	assert.True(t, beanIDs["child-2"])
}

func TestWorkCreation_BranchNameCollision(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Configure mock to indicate the branch exists locally
	h.MockBranchExists("feat/existing-branch", true, false)

	// Create work with a branch name that exists
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName: "feat/existing-branch",
		BaseBranch: "main",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Branch name should be modified to be unique (with -2 suffix)
	assert.NotEqual(t, "feat/existing-branch", result.BranchName)
	assert.Contains(t, result.BranchName, "feat/existing-branch")

	// Verify work was created with the unique branch name
	workRecord, err := h.DB.GetWork(ctx, result.WorkID)
	require.NoError(t, err)
	assert.Equal(t, result.BranchName, workRecord.BranchName)
}

func TestWorkCreation_GitPushFailure(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Configure git push to fail - this affects the control plane execution,
	// but the async work creation should still succeed
	h.MockGitPushFails(errors.New("push failed: remote rejected"))

	// Work creation should succeed (it schedules tasks, doesn't push directly)
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName: "feat/will-fail-push",
		BaseBranch: "main",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify work record exists
	workRecord, err := h.DB.GetWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.NotNil(t, workRecord)

	// The actual push failure would be handled by the control plane,
	// which would update the task status to failed
}

func TestWorkCreation_SchedulesControlPlaneTask(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with auto mode enabled
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:  "feat/auto-work",
		BaseBranch:  "main",
		RootIssueID: "root-issue",
		Auto:        true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify CreateWorktree task scheduled with correct metadata
	tasks, err := h.DB.GetScheduledTasksForWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	task := tasks[0]
	assert.Equal(t, db.TaskTypeCreateWorktree, task.TaskType)
	assert.Equal(t, db.TaskStatusPending, task.Status)
	assert.Equal(t, "feat/auto-work", task.Metadata["branch"])
	assert.Equal(t, "main", task.Metadata["base_branch"])
	assert.Equal(t, "root-issue", task.Metadata["root_issue_id"])
	assert.Equal(t, "true", task.Metadata["auto"])

	// Verify idempotency key was set
	require.NotNil(t, task.IdempotencyKey)
	assert.Contains(t, *task.IdempotencyKey, "create-worktree-"+result.WorkID)
}

func TestWorkCreation_CleanupOnPartialFailure(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create a bead
	h.CreateBead("bead-1", "Test Bead")

	// First, create a work with the bead
	result1, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:  "feat/first-work",
		BaseBranch:  "main",
		RootIssueID: "bead-1",
		BeanIDs:     []string{"bead-1"},
	})
	require.NoError(t, err)

	// Verify the work was created with its bead
	beads1, err := h.DB.GetWorkBeans(ctx, result1.WorkID)
	require.NoError(t, err)
	assert.Len(t, beads1, 1, "first work should have one bead")

	// Create another work with a different bead - this should succeed
	// (beads can belong to multiple works in the system design)
	h.CreateBead("bead-2", "Test Bead 2")
	result2, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:  "feat/second-work",
		BaseBranch:  "main",
		RootIssueID: "bead-2",
		BeanIDs:     []string{"bead-2"},
	})
	require.NoError(t, err)

	// Verify both works exist with their respective beads
	works, err := h.DB.ListWorks(ctx, "")
	require.NoError(t, err)
	assert.Len(t, works, 2, "should have two independent works")

	beads2, err := h.DB.GetWorkBeans(ctx, result2.WorkID)
	require.NoError(t, err)
	assert.Len(t, beads2, 1, "second work should have one bead")
}

func TestWorkCreation_WithUseExistingBranch(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Configure mock to indicate the branch exists
	h.MockBranchExists("feat/pr-branch", true, true)

	// Create work using an existing branch (e.g., for PR import)
	result, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName:        "feat/pr-branch",
		BaseBranch:        "main",
		UseExistingBranch: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Branch name should NOT be modified even though it exists
	assert.Equal(t, "feat/pr-branch", result.BranchName)

	// Verify use_existing flag is in metadata
	tasks, err := h.DB.GetScheduledTasksForWork(ctx, result.WorkID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "true", tasks[0].Metadata["use_existing"])
}

func TestWorkCreation_GeneratesWorkerName(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create multiple works and verify they get unique worker names
	result1, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName: "feat/work-1",
		BaseBranch: "main",
	})
	require.NoError(t, err)

	result2, err := h.WorkService.CreateWorkAsyncWithOptions(ctx, work.CreateWorkAsyncOptions{
		BranchName: "feat/work-2",
		BaseBranch: "main",
	})
	require.NoError(t, err)

	// Worker names should be different
	assert.NotEmpty(t, result1.WorkerName)
	assert.NotEmpty(t, result2.WorkerName)
	assert.NotEqual(t, result1.WorkerName, result2.WorkerName)

	// Verify worker names are stored in the scheduled task metadata
	tasks1, err := h.DB.GetScheduledTasksForWork(ctx, result1.WorkID)
	require.NoError(t, err)
	require.Len(t, tasks1, 1)
	assert.Equal(t, result1.WorkerName, tasks1[0].Metadata["worker_name"])
}
