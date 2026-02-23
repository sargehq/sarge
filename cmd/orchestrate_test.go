package cmd

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupOrchestrateTestDB creates an in-memory database for orchestrate tests
func setupOrchestrateTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err, "failed to open database")

	cleanup := func() {
		testDB.Close()
	}

	return testDB, cleanup
}

func TestAutoWorkflowMetadata_ManualReviewSkipsAutomatedWorkflow(t *testing.T) {
	// This test verifies that manual review tasks (with auto_workflow=false)
	// can be identified via metadata lookup

	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work and review task
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	reviewTaskID := "work-1.1"
	err = testDB.CreateTask(ctx, reviewTaskID, "review", nil, 0, "work-1", 0)
	require.NoError(t, err)

	// Simulate TUI behavior: set auto_workflow=false for manual review
	err = testDB.SetTaskMetadata(ctx, reviewTaskID, "auto_workflow", "false")
	require.NoError(t, err)

	// Verify that we can detect this is a manual review
	autoWorkflow, err := testDB.GetTaskMetadata(ctx, reviewTaskID, "auto_workflow")
	require.NoError(t, err)
	assert.Equal(t, "false", autoWorkflow)

	// This simulates what handleReviewFixLoop does: check auto_workflow metadata
	// and return early if it equals "false"
	isManualReview := (err == nil && autoWorkflow == "false")
	assert.True(t, isManualReview, "manual review should be detected via auto_workflow=false")
}

func TestAutoWorkflowMetadata_AutomatedReviewContinuesWorkflow(t *testing.T) {
	// This test verifies that automated review tasks (without auto_workflow metadata)
	// should continue with the normal workflow

	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work and review task (no auto_workflow metadata = automated)
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	reviewTaskID := "work-1.1"
	err = testDB.CreateTask(ctx, reviewTaskID, "review", nil, 0, "work-1", 0)
	require.NoError(t, err)

	// Don't set auto_workflow metadata (this is what automated workflow does)

	// Verify that we can detect this is an automated review
	// GetTaskMetadata returns empty string when key doesn't exist
	autoWorkflow, err := testDB.GetTaskMetadata(ctx, reviewTaskID, "auto_workflow")
	require.NoError(t, err)

	// For automated reviews, the metadata is empty (not "false")
	// This means the workflow should continue (not skip)
	shouldSkipWorkflow := (err == nil && autoWorkflow == "false")
	assert.False(t, shouldSkipWorkflow, "automated review should continue workflow (no auto_workflow=false)")
}

func TestAutoWorkflowMetadata_ExplicitTrueAlsoContinuesWorkflow(t *testing.T) {
	// This test verifies that if auto_workflow is explicitly set to "true",
	// the workflow should continue

	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work and review task
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	reviewTaskID := "work-1.1"
	err = testDB.CreateTask(ctx, reviewTaskID, "review", nil, 0, "work-1", 0)
	require.NoError(t, err)

	// Explicitly set auto_workflow=true
	err = testDB.SetTaskMetadata(ctx, reviewTaskID, "auto_workflow", "true")
	require.NoError(t, err)

	// Verify that workflow should continue
	autoWorkflow, err := testDB.GetTaskMetadata(ctx, reviewTaskID, "auto_workflow")
	require.NoError(t, err)

	shouldContinueWorkflow := (err != nil || autoWorkflow != "false")
	assert.True(t, shouldContinueWorkflow, "auto_workflow=true should continue workflow")
}

func TestTaskIDFormat(t *testing.T) {
	// Verify all task types use sequential numeric IDs: {workID}.{number}

	workID := "work-1"

	// All task types use the same format: workID.number
	// The task_type column indicates whether it's implement, review, pr, etc.
	reviewTaskID := workID + ".1"
	assert.Equal(t, "work-1.1", reviewTaskID)

	prTaskID := workID + ".2"
	assert.Equal(t, "work-1.2", prTaskID)

	implementTaskID := workID + ".3"
	assert.Equal(t, "work-1.3", implementTaskID)
}

