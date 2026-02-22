package work_test

import (
	"context"
	"io"
	"testing"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWork_CreatesTasksFromBeads(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create test beads
	h.CreateBead("bead-1", "Implement feature A")
	h.CreateBead("bead-2", "Implement feature B")
	h.CreateBead("bead-3", "Implement feature C")

	// Create work with beads
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")
	h.AddBeadToWork("w-test", "bead-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Run work without planning (creates one task per bead)
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify tasks were created
	assert.Equal(t, "w-test", result.WorkID)
	assert.Equal(t, 3, result.TasksCreated, "expected 3 tasks (one per bead)")

	// Verify task records in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 3)

	// Verify each task has correct bead association
	for i, task := range tasks {
		beadIDs, err := h.DB.GetTaskBeads(ctx, task.ID)
		require.NoError(t, err)
		assert.Len(t, beadIDs, 1, "task %d should have 1 bead", i)
		assert.Equal(t, "implement", task.TaskType)
		assert.Equal(t, db.StatusPending, task.Status)
	}
}

func TestRunWork_RespectsBeadDependencies(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beads with dependencies
	// bead-2 depends on bead-1
	// bead-3 depends on bead-2
	h.CreateBead("bead-1", "Base feature")
	h.CreateBead("bead-2", "Depends on bead-1")
	h.CreateBead("bead-3", "Depends on bead-2")

	h.SetBeadDependency("bead-2", "bead-1")
	h.SetBeadDependency("bead-3", "bead-2")

	// Create work with beads
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")
	h.AddBeadToWork("w-test", "bead-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Configure task planner to return tasks in correct order
	h.TaskPlanner.PlanFunc = func(ctx context.Context, beadList []beads.Bead, dependencies map[string][]beads.Dependency, budget int) ([]task.Task, error) {
		// Return tasks respecting dependencies: bead-1, then bead-2, then bead-3
		return []task.Task{
			{ID: "task-1", BeadIDs: []string{"bead-1"}, Beads: []beads.Bead{{ID: "bead-1"}}},
			{ID: "task-2", BeadIDs: []string{"bead-2"}, Beads: []beads.Bead{{ID: "bead-2"}}},
			{ID: "task-3", BeadIDs: []string{"bead-3"}, Beads: []beads.Bead{{ID: "bead-3"}}},
		}, nil
	}

	// Run work with planning enabled
	result, err := h.WorkService.RunWork(ctx, "w-test", true, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 3, result.TasksCreated)

	// Verify planner was called with correct dependencies
	calls := h.TaskPlanner.PlanCalls()
	require.Len(t, calls, 1)
	assert.Len(t, calls[0].BeadList, 3)
	assert.Contains(t, calls[0].Dependencies, "bead-2", "dependencies should include bead-2")
	assert.Contains(t, calls[0].Dependencies, "bead-3", "dependencies should include bead-3")
}

func TestRunWork_WithPlanningEnabled(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beads
	h.CreateBead("bead-1", "Simple task")
	h.CreateBead("bead-2", "Another simple task")
	h.CreateBead("bead-3", "Complex task")

	// Create work with beads
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")
	h.AddBeadToWork("w-test", "bead-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Configure task planner to group beads by complexity
	// Simple beads grouped together, complex bead in separate task
	h.TaskPlanner.PlanFunc = func(ctx context.Context, beadList []beads.Bead, dependencies map[string][]beads.Dependency, budget int) ([]task.Task, error) {
		return []task.Task{
			{
				ID:              "task-1",
				BeadIDs:         []string{"bead-1", "bead-2"},
				Beads:           []beads.Bead{{ID: "bead-1"}, {ID: "bead-2"}},
				Complexity:      4,
				EstimatedTokens: 20000,
			},
			{
				ID:              "task-2",
				BeadIDs:         []string{"bead-3"},
				Beads:           []beads.Bead{{ID: "bead-3"}},
				Complexity:      8,
				EstimatedTokens: 80000,
			},
		}, nil
	}

	// Run work with planning enabled
	result, err := h.WorkService.RunWork(ctx, "w-test", true, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify tasks were grouped
	assert.Equal(t, 2, result.TasksCreated, "expected 2 tasks with planning")

	// Verify task grouping in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	// Tasks ordered by task_number DESC, so tasks[0] is the last-created task
	// Second task (higher number) should have 1 bead
	task2Beads, err := h.DB.GetTaskBeads(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Len(t, task2Beads, 1)
	assert.Contains(t, task2Beads, "bead-3")

	// First task (lower number) should have 2 beads
	task1Beads, err := h.DB.GetTaskBeads(ctx, tasks[1].ID)
	require.NoError(t, err)
	assert.Len(t, task1Beads, 2)
	assert.Contains(t, task1Beads, "bead-1")
	assert.Contains(t, task1Beads, "bead-2")
}

func TestRunWork_SkipsAlreadyAssignedBeads(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create initial bead
	h.CreateBead("bead-1", "Initially assigned")

	// Create work with initial bead
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// First run: creates task for bead-1
	result1, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 1, result1.TasksCreated)

	// Now add more beads to the work
	h.CreateBead("bead-2", "Newly added")
	h.CreateBead("bead-3", "Also newly added")
	h.AddBeadToWork("w-test", "bead-2")
	h.AddBeadToWork("w-test", "bead-3")

	// Second run: should only create tasks for newly added beads
	result2, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Should only create 2 tasks (for bead-2 and bead-3)
	assert.Equal(t, 2, result2.TasksCreated, "expected 2 tasks for newly added beads")

	// Verify total tasks in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 3, "should have 3 total tasks")
}

func TestRunWork_SpawnsOrchestrator(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create a bead and work
	h.CreateBead("bead-1", "Test bead")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Track orchestrator spawn calls
	ensureCalled := false
	h.OrchestratorManager.EnsureWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
		ensureCalled = true
		assert.Equal(t, "w-test", workID)
		assert.Equal(t, workRecord.WorktreePath, workDir)
		return true, nil // Indicate orchestrator was spawned
	}

	// Run work
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify orchestrator was spawned
	assert.True(t, ensureCalled, "EnsureWorkOrchestrator should have been called")
	assert.True(t, result.OrchestratorSpawned)
}

func TestRunWork_IdempotentOnRerun(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beads and work
	h.CreateBead("bead-1", "Test bead 1")
	h.CreateBead("bead-2", "Test bead 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// First run: create tasks
	result1, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 2, result1.TasksCreated)

	// Verify tasks exist
	tasks1, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks1, 2)

	// Second run: should not create more tasks (all beads already assigned)
	result2, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 0, result2.TasksCreated, "second run should not create new tasks")

	// Verify no additional tasks were created
	tasks2, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks2, 2, "should still have only 2 tasks")
}

func TestRunWork_FailsWithoutWorktree(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work without worktree path
	err := h.DB.CreateWork(ctx, "w-no-worktree", "No Worktree", "", "feat/test", "main", "", false)
	require.NoError(t, err)

	// Run should fail
	result, err := h.WorkService.RunWork(ctx, "w-no-worktree", false, io.Discard)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no worktree path")
}

func TestRunWork_FailsWhenWorktreeNotExists(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work with worktree path
	h.CreateWork("w-test", "feat/test-branch")

	// Configure worktree to NOT exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return false
	}

	// Run should fail
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "worktree does not exist")
}

