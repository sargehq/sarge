package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteWork(t *testing.T) {
	// Create a test database
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	workID := "w-test"
	branchName := "feature/test"
	baseBranch := "main"
	worktreePath := "/tmp/test-work/tree"

	err := db.CreateWork(ctx, workID, "", worktreePath, branchName, baseBranch, "root-issue-1", false)
	require.NoError(t, err, "Failed to create work")

	// Create tasks for the work
	task1ID := "w-test.1"
	task2ID := "w-test.2"

	err = db.CreateTask(ctx, task1ID, "implement", []string{"bean-1", "bean-2"}, 50, workID, 0)
	require.NoError(t, err, "Failed to create task 1")

	err = db.CreateTask(ctx, task2ID, "implement", []string{"bean-3"}, 30, workID, 0)
	require.NoError(t, err, "Failed to create task 2")

	// Note: CreateTask already adds the task to work_tasks when workID is provided,
	// so we don't need to call AddTaskToWork separately.

	// Verify work exists
	work, err := db.GetWork(ctx, workID)
	require.NoError(t, err, "Failed to get work")
	require.NotNil(t, work, "Work should exist")

	// Verify tasks exist
	tasks, err := db.GetWorkTasks(ctx, workID)
	require.NoError(t, err, "Failed to get work tasks")
	require.Len(t, tasks, 2, "Expected 2 tasks")

	// Delete the work
	err = db.DeleteWork(ctx, workID)
	require.NoError(t, err, "Failed to delete work")

	// Verify work is deleted
	work, err = db.GetWork(ctx, workID)
	require.NoError(t, err, "Failed to get work after deletion")
	assert.Nil(t, work, "Work should be deleted")

	// Verify tasks are deleted
	task1, err := db.GetTask(ctx, task1ID)
	require.NoError(t, err, "Failed to get task 1 after deletion")
	assert.Nil(t, task1, "Task 1 should be deleted")

	task2, err := db.GetTask(ctx, task2ID)
	require.NoError(t, err, "Failed to get task 2 after deletion")
	assert.Nil(t, task2, "Task 2 should be deleted")

	// Verify work_tasks relationships are deleted
	tasks, err = db.GetWorkTasks(ctx, workID)
	require.NoError(t, err, "Failed to get work tasks after deletion")
	assert.Empty(t, tasks, "Expected 0 tasks after deletion")
}

func TestDeleteWorkNotFound(t *testing.T) {
	// Create a test database
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Try to delete a non-existent work
	err := db.DeleteWork(ctx, "w-nonexistent")
	assert.Error(t, err, "Expected error when deleting non-existent work")
}

func TestAddWorkBeans(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work first
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Add beans to work
	err = db.AddWorkBeans(ctx, "w-test", []string{"bean-1", "bean-2"})
	require.NoError(t, err)

	// Verify beans were added
	beans, err := db.GetWorkBeans(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, beans, 2)
}

func TestAddWorkBeansDuplicateError(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work first
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Add initial beans
	err = db.AddWorkBeans(ctx, "w-test", []string{"bean-1", "bean-2"})
	require.NoError(t, err)

	// Try to add duplicate beans - should error
	err = db.AddWorkBeans(ctx, "w-test", []string{"bean-2", "bean-3"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "beans already exist in work")
	assert.Contains(t, err.Error(), "bean-2")
}

func TestAddWorkBeansEmptyList(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work first
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Add empty list - should succeed with no effect
	err = db.AddWorkBeans(ctx, "w-test", []string{})
	require.NoError(t, err)

	beans, err := db.GetWorkBeans(ctx, "w-test")
	require.NoError(t, err)
	assert.Empty(t, beans)
}

func TestAddWorkBeansMultipleBatches(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work first
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Add beans in multiple batches
	err = db.AddWorkBeans(ctx, "w-test", []string{"bean-1", "bean-2"})
	require.NoError(t, err)
	err = db.AddWorkBeans(ctx, "w-test", []string{"bean-3"})
	require.NoError(t, err)

	// Verify all beans were added
	beans, err := db.GetWorkBeans(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, beans, 3)
}

func TestWorkRootIssueID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work with a root issue ID
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "root-bean-123", false)
	require.NoError(t, err)

	// Verify root issue ID was stored
	work, err := db.GetWork(ctx, "w-test")
	require.NoError(t, err)
	require.NotNil(t, work)
	assert.Equal(t, "root-bean-123", work.RootIssueID)
}

func TestWorkRootIssueIDEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work without a root issue ID (empty string)
	err := db.CreateWork(ctx, "w-test", "", "/tmp/tree", "feature/test", "main", "", false)
	require.NoError(t, err)

	// Verify empty root issue ID
	work, err := db.GetWork(ctx, "w-test")
	require.NoError(t, err)
	require.NotNil(t, work)
	assert.Equal(t, "", work.RootIssueID)
}