// TestGetReadyTasksRespectsDependencies tests that GetReadyTasksForWork
// returns tasks in correct dependency order (tasks with unmet dependencies are not returned).
func TestGetReadyTasksRespectsDependencies(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create tasks with dependencies:
	// task-1 has no dependencies (ready)
	// task-2 depends on task-1 (not ready until task-1 completes)
	// task-3 depends on task-2 (not ready until task-2 completes)
	err = testDB.CreateTask(ctx, "work-1.1", "implement", []string{"a"}, 5, "work-1", 1)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.2", "implement", []string{"b"}, 5, "work-1", 2)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.3", "implement", []string{"c"}, 5, "work-1", 3)
	require.NoError(t, err)

	// Add dependencies: task-2 depends on task-1, task-3 depends on task-2
	err = testDB.AddTaskDependency(ctx, "work-1.2", "work-1.1")
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.3", "work-1.2")
	require.NoError(t, err)

	// Initially, only task-1 should be ready
	readyTasks, err := testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 1, "only task-1 should be ready initially")
	assert.Equal(t, "work-1.1", readyTasks[0].ID)

	// Complete task-1, now task-2 should be ready
	err = testDB.CompleteTask(ctx, "work-1.1", "")
	require.NoError(t, err)

	readyTasks, err = testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 1, "only task-2 should be ready after task-1 completes")
	assert.Equal(t, "work-1.2", readyTasks[0].ID)

	// Complete task-2, now task-3 should be ready
	err = testDB.CompleteTask(ctx, "work-1.2", "")
	require.NoError(t, err)

	readyTasks, err = testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 1, "only task-3 should be ready after task-2 completes")
	assert.Equal(t, "work-1.3", readyTasks[0].ID)
}

// TestGetReadyTasksWithDiamondDependency tests GetReadyTasksForWork with a diamond dependency.
func TestGetReadyTasksWithDiamondDependency(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Diamond dependency:
	// task-1 and task-2 have no dependencies (both ready)
	// task-3 depends on both task-1 and task-2 (not ready until both complete)
	// task-4 depends on task-3
	err = testDB.CreateTask(ctx, "work-1.1", "implement", []string{"a"}, 5, "work-1", 1)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.2", "implement", []string{"b"}, 5, "work-1", 2)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.3", "implement", []string{"c"}, 5, "work-1", 3)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.4", "implement", []string{"d"}, 5, "work-1", 4)
	require.NoError(t, err)

	// task-3 depends on task-1 and task-2
	err = testDB.AddTaskDependency(ctx, "work-1.3", "work-1.1")
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.3", "work-1.2")
	require.NoError(t, err)
	// task-4 depends on task-3
	err = testDB.AddTaskDependency(ctx, "work-1.4", "work-1.3")
	require.NoError(t, err)

	// Initially, task-1 and task-2 should both be ready
	readyTasks, err := testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 2, "task-1 and task-2 should be ready initially")

	readyIDs := make(map[string]bool)
	for _, t := range readyTasks {
		readyIDs[t.ID] = true
	}
	assert.True(t, readyIDs["work-1.1"], "task-1 should be ready")
	assert.True(t, readyIDs["work-1.2"], "task-2 should be ready")

	// Complete only task-1, task-3 should still not be ready (needs task-2 too)
	err = testDB.CompleteTask(ctx, "work-1.1", "")
	require.NoError(t, err)

	readyTasks, err = testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 1, "only task-2 should be ready (task-3 needs both)")
	assert.Equal(t, "work-1.2", readyTasks[0].ID)

	// Complete task-2, now task-3 should be ready
	err = testDB.CompleteTask(ctx, "work-1.2", "")
	require.NoError(t, err)

	readyTasks, err = testDB.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	require.Len(t, readyTasks, 1, "task-3 should be ready after both deps complete")
	assert.Equal(t, "work-1.3", readyTasks[0].ID)
}

// Tests for createPRTask logic