func TestRunWork_WorkNotFound(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Run should fail for non-existent work
	result, err := h.WorkService.RunWork(ctx, "non-existent", false, io.Discard)
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not found")
}

func TestPlanWorkTasks_CreatesTasksWithoutOrchestrator(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beads and work
	h.CreateBead("bead-1", "Test bead 1")
	h.CreateBead("bead-2", "Test bead 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")

	// Configure worktree to exist (even though PlanWorkTasks doesn't check)
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Track orchestrator calls - should NOT be called
	ensureCalled := false
	h.OrchestratorManager.EnsureWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
		ensureCalled = true
		return true, nil
	}

	// Plan tasks (without auto-grouping)
	result, err := h.WorkService.PlanWorkTasks(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify tasks were created
	assert.Equal(t, 2, result.TasksCreated)

	// Verify orchestrator was NOT called
	assert.False(t, ensureCalled, "EnsureWorkOrchestrator should NOT have been called")

	// Verify tasks exist in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 2)
}

func TestRunWorkAuto_CreatesEstimateTask(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beads and work
	h.CreateBead("bead-1", "Task 1")
	h.CreateBead("bead-2", "Task 2")
	h.CreateBead("bead-3", "Task 3")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "bead-1")
	h.AddBeadToWork("w-test", "bead-2")
	h.AddBeadToWork("w-test", "bead-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Run work in auto mode
	result, err := h.WorkService.RunWorkAuto(ctx, "w-test", io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "w-test", result.WorkID)
	assert.True(t, result.EstimateTaskCreated)
	assert.True(t, result.OrchestratorSpawned)

	// Verify an estimate task was created
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	assert.Equal(t, "estimate", tasks[0].TaskType)
	assert.Equal(t, db.StatusPending, tasks[0].Status)

	// Verify all beads are in the estimate task
	beadIDs, err := h.DB.GetTaskBeads(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Len(t, beadIDs, 3)
	assert.Contains(t, beadIDs, "bead-1")
	assert.Contains(t, beadIDs, "bead-2")
	assert.Contains(t, beadIDs, "bead-3")
}

func TestCreateEstimateTaskFromWorkBeads_FailsWithNoUnassignedBeads(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work without beads
	h.CreateWork("w-test", "feat/test-branch")

	// Should fail with no unassigned beads
	err := h.WorkService.CreateEstimateTaskFromWorkBeads(ctx, "w-test", io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no unassigned beads")
}

func TestRunWork_WithEpicBeads(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create an epic with children
	h.CreateEpicWithChildren("epic-1", "child-1", "child-2", "child-3")

	// Create work with all beads
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeadToWork("w-test", "epic-1")
	h.AddBeadToWork("w-test", "child-1")
	h.AddBeadToWork("w-test", "child-2")
	h.AddBeadToWork("w-test", "child-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Run work
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// All 4 beads should get their own task
	assert.Equal(t, 4, result.TasksCreated)

	// Verify tasks in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 4)
}
