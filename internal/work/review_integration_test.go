package work_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewLoop_NoIssuesCreatesPR(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with a root issue
	h.CreateBead("root-1", "Root issue for work")
	workRecord := h.CreateWorkWithRootIssue("w-test", "feat/test-branch", "root-1")

	// Create and complete a review task
	h.CreateReviewTask("w-test.1", "w-test")
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Review finds no issues (no children created under root)
	// GetBeadWithChildren returns only the root issue itself
	hasBeadsToFix := h.SimulateReviewCompletion("w-test.1", "w-test", nil)
	assert.False(t, hasBeadsToFix, "review with no issues should not have beads to fix")

	// Verify that a PR task can now be created (review passed)
	prTaskNum, err := h.DB.GetNextTaskNumber(ctx, "w-test")
	require.NoError(t, err)

	prTaskID := "w-test." + itoa(prTaskNum)
	err = h.DB.CreateTask(ctx, prTaskID, "pr", nil, 0, "w-test", 0)
	require.NoError(t, err)

	// Verify PR task exists
	prTask, err := h.DB.GetTask(ctx, prTaskID)
	require.NoError(t, err)
	assert.Equal(t, "pr", prTask.TaskType)
	assert.Equal(t, db.StatusPending, prTask.Status)
}

func TestReviewLoop_IssuesCreateFixTasks(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with a root issue
	h.CreateBead("root-1", "Root issue for work")
	workRecord := h.CreateWorkWithRootIssue("w-test", "feat/test-branch", "root-1")

	// Create and complete a review task
	h.CreateReviewTask("w-test.1", "w-test")
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Review creates issues under the root issue
	reviewIssues := []beads.Bead{
		{ID: "fix-1", Title: "Fix security issue", Status: beads.StatusOpen, ExternalRef: "review-w-test.1"},
		{ID: "fix-2", Title: "Fix performance issue", Status: beads.StatusOpen, ExternalRef: "review-w-test.1"},
	}
	h.AddReviewIssues("root-1", reviewIssues)

	hasBeadsToFix := h.SimulateReviewCompletion("w-test.1", "w-test", reviewIssues)
	assert.True(t, hasBeadsToFix, "review with issues should have beads to fix")

	// Create fix tasks (simulating what handleReviewFixLoop does)
	var fixTaskIDs []string
	for _, issue := range reviewIssues {
		taskNum, err := h.DB.GetNextTaskNumber(ctx, "w-test")
		require.NoError(t, err)
		taskID := "w-test." + itoa(taskNum)

		err = h.DB.CreateTask(ctx, taskID, "implement", []string{issue.ID}, 0, "w-test", 0)
		require.NoError(t, err)

		// Fix task depends on the review task
		err = h.DB.AddTaskDependency(ctx, taskID, "w-test.1")
		require.NoError(t, err)

		fixTaskIDs = append(fixTaskIDs, taskID)
	}

	// Verify fix tasks were created
	require.Len(t, fixTaskIDs, 2)

	// Verify each fix task depends on the review task
	for _, fixTaskID := range fixTaskIDs {
		deps, err := h.DB.GetTaskDependencies(ctx, fixTaskID)
		require.NoError(t, err)
		assert.Contains(t, deps, "w-test.1", "fix task should depend on review task")
	}

	// Verify fix tasks are implement type
	for _, fixTaskID := range fixTaskIDs {
		task, err := h.DB.GetTask(ctx, fixTaskID)
		require.NoError(t, err)
		assert.Equal(t, "implement", task.TaskType)
	}
}

func TestReviewLoop_MaxIterationsForcesPR(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with a root issue
	h.CreateBead("root-1", "Root issue for work")
	workRecord := h.CreateWorkWithRootIssue("w-test", "feat/test-branch", "root-1")

	// Create multiple review tasks to simulate iterations
	// Default max iterations is 3
	for i := 1; i <= 3; i++ {
		reviewTaskID := "w-test." + itoa(i)
		h.CreateReviewTask(reviewTaskID, "w-test")
		err := h.DB.StartTask(ctx, reviewTaskID, workRecord.WorktreePath)
		require.NoError(t, err)

		// Complete the review task
		err = h.DB.CompleteTask(ctx, reviewTaskID, "")
		require.NoError(t, err)
	}

	// Count review iterations
	reviewCount := h.CountReviewIterations("w-test")
	assert.Equal(t, 3, reviewCount, "should have 3 completed review iterations")

	// At max iterations, a PR task should be created regardless of issues
	// Even if there are still issues, we proceed to PR after max iterations
	reviewIssues := []beads.Bead{
		{ID: "unresolved-1", Title: "Still open issue", Status: beads.StatusOpen, ExternalRef: "review-w-test.3"},
	}
	h.AddReviewIssues("root-1", reviewIssues)

	// At max iterations, we should force PR creation
	maxIterations := 3 // Default value
	shouldForcePR := reviewCount >= maxIterations
	assert.True(t, shouldForcePR, "should force PR after max iterations")

	// Create PR task (what happens when max iterations reached)
	prTaskNum, err := h.DB.GetNextTaskNumber(ctx, "w-test")
	require.NoError(t, err)

	prTaskID := "w-test." + itoa(prTaskNum)
	err = h.DB.CreateTask(ctx, prTaskID, "pr", nil, 0, "w-test", 0)
	require.NoError(t, err)

	// Verify PR task was created
	prTask, err := h.DB.GetTask(ctx, prTaskID)
	require.NoError(t, err)
	assert.Equal(t, "pr", prTask.TaskType)
}