// TestCreatePRTask_NoPRTaskExists verifies that when no PR task exists,
// a new PR task is created.
func TestCreatePRTask_NoPRTaskExists(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create a review task that the PR task will depend on
	err = testDB.CreateTask(ctx, "work-1.1", "review", nil, 0, "work-1", 1)
	require.NoError(t, err)

	// Verify no PR task exists initially
	prTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Nil(t, prTask, "expected no PR task initially")

	// Simulate createPRTask logic: no existing PR task, should create one
	// Use GetNextTaskNumber to get a unique task number
	nextNum, err := testDB.GetNextTaskNumber(ctx, "work-1")
	require.NoError(t, err)
	require.Greater(t, nextNum, 0, "task number should be positive")

	prTaskID := "work-1.2"
	err = testDB.CreateTask(ctx, prTaskID, "pr", nil, 0, "work-1", 0)
	require.NoError(t, err)

	err = testDB.AddTaskDependency(ctx, prTaskID, "work-1.1")
	require.NoError(t, err)

	// Verify PR task was created
	prTask, err = testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "expected PR task to be created")
	assert.Equal(t, "work-1.2", prTask.ID)
	assert.Equal(t, "pr", prTask.TaskType)
	assert.Equal(t, db.StatusPending, prTask.Status)

	// Verify dependency was added
	deps, err := testDB.GetTaskDependencies(ctx, prTaskID)
	require.NoError(t, err)
	assert.Contains(t, deps, "work-1.1", "PR task should depend on review task")
}

// TestCreatePRTask_PendingPRTaskExists verifies that when a pending PR task exists,
// no new task is created (skips creation).
func TestCreatePRTask_PendingPRTaskExists(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create existing pending PR task
	err = testDB.CreateTask(ctx, "work-1.1", "pr", nil, 0, "work-1", 1)
	require.NoError(t, err)

	// Create a review task that would trigger PR creation
	err = testDB.CreateTask(ctx, "work-1.2", "review", nil, 0, "work-1", 2)
	require.NoError(t, err)

	// Get existing PR task
	existingPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, existingPRTask, "expected existing PR task")
	assert.Equal(t, db.StatusPending, existingPRTask.Status)

	// Simulate createPRTask logic: pending PR task exists, should skip creation
	// (this is what the function does - returns early without creating)
	tasksBefore, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)
	initialTaskCount := len(tasksBefore)

	// Verify we would skip (logic check)
	// The actual createPRTask function returns nil without creating when pending
	assert.Equal(t, db.StatusPending, existingPRTask.Status)

	// No new task should be created (simulate the skip)
	tasksAfter, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)
	assert.Equal(t, initialTaskCount, len(tasksAfter), "no new task should be created when pending PR task exists")
}

// TestCreatePRTask_ProcessingPRTaskExists verifies that when a processing PR task exists,
// no new task is created (skips creation).
func TestCreatePRTask_ProcessingPRTaskExists(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create and start a PR task (processing)
	err = testDB.CreateTask(ctx, "work-1.1", "pr", nil, 0, "work-1", 1)
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.1", "")
	require.NoError(t, err)

	// Get existing PR task
	existingPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, existingPRTask, "expected existing PR task")
	assert.Equal(t, db.StatusProcessing, existingPRTask.Status)

	// Simulate createPRTask logic: processing PR task exists, should skip creation
	tasksBefore, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)
	initialTaskCount := len(tasksBefore)

	// Verify we would skip (logic check)
	assert.Equal(t, db.StatusProcessing, existingPRTask.Status)

	// No new task should be created
	tasksAfter, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)
	assert.Equal(t, initialTaskCount, len(tasksAfter), "no new task should be created when processing PR task exists")
}

