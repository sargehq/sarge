package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestWork creates a test work and returns the work ID
func createTestWork(t *testing.T, db *DB) string {
	t.Helper()
	err := db.CreateWork(context.Background(), "test-work", "", "/tmp/worktree", "feat/test", "main", "root-issue", false)
	require.NoError(t, err, "failed to create test work")
	return "test-work"
}

func TestCreateTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	err := db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1", "bead-2"}, 100, workID)
	require.NoError(t, err, "CreateTask failed")

	// Verify task was created
	task, err := db.GetTask(context.Background(), "task-1")
	require.NoError(t, err, "GetTask failed")
	require.NotNil(t, task, "expected task, got nil")
	assert.Equal(t, "task-1", task.ID)
	assert.Equal(t, StatusPending, task.Status)
	assert.Equal(t, 100, task.ComplexityBudget)

	// Verify beads were added
	beads, err := db.GetTaskBeads(context.Background(), "task-1")
	require.NoError(t, err, "GetTaskBeads failed")
	assert.Len(t, beads, 2, "expected 2 beads")
}

func TestStartTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	err := db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)
	require.NoError(t, err, "CreateTask failed")

	err = db.StartTask(context.Background(), "task-1", "")
	require.NoError(t, err, "StartTask failed")

	task, err := db.GetTask(context.Background(), "task-1")
	require.NoError(t, err, "GetTask failed")
	assert.Equal(t, StatusProcessing, task.Status)
	// WorktreePath is now managed at work level, should be empty
	assert.Empty(t, task.WorktreePath, "expected empty worktree path (managed at work level)")
	assert.NotNil(t, task.StartedAt, "expected StartedAt to be set")
}

func TestStartTaskNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.StartTask(context.Background(), "nonexistent", "")
	assert.Error(t, err, "expected error for nonexistent task")
}

func TestCompleteTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)
	db.StartTask(context.Background(), "task-1", "")

	err := db.CompleteTask(context.Background(), "task-1", "https://github.com/example/pr/1")
	require.NoError(t, err, "CompleteTask failed")

	task, _ := db.GetTask(context.Background(), "task-1")
	assert.Equal(t, StatusCompleted, task.Status)
	assert.Equal(t, "https://github.com/example/pr/1", task.PRURL)
	assert.NotNil(t, task.CompletedAt, "expected CompletedAt to be set")
}

func TestCompleteTaskNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.CompleteTask(context.Background(), "nonexistent", "")
	assert.Error(t, err, "expected error for nonexistent task")
}

func TestFailTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)
	db.StartTask(context.Background(), "task-1", "")

	err := db.FailTask(context.Background(), "task-1", "something went wrong")
	require.NoError(t, err, "FailTask failed")

	task, _ := db.GetTask(context.Background(), "task-1")
	assert.Equal(t, StatusFailed, task.Status)
	assert.Equal(t, "something went wrong", task.ErrorMessage)
	assert.NotNil(t, task.CompletedAt, "expected CompletedAt to be set")
}

func TestFailTaskNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	err := db.FailTask(context.Background(), "nonexistent", "error")
	assert.Error(t, err, "expected error for nonexistent task")
}

func TestGetTaskNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	task, err := db.GetTask(context.Background(), "nonexistent")
	require.NoError(t, err, "GetTask failed")
	assert.Nil(t, task, "expected nil for nonexistent task")
}

func TestGetTaskForBead(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1", "bead-2"}, 100, workID)

	taskID, err := db.GetTaskForBead(context.Background(), "bead-1")
	require.NoError(t, err, "GetTaskForBead failed")
	assert.Equal(t, "task-1", taskID)

	taskID, err = db.GetTaskForBead(context.Background(), "bead-2")
	require.NoError(t, err, "GetTaskForBead failed")
	assert.Equal(t, "task-1", taskID)

	// Nonexistent bead
	taskID, err = db.GetTaskForBead(context.Background(), "nonexistent")
	require.NoError(t, err, "GetTaskForBead failed")
	assert.Empty(t, taskID, "expected empty string")
}