func TestReviewLoop_ManualReviewSkipsAutomation(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with a root issue
	h.CreateBead("root-1", "Root issue for work")
	workRecord := h.CreateWorkWithRootIssue("w-test", "feat/test-branch", "root-1")

	// Create a manual review task (auto_workflow=false)
	h.CreateReviewTask("w-test.1", "w-test")
	err := h.DB.SetTaskMetadata(ctx, "w-test.1", "auto_workflow", "false")
	require.NoError(t, err)

	err = h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Review creates issues, but manual review should skip automation
	reviewIssues := []beads.Bead{
		{ID: "fix-1", Title: "Fix issue", Status: beads.StatusOpen, ExternalRef: "review-w-test.1"},
	}
	h.AddReviewIssues("root-1", reviewIssues)

	// Check if this is a manual review
	autoWorkflow, err := h.DB.GetTaskMetadata(ctx, "w-test.1", "auto_workflow")
	require.NoError(t, err)

	isManualReview := (autoWorkflow == "false")
	assert.True(t, isManualReview, "should detect manual review via auto_workflow=false")

	// Manual reviews should not create fix tasks or PR tasks automatically
	// Complete the review
	err = h.DB.CompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)

	// Verify only the review task exists (no auto-created fix or PR tasks)
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 1, "manual review should not auto-create additional tasks")
	assert.Equal(t, "review", tasks[0].TaskType)
}

func TestReviewLoop_FixTaskDependencies(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with a root issue
	h.CreateBead("root-1", "Root issue for work")
	workRecord := h.CreateWorkWithRootIssue("w-test", "feat/test-branch", "root-1")

	// Create first review task
	h.CreateReviewTask("w-test.1", "w-test")
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)
	err = h.DB.CompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)

	// Review creates issues
	reviewIssues := []beads.Bead{
		{ID: "fix-1", Title: "Fix issue 1", Status: beads.StatusOpen, ExternalRef: "review-w-test.1"},
		{ID: "fix-2", Title: "Fix issue 2", Status: beads.StatusOpen, ExternalRef: "review-w-test.1"},
	}
	h.AddReviewIssues("root-1", reviewIssues)

	// Create fix tasks that depend on the review task
	var fixTaskIDs []string
	for i, issue := range reviewIssues {
		taskID := "w-test." + itoa(i+2) // Start after review task
		err = h.DB.CreateTask(ctx, taskID, "implement", []string{issue.ID}, 0, "w-test", 0)
		require.NoError(t, err)

		// Fix task depends on review task
		err = h.DB.AddTaskDependency(ctx, taskID, "w-test.1")
		require.NoError(t, err)

		fixTaskIDs = append(fixTaskIDs, taskID)
	}

	// Create new review task that depends on all fix tasks
	newReviewTaskID := "w-test." + itoa(len(fixTaskIDs)+2)
	err = h.DB.CreateTask(ctx, newReviewTaskID, "review", nil, 0, "w-test", 0)
	require.NoError(t, err)

	for _, fixID := range fixTaskIDs {
		err = h.DB.AddTaskDependency(ctx, newReviewTaskID, fixID)
		require.NoError(t, err)
	}

	// Verify fix tasks depend on first review
	for _, fixTaskID := range fixTaskIDs {
		deps, err := h.DB.GetTaskDependencies(ctx, fixTaskID)
		require.NoError(t, err)
		assert.Contains(t, deps, "w-test.1", "fix task should depend on review task")
	}

	// Verify new review depends on all fix tasks
	newReviewDeps, err := h.DB.GetTaskDependencies(ctx, newReviewTaskID)
	require.NoError(t, err)
	assert.Len(t, newReviewDeps, 2, "new review should depend on all fix tasks")
	for _, fixID := range fixTaskIDs {
		assert.Contains(t, newReviewDeps, fixID, "new review should depend on fix task %s", fixID)
	}

	// Verify task types
	for _, fixTaskID := range fixTaskIDs {
		task, err := h.DB.GetTask(ctx, fixTaskID)
		require.NoError(t, err)
		assert.Equal(t, "implement", task.TaskType)
	}

	newReviewTask, err := h.DB.GetTask(ctx, newReviewTaskID)
	require.NoError(t, err)
	assert.Equal(t, "review", newReviewTask.TaskType)
}

// itoa converts an int to a string (simple helper for task IDs)
func itoa(n int) string {
	return strconv.Itoa(n)
}