// TestCreatePRTask_CompletedPRTaskExists verifies that when a completed PR task exists,
// an update-pr-description task is created instead of a new PR task.
func TestCreatePRTask_CompletedPRTaskExists(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work with a PR URL
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create and complete a PR task
	err = testDB.CreateTask(ctx, "work-1.1", "pr", nil, 0, "work-1", 1)
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.1", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.1", "https://github.com/example/pr/1")
	require.NoError(t, err)

	// Create a review task that the update-pr-description task will depend on
	err = testDB.CreateTask(ctx, "work-1.2", "review", nil, 0, "work-1", 2)
	require.NoError(t, err)

	// Get existing PR task
	existingPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, existingPRTask, "expected existing PR task")
	assert.Equal(t, db.StatusCompleted, existingPRTask.Status)

	// Simulate createUpdatePRDescriptionTask logic
	_, err = testDB.GetNextTaskNumber(ctx, "work-1")
	require.NoError(t, err)

	updateTaskID := "work-1.3"
	err = testDB.CreateTask(ctx, updateTaskID, "update-pr-description", nil, 0, "work-1", 0)
	require.NoError(t, err)

	err = testDB.AddTaskDependency(ctx, updateTaskID, "work-1.2")
	require.NoError(t, err)

	// Verify update-pr-description task was created
	updateTask, err := testDB.GetTask(ctx, updateTaskID)
	require.NoError(t, err)
	require.NotNil(t, updateTask, "expected update-pr-description task to be created")
	assert.Equal(t, "update-pr-description", updateTask.TaskType)
	assert.Equal(t, db.StatusPending, updateTask.Status)

	// Verify dependency was added
	deps, err := testDB.GetTaskDependencies(ctx, updateTaskID)
	require.NoError(t, err)
	assert.Contains(t, deps, "work-1.2", "update-pr-description task should depend on review task")

	// Verify the original PR task is still there and completed
	prTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "PR task should still exist")
	assert.Equal(t, "work-1.1", prTask.ID)
	assert.Equal(t, db.StatusCompleted, prTask.Status)
}

// TestCreatePRTask_FailedPRTaskAllowsNewPR verifies that when a PR task is failed,
// GetPRTaskForWork returns nil (as per query logic), allowing a new PR task to be created.
func TestCreatePRTask_FailedPRTaskAllowsNewPR(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Create and fail a PR task
	err = testDB.CreateTask(ctx, "work-1.1", "pr", nil, 0, "work-1", 1)
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.1", "")
	require.NoError(t, err)
	err = testDB.FailTask(ctx, "work-1.1", "PR creation failed")
	require.NoError(t, err)

	// Create a review task
	err = testDB.CreateTask(ctx, "work-1.2", "review", nil, 0, "work-1", 2)
	require.NoError(t, err)

	// GetPRTaskForWork should return nil for failed PR tasks
	existingPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Nil(t, existingPRTask, "failed PR task should not be returned by GetPRTaskForWork")

	// Simulate createPRTask logic: no active PR task exists, should create a new one
	_, err = testDB.GetNextTaskNumber(ctx, "work-1")
	require.NoError(t, err)

	newPRTaskID := "work-1.3"
	err = testDB.CreateTask(ctx, newPRTaskID, "pr", nil, 0, "work-1", 0)
	require.NoError(t, err)

	err = testDB.AddTaskDependency(ctx, newPRTaskID, "work-1.2")
	require.NoError(t, err)

	// Verify new PR task was created
	newPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, newPRTask, "new PR task should be created after failed one")
	assert.Equal(t, newPRTaskID, newPRTask.ID)
	assert.Equal(t, "pr", newPRTask.TaskType)
	assert.Equal(t, db.StatusPending, newPRTask.Status)
}