func TestCompleteTaskBead(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1", "bead-2"}, 100, workID)

	err := db.CompleteTaskBead(context.Background(), "task-1", "bead-1")
	require.NoError(t, err, "CompleteTaskBead failed")

	// Verify via CountTaskBeadStatuses (should be false since bead-2 is still pending)
	total, completed, err := db.CountTaskBeadStatuses(context.Background(), "task-1")
	require.NoError(t, err, "CountTaskBeadStatuses failed")
	assert.False(t, total > 0 && total == completed, "expected task to not be completed yet")
}

func TestCompleteTaskBeadNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)

	err := db.CompleteTaskBead(context.Background(), "task-1", "nonexistent")
	assert.Error(t, err, "expected error for nonexistent task bead")
}

func TestFailTaskBead(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)

	err := db.FailTaskBead(context.Background(), "task-1", "bead-1")
	require.NoError(t, err, "FailTaskBead failed")

	// Task should not be considered completed since bead is failed
	total, completed, _ := db.CountTaskBeadStatuses(context.Background(), "task-1")
	assert.False(t, total > 0 && total == completed, "expected task to not be completed when bead is failed")
}

func TestFailTaskBeadNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)

	err := db.FailTaskBead(context.Background(), "task-1", "nonexistent")
	assert.Error(t, err, "expected error for nonexistent task bead")
}

func TestIsTaskCompleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1", "bead-2"}, 100, workID)

	// Initially not completed
	total, completed, err := db.CountTaskBeadStatuses(context.Background(), "task-1")
	require.NoError(t, err, "CountTaskBeadStatuses failed")
	assert.False(t, total > 0 && total == completed, "expected task to not be completed initially")

	// Complete first bead
	db.CompleteTaskBead(context.Background(), "task-1", "bead-1")
	total, completed, _ = db.CountTaskBeadStatuses(context.Background(), "task-1")
	assert.False(t, total > 0 && total == completed, "expected task to not be completed with one bead pending")

	// Complete second bead
	db.CompleteTaskBead(context.Background(), "task-1", "bead-2")
	total, completed, _ = db.CountTaskBeadStatuses(context.Background(), "task-1")
	assert.True(t, total > 0 && total == completed, "expected task to be completed when all beads are completed")
}

func TestIsTaskCompletedEmpty(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	// Task with no beads
	_, err := db.Exec(`INSERT INTO tasks (id, status, work_id) VALUES ('empty-task', 'pending', ?)`, workID)
	require.NoError(t, err, "failed to create empty task")

	total, completed, err := db.CountTaskBeadStatuses(context.Background(), "empty-task")
	require.NoError(t, err, "CountTaskBeadStatuses failed")
	assert.False(t, total > 0 && total == completed, "expected empty task to not be considered completed")
}

func TestCheckAndCompleteTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1", "bead-2"}, 100, workID)
	db.StartTask(context.Background(), "task-1", "")

	// Not all beads completed yet
	autoCompleted, err := db.CheckAndCompleteTask(context.Background(), "task-1", "https://github.com/pr/1")
	require.NoError(t, err, "CheckAndCompleteTask failed")
	assert.False(t, autoCompleted, "expected not auto-completed when beads are pending")

	task, _ := db.GetTask(context.Background(), "task-1")
	assert.Equal(t, StatusProcessing, task.Status)

	// Complete all beads
	db.CompleteTaskBead(context.Background(), "task-1", "bead-1")
	db.CompleteTaskBead(context.Background(), "task-1", "bead-2")

	autoCompleted, err = db.CheckAndCompleteTask(context.Background(), "task-1", "https://github.com/pr/1")
	require.NoError(t, err, "CheckAndCompleteTask failed")
	assert.True(t, autoCompleted, "expected auto-completed when all beads are completed")

	task, _ = db.GetTask(context.Background(), "task-1")
	assert.Equal(t, StatusCompleted, task.Status)
	assert.Equal(t, "https://github.com/pr/1", task.PRURL)
}