func TestListWorksWithRootIssueID(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create works with different root issue IDs
	err := db.CreateWork(ctx, "w-test1", "", "/tmp/tree1", "feature/test1", "main", "bean-1", false)
	require.NoError(t, err)
	err = db.CreateWork(ctx, "w-test2", "", "/tmp/tree2", "feature/test2", "main", "bean-2", false)
	require.NoError(t, err)

	// List all works
	works, err := db.ListWorks(ctx, "")
	require.NoError(t, err)
	assert.Len(t, works, 2)

	// Verify root issue IDs are preserved
	rootIssues := make(map[string]string)
	for _, w := range works {
		rootIssues[w.ID] = w.RootIssueID
	}
	assert.Equal(t, "bean-1", rootIssues["w-test1"])
	assert.Equal(t, "bean-2", rootIssues["w-test2"])
}

func TestWorkStatusTransitionToCompleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	workID := "w-test"
	err := db.CreateWork(ctx, workID, "", "/tmp/tree", "feature/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Verify initial status is pending
	work, err := db.GetWork(ctx, workID)
	require.NoError(t, err)
	assert.Equal(t, StatusPending, work.Status)

	// Start the work (transitions to processing)
	err = db.StartWork(ctx, workID)
	require.NoError(t, err)

	work, err = db.GetWork(ctx, workID)
	require.NoError(t, err)
	assert.Equal(t, StatusProcessing, work.Status)

	// Create tasks for the work
	task1ID := workID + ".1"
	task2ID := workID + ".2"
	err = db.CreateTask(ctx, task1ID, "implement", []string{"bean-1"}, 10, workID, 0)
	require.NoError(t, err)
	err = db.CreateTask(ctx, task2ID, "implement", []string{"bean-2"}, 10, workID, 0)
	require.NoError(t, err)

	// Verify work is not completed yet
	isCompleted, err := db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.False(t, isCompleted)

	// Complete the first task
	err = db.CompleteTask(ctx, task1ID, "")
	require.NoError(t, err)

	// Verify work is still not completed (one task remaining)
	isCompleted, err = db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.False(t, isCompleted)

	// Complete the second task
	err = db.CompleteTask(ctx, task2ID, "")
	require.NoError(t, err)

	// Now all tasks are completed
	isCompleted, err = db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.True(t, isCompleted)

	// Complete the work (this is what the orchestrator does when all tasks complete)
	prURL := "https://github.com/example/repo/pull/123"
	err = db.CompleteWork(ctx, workID, prURL)
	require.NoError(t, err)

	// Verify work status is completed
	work, err = db.GetWork(ctx, workID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, work.Status)
	assert.Equal(t, prURL, work.PRURL)
	assert.NotNil(t, work.CompletedAt)
}

func TestWorkStatusTransitionToCompletedWithoutPR(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create and start a work
	workID := "w-test"
	err := db.CreateWork(ctx, workID, "", "/tmp/tree", "feature/test", "main", "", false)
	require.NoError(t, err)
	err = db.StartWork(ctx, workID)
	require.NoError(t, err)

	// Create and complete a task
	taskID := workID + ".1"
	err = db.CreateTask(ctx, taskID, "implement", []string{"bean-1"}, 10, workID, 0)
	require.NoError(t, err)
	err = db.CompleteTask(ctx, taskID, "")
	require.NoError(t, err)

	// Complete work without a PR URL (empty string)
	err = db.CompleteWork(ctx, workID, "")
	require.NoError(t, err)

	// Verify work status is completed even without PR URL
	work, err := db.GetWork(ctx, workID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, work.Status)
	assert.Equal(t, "", work.PRURL)
	assert.NotNil(t, work.CompletedAt)
}

func TestIsWorkCompletedWithNoTasks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a work without any tasks
	workID := "w-test"
	err := db.CreateWork(context.Background(), workID, "", "/tmp/tree", "feature/test", "main", "", false)
	require.NoError(t, err)

	// Work with no tasks should not be considered completed
	isCompleted, err := db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.False(t, isCompleted)
}

func TestIsWorkCompletedWithPartialCompletion(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work with multiple tasks
	workID := "w-test"
	err := db.CreateWork(ctx, workID, "", "/tmp/tree", "feature/test", "main", "", false)
	require.NoError(t, err)

	// Create three tasks
	err = db.CreateTask(ctx, workID+".1", "implement", []string{"bean-1"}, 10, workID, 1)
	require.NoError(t, err)
	err = db.CreateTask(ctx, workID+".2", "implement", []string{"bean-2"}, 10, workID, 2)
	require.NoError(t, err)
	err = db.CreateTask(ctx, workID+".3", "implement", []string{"bean-3"}, 10, workID, 3)
	require.NoError(t, err)

	// Complete only one task
	err = db.CompleteTask(ctx, workID+".1", "")
	require.NoError(t, err)

	// Work should not be completed
	isCompleted, err := db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.False(t, isCompleted)

	// Complete second task
	err = db.CompleteTask(ctx, workID+".2", "")
	require.NoError(t, err)

	// Still not completed (one task remaining)
	isCompleted, err = db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.False(t, isCompleted)

	// Complete final task
	err = db.CompleteTask(ctx, workID+".3", "")
	require.NoError(t, err)

	// Now work should be completed
	isCompleted, err = db.IsWorkCompleted(workID)
	require.NoError(t, err)
	assert.True(t, isCompleted)
}