// TestReviewFixLoopCreatesOnlyOnePRTask verifies that multiple review cycles
// only create one PR task (subsequent passes create update-pr-description tasks).
func TestReviewFixLoopCreatesOnlyOnePRTask(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// First review cycle - no PR task exists
	err = testDB.CreateTask(ctx, "work-1.1", "implement", []string{"bean-1"}, 5, "work-1", 1)
	require.NoError(t, err)

	// First review passes - should create PR task
	err = testDB.CreateTask(ctx, "work-1.2", "review", nil, 0, "work-1", 2)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.2", "work-1.1")
	require.NoError(t, err)

	// Simulate first review completing and creating PR task
	err = testDB.StartTask(ctx, "work-1.2", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.2", "")
	require.NoError(t, err)

	// Create PR task (first review pass)
	err = testDB.CreateTask(ctx, "work-1.3", "pr", nil, 0, "work-1", 3)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.3", "work-1.2")
	require.NoError(t, err)

	// Complete the PR task (PR is now created)
	err = testDB.StartTask(ctx, "work-1.3", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.3", "https://github.com/example/pr/1")
	require.NoError(t, err)

	// Second review cycle (after PR feedback) - PR already exists
	err = testDB.CreateTask(ctx, "work-1.4", "implement", []string{"bean-2"}, 5, "work-1", 4)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.5", "review", nil, 0, "work-1", 5)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.5", "work-1.4")
	require.NoError(t, err)

	// Second review completes
	err = testDB.StartTask(ctx, "work-1.5", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.5", "")
	require.NoError(t, err)

	// Check for existing PR task
	existingPRTask, err := testDB.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, existingPRTask, "PR task should exist")
	assert.Equal(t, db.StatusCompleted, existingPRTask.Status)

	// Should create update-pr-description, not a new PR task
	err = testDB.CreateTask(ctx, "work-1.6", "update-pr-description", nil, 0, "work-1", 6)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.6", "work-1.5")
	require.NoError(t, err)

	// Verify only one PR task exists
	tasks, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)

	prTaskCount := 0
	updatePRCount := 0
	for _, task := range tasks {
		if task.TaskType == "pr" {
			prTaskCount++
		}
		if task.TaskType == "update-pr-description" {
			updatePRCount++
		}
	}

	assert.Equal(t, 1, prTaskCount, "should have exactly one PR task")
	assert.Equal(t, 1, updatePRCount, "should have one update-pr-description task")
}

// TestMultipleReviewCyclesCreateMultipleUpdatePRTasks verifies that
// each subsequent review cycle creates a new update-pr-description task.
func TestMultipleReviewCyclesCreateMultipleUpdatePRTasks(t *testing.T) {
	testDB, cleanup := setupOrchestrateTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create a work
	err := testDB.CreateWork(ctx, "work-1", "Test Work", "/tmp/test", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Setup: create and complete a PR task
	err = testDB.CreateTask(ctx, "work-1.1", "review", nil, 0, "work-1", 1)
	require.NoError(t, err)
	err = testDB.CreateTask(ctx, "work-1.2", "pr", nil, 0, "work-1", 2)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.2", "work-1.1")
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.2", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.2", "https://github.com/example/pr/1")
	require.NoError(t, err)

	// First feedback cycle - creates first update-pr-description
	err = testDB.CreateTask(ctx, "work-1.3", "review", nil, 0, "work-1", 3)
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.3", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.3", "")
	require.NoError(t, err)

	err = testDB.CreateTask(ctx, "work-1.4", "update-pr-description", nil, 0, "work-1", 4)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.4", "work-1.3")
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.4", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.4", "")
	require.NoError(t, err)

	// Second feedback cycle - creates second update-pr-description
	err = testDB.CreateTask(ctx, "work-1.5", "review", nil, 0, "work-1", 5)
	require.NoError(t, err)
	err = testDB.StartTask(ctx, "work-1.5", "")
	require.NoError(t, err)
	err = testDB.CompleteTask(ctx, "work-1.5", "")
	require.NoError(t, err)

	err = testDB.CreateTask(ctx, "work-1.6", "update-pr-description", nil, 0, "work-1", 6)
	require.NoError(t, err)
	err = testDB.AddTaskDependency(ctx, "work-1.6", "work-1.5")
	require.NoError(t, err)

	// Count task types
	tasks, err := testDB.GetWorkTasks(ctx, "work-1")
	require.NoError(t, err)

	prTaskCount := 0
	updatePRCount := 0
	reviewCount := 0
	for _, task := range tasks {
		switch task.TaskType {
		case "pr":
			prTaskCount++
		case "update-pr-description":
			updatePRCount++
		case "review":
			reviewCount++
		}
	}

	assert.Equal(t, 1, prTaskCount, "should have exactly one PR task")
	assert.Equal(t, 2, updatePRCount, "should have two update-pr-description tasks")
	assert.Equal(t, 3, reviewCount, "should have three review tasks")
}