func TestListTasks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	workID := createTestWork(t, db)

	// Create several tasks with different statuses
	db.CreateTask(context.Background(), "task-1", "implement", []string{"bead-1"}, 100, workID)
	db.CreateTask(context.Background(), "task-2", "implement", []string{"bead-2"}, 100, workID)
	db.StartTask(context.Background(), "task-2", "")
	db.CreateTask(context.Background(), "task-3", "implement", []string{"bead-3"}, 100, workID)
	db.StartTask(context.Background(), "task-3", "")
	db.CompleteTask(context.Background(), "task-3", "")
	db.CreateTask(context.Background(), "task-4", "implement", []string{"bead-4"}, 100, workID)
	db.StartTask(context.Background(), "task-4", "")
	db.FailTask(context.Background(), "task-4", "error")

	// List all - should be ordered by created_at DESC (newest first)
	tasks, err := db.ListTasks(context.Background(), "")
	require.NoError(t, err, "ListTasks failed")
	assert.Len(t, tasks, 4, "expected 4 tasks")
	// Verify ordering: newest first (task-4, task-3, task-2, task-1)
	for i := 0; i < len(tasks)-1; i++ {
		assert.True(t, !tasks[i].CreatedAt.Before(tasks[i+1].CreatedAt),
			"tasks should be ordered newest first, but task %s (created %v) is before task %s (created %v)",
			tasks[i].ID, tasks[i].CreatedAt, tasks[i+1].ID, tasks[i+1].CreatedAt)
	}

	// List pending only
	tasks, err = db.ListTasks(context.Background(), StatusPending)
	require.NoError(t, err, "ListTasks failed")
	assert.Len(t, tasks, 1, "expected 1 pending task")

	// List processing only
	tasks, err = db.ListTasks(context.Background(), StatusProcessing)
	require.NoError(t, err, "ListTasks failed")
	assert.Len(t, tasks, 1, "expected 1 processing task")

	// List completed only
	tasks, err = db.ListTasks(context.Background(), StatusCompleted)
	require.NoError(t, err, "ListTasks failed")
	assert.Len(t, tasks, 1, "expected 1 completed task")

	// List failed only
	tasks, err = db.ListTasks(context.Background(), StatusFailed)
	require.NoError(t, err, "ListTasks failed")
	assert.Len(t, tasks, 1, "expected 1 failed task")
}

// Task dependency tests

func TestAddTaskDependency(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work and tasks
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err, "CreateWork failed")

	err = db.CreateTask(ctx, "task-1", "implement", []string{"bead-1"}, 0, "work-1")
	require.NoError(t, err, "CreateTask task-1 failed")

	err = db.CreateTask(ctx, "task-2", "implement", []string{"bead-2"}, 0, "work-1")
	require.NoError(t, err, "CreateTask task-2 failed")

	// Add dependency: task-2 depends on task-1
	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err, "AddTaskDependency failed")

	// Verify dependencies
	deps, err := db.GetTaskDependencies(ctx, "task-2")
	require.NoError(t, err, "GetTaskDependencies failed")
	assert.Len(t, deps, 1)
	assert.Equal(t, "task-1", deps[0])

	// Verify dependents
	dependents, err := db.GetTaskDependents(ctx, "task-1")
	require.NoError(t, err, "GetTaskDependents failed")
	assert.Len(t, dependents, 1)
	assert.Equal(t, "task-2", dependents[0])
}

func TestGetReadyTasksForWork(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err, "CreateWork failed")

	// Create tasks: task-1 has no deps, task-2 depends on task-1, task-3 depends on task-2
	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err, "CreateTask task-1 failed")

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err, "CreateTask task-2 failed")
	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err, "AddTaskDependency task-2 -> task-1 failed")

	err = db.CreateTask(ctx, "task-3", "implement", nil, 0, "work-1")
	require.NoError(t, err, "CreateTask task-3 failed")
	err = db.AddTaskDependency(ctx, "task-3", "task-2")
	require.NoError(t, err, "AddTaskDependency task-3 -> task-2 failed")

	// Initially, only task-1 should be ready (no dependencies)
	ready, err := db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err, "GetReadyTasksForWork failed")
	assert.Len(t, ready, 1, "expected 1 ready task initially")
	assert.Equal(t, "task-1", ready[0].ID)

	// Complete task-1, now task-2 should be ready
	db.StartTask(ctx, "task-1", "")
	db.CompleteTask(ctx, "task-1", "")

	ready, err = db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err, "GetReadyTasksForWork failed")
	assert.Len(t, ready, 1, "expected 1 ready task after task-1 completes")
	assert.Equal(t, "task-2", ready[0].ID)

	// Complete task-2, now task-3 should be ready
	db.StartTask(ctx, "task-2", "")
	db.CompleteTask(ctx, "task-2", "")

	ready, err = db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err, "GetReadyTasksForWork failed")
	assert.Len(t, ready, 1, "expected 1 ready task after task-2 completes")
	assert.Equal(t, "task-3", ready[0].ID)

	// Complete task-3, no more ready tasks
	db.StartTask(ctx, "task-3", "")
	db.CompleteTask(ctx, "task-3", "")

	ready, err = db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err, "GetReadyTasksForWork failed")
	assert.Len(t, ready, 0, "expected no ready tasks after all complete")
}

func TestGetReadyTasksForWorkMultipleDependencies(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err, "CreateWork failed")

	// task-3 depends on both task-1 AND task-2
	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-3", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.AddTaskDependency(ctx, "task-3", "task-1")
	require.NoError(t, err)
	err = db.AddTaskDependency(ctx, "task-3", "task-2")
	require.NoError(t, err)

	// Both task-1 and task-2 should be ready (no deps)
	ready, err := db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Len(t, ready, 2, "expected 2 ready tasks initially")

	// Complete only task-1, task-3 should NOT be ready yet
	db.StartTask(ctx, "task-1", "")
	db.CompleteTask(ctx, "task-1", "")

	ready, err = db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Len(t, ready, 1, "expected only task-2 ready")
	assert.Equal(t, "task-2", ready[0].ID)

	// Complete task-2, now task-3 should be ready
	db.StartTask(ctx, "task-2", "")
	db.CompleteTask(ctx, "task-2", "")

	ready, err = db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Len(t, ready, 1, "expected task-3 ready")
	assert.Equal(t, "task-3", ready[0].ID)
}

func TestHasPendingDependencies(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work and tasks
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err)

	// task-1 has no dependencies
	hasPending, err := db.HasPendingDependencies(ctx, "task-1")
	require.NoError(t, err)
	assert.False(t, hasPending, "task-1 should have no pending dependencies")

	// task-2 has pending dependency (task-1 is not completed)
	hasPending, err = db.HasPendingDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.True(t, hasPending, "task-2 should have pending dependencies")

	// Complete task-1
	db.StartTask(ctx, "task-1", "")
	db.CompleteTask(ctx, "task-1", "")

	// Now task-2 should have no pending dependencies
	hasPending, err = db.HasPendingDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.False(t, hasPending, "task-2 should have no pending dependencies after task-1 completes")
}

func TestDeleteTaskDependency(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work and tasks
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err)

	// Verify dependency exists
	deps, err := db.GetTaskDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.Len(t, deps, 1)

	// Delete the dependency
	err = db.DeleteTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err)

	// Verify dependency is gone
	deps, err = db.GetTaskDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.Len(t, deps, 0, "expected no dependencies after deletion")
}

func TestBlockedTasksNotReady(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Create a chain: task-1 -> task-2 -> task-3
	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err)
	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-3", "implement", nil, 0, "work-1")
	require.NoError(t, err)
	err = db.AddTaskDependency(ctx, "task-3", "task-2")
	require.NoError(t, err)

	// Start task-1 (now processing, not completed)
	db.StartTask(ctx, "task-1", "")

	// task-2 should still be blocked (task-1 is processing, not completed)
	ready, err := db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Len(t, ready, 0, "no tasks should be ready when task-1 is only processing")

	hasPending, err := db.HasPendingDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.True(t, hasPending, "task-2 should still have pending dependencies")
}

func TestFailedTaskBlocksDependents(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Create: task-2 depends on task-1
	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	err = db.CreateTask(ctx, "task-2", "implement", nil, 0, "work-1")
	require.NoError(t, err)
	err = db.AddTaskDependency(ctx, "task-2", "task-1")
	require.NoError(t, err)

	// Fail task-1
	db.StartTask(ctx, "task-1", "")
	db.FailTask(ctx, "task-1", "error")

	// task-2 should still have pending dependencies (failed != completed)
	hasPending, err := db.HasPendingDependencies(ctx, "task-2")
	require.NoError(t, err)
	assert.True(t, hasPending, "task-2 should have pending dependencies when task-1 failed")

	// task-2 should NOT be ready
	ready, err := db.GetReadyTasksForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Len(t, ready, 0, "no tasks should be ready when dependency failed")
}

func TestGetPRTaskForWork(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// No PR task exists yet
	prTask, err := db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Nil(t, prTask, "expected nil when no PR task exists")

	// Create a non-PR task
	err = db.CreateTask(ctx, "task-1", "implement", nil, 0, "work-1")
	require.NoError(t, err)

	// Still no PR task
	prTask, err = db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Nil(t, prTask, "expected nil when only non-PR tasks exist")

	// Create a PR task (pending)
	err = db.CreateTask(ctx, "task-2", "pr", nil, 0, "work-1")
	require.NoError(t, err)

	prTask, err = db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "expected PR task to be returned")
	assert.Equal(t, "task-2", prTask.ID)
	assert.Equal(t, "pr", prTask.TaskType)
	assert.Equal(t, StatusPending, prTask.Status)

	// Start the PR task (processing)
	err = db.StartTask(ctx, "task-2", "")
	require.NoError(t, err)

	prTask, err = db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "expected PR task to be returned when processing")
	assert.Equal(t, StatusProcessing, prTask.Status)

	// Complete the PR task
	err = db.CompleteTask(ctx, "task-2", "https://github.com/example/pr/1")
	require.NoError(t, err)

	prTask, err = db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "expected PR task to be returned when completed")
	assert.Equal(t, StatusCompleted, prTask.Status)
	assert.Equal(t, "https://github.com/example/pr/1", prTask.PRURL)
}

func TestGetPRTaskForWork_FailedNotReturned(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create work
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree", "feat/test", "main", "root-issue-1", false)
	require.NoError(t, err)

	// Create a PR task and fail it
	err = db.CreateTask(ctx, "task-1", "pr", nil, 0, "work-1")
	require.NoError(t, err)
	err = db.StartTask(ctx, "task-1", "")
	require.NoError(t, err)
	err = db.FailTask(ctx, "task-1", "PR creation failed")
	require.NoError(t, err)

	// Failed PR task should not be returned
	prTask, err := db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	assert.Nil(t, prTask, "expected nil when PR task is failed")
}

func TestGetPRTaskForWork_MultipleWorks(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create two works
	err := db.CreateWork(ctx, "work-1", "", "/tmp/worktree1", "feat/test1", "main", "root-issue-1", false)
	require.NoError(t, err)
	err = db.CreateWork(ctx, "work-2", "", "/tmp/worktree2", "feat/test2", "main", "root-issue-2", false)
	require.NoError(t, err)

	// Create PR task only for work-1
	err = db.CreateTask(ctx, "task-1", "pr", nil, 0, "work-1")
	require.NoError(t, err)

	// work-1 should have a PR task
	prTask, err := db.GetPRTaskForWork(ctx, "work-1")
	require.NoError(t, err)
	require.NotNil(t, prTask, "expected PR task for work-1")
	assert.Equal(t, "task-1", prTask.ID)

	// work-2 should not have a PR task
	prTask, err = db.GetPRTaskForWork(ctx, "work-2")
	require.NoError(t, err)
	assert.Nil(t, prTask, "expected nil for work-2 which has no PR task")
}
