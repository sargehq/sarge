package work_test

import (
	"context"
	"io"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunWork_CreatesTasksFromBeans(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create test beans
	h.CreateBean("bean-1", "Implement feature A")
	h.CreateBean("bean-2", "Implement feature B")
	h.CreateBean("bean-3", "Implement feature C")

	// Create work with beans
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Run work without planning (creates one task per bean)
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify tasks were created
	assert.Equal(t, "w-test", result.WorkID)
	assert.Equal(t, 3, result.TasksCreated, "expected 3 tasks (one per bean)")

	// Verify task records in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 3)

	// Verify each task has correct bean association
	for i, task := range tasks {
		beanIDs, err := h.DB.GetTaskBeans(ctx, task.ID)
		require.NoError(t, err)
		assert.Len(t, beanIDs, 1, "task %d should have 1 bean", i)
		assert.Equal(t, "implement", task.TaskType)
		assert.Equal(t, db.StatusPending, task.Status)
	}
}

func TestRunWork_RespectsBeanDependencies(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans with dependencies
	// bean-2 depends on bean-1
	// bean-3 depends on bean-2
	h.CreateBean("bean-1", "Base feature")
	h.CreateBean("bean-2", "Depends on bean-1")
	h.CreateBean("bean-3", "Depends on bean-2")

	h.SetBeanDependency("bean-2", "bean-1")
	h.SetBeanDependency("bean-3", "bean-2")

	// Create work with beans
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Configure task planner to return tasks in correct order
	h.TaskPlanner.PlanFunc = func(ctx context.Context, beanList []beans.Bean, dependencies map[string][]beans.Dependency, budget int) ([]task.Task, error) {
		// Return tasks respecting dependencies: bean-1, then bean-2, then bean-3
		return []task.Task{
			{ID: "task-1", BeanIDs: []string{"bean-1"}, Beans: []beans.Bean{{ID: "bean-1"}}},
			{ID: "task-2", BeanIDs: []string{"bean-2"}, Beans: []beans.Bean{{ID: "bean-2"}}},
			{ID: "task-3", BeanIDs: []string{"bean-3"}, Beans: []beans.Bean{{ID: "bean-3"}}},
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
	assert.Len(t, calls[0].BeanList, 3)
	assert.Contains(t, calls[0].Dependencies, "bean-2", "dependencies should include bean-2")
	assert.Contains(t, calls[0].Dependencies, "bean-3", "dependencies should include bean-3")
}

func TestRunWork_WithPlanningEnabled(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans
	h.CreateBean("bean-1", "Simple task")
	h.CreateBean("bean-2", "Another simple task")
	h.CreateBean("bean-3", "Complex task")

	// Create work with beans
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Configure task planner to group beans by complexity
	// Simple beans grouped together, complex bean in separate task
	h.TaskPlanner.PlanFunc = func(ctx context.Context, beanList []beans.Bean, dependencies map[string][]beans.Dependency, budget int) ([]task.Task, error) {
		return []task.Task{
			{
				ID:              "task-1",
				BeanIDs:         []string{"bean-1", "bean-2"},
				Beans:           []beans.Bean{{ID: "bean-1"}, {ID: "bean-2"}},
				Complexity:      4,
				EstimatedTokens: 20000,
			},
			{
				ID:              "task-2",
				BeanIDs:         []string{"bean-3"},
				Beans:           []beans.Bean{{ID: "bean-3"}},
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
	// Second task (higher number) should have 1 bean
	task2Beans, err := h.DB.GetTaskBeans(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Len(t, task2Beans, 1)
	assert.Contains(t, task2Beans, "bean-3")

	// First task (lower number) should have 2 beans
	task1Beans, err := h.DB.GetTaskBeans(ctx, tasks[1].ID)
	require.NoError(t, err)
	assert.Len(t, task1Beans, 2)
	assert.Contains(t, task1Beans, "bean-1")
	assert.Contains(t, task1Beans, "bean-2")
}

func TestRunWork_SkipsAlreadyAssignedBeans(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create initial bean
	h.CreateBean("bean-1", "Initially assigned")

	// Create work with initial bean
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// First run: creates task for bean-1
	result1, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, 1, result1.TasksCreated)

	// Now add more beans to the work
	h.CreateBean("bean-2", "Newly added")
	h.CreateBean("bean-3", "Also newly added")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Second run: should only create tasks for newly added beans
	result2, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result2)

	// Should only create 2 tasks (for bean-2 and bean-3)
	assert.Equal(t, 2, result2.TasksCreated, "expected 2 tasks for newly added beans")

	// Verify total tasks in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 3, "should have 3 total tasks")
}

func TestRunWork_SpawnsOrchestrator(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create a bean and work
	h.CreateBean("bean-1", "Test bean")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")

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

	// Create beans and work
	h.CreateBean("bean-1", "Test bean 1")
	h.CreateBean("bean-2", "Test bean 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

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

	// Second run: should not create more tasks (all beans already assigned)
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

	// Create beans and work
	h.CreateBean("bean-1", "Test bean 1")
	h.CreateBean("bean-2", "Test bean 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

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

	// Create beans and work
	h.CreateBean("bean-1", "Task 1")
	h.CreateBean("bean-2", "Task 2")
	h.CreateBean("bean-3", "Task 3")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

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

	// Verify all beans are in the estimate task
	beanIDs, err := h.DB.GetTaskBeans(ctx, tasks[0].ID)
	require.NoError(t, err)
	assert.Len(t, beanIDs, 3)
	assert.Contains(t, beanIDs, "bean-1")
	assert.Contains(t, beanIDs, "bean-2")
	assert.Contains(t, beanIDs, "bean-3")
}

func TestCreateEstimateTaskFromWorkBeans_FailsWithNoUnassignedBeans(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work without beans
	h.CreateWork("w-test", "feat/test-branch")

	// Should fail with no unassigned beans
	err := h.WorkService.CreateEstimateTaskFromWorkBeans(ctx, "w-test", io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no unassigned beans")
}

func TestRunWork_WithEpicBeans(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create an epic with children
	h.CreateEpicWithChildren("epic-1", "child-1", "child-2", "child-3")

	// Create work with all beans
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "epic-1")
	h.AddBeanToWork("w-test", "child-1")
	h.AddBeanToWork("w-test", "child-2")
	h.AddBeanToWork("w-test", "child-3")

	// Configure worktree to exist
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return worktreePath == workRecord.WorktreePath
	}

	// Run work
	result, err := h.WorkService.RunWork(ctx, "w-test", false, io.Discard)
	require.NoError(t, err)
	require.NotNil(t, result)

	// All 4 beans should get their own task
	assert.Equal(t, 4, result.TasksCreated)

	// Verify tasks in database
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	assert.Len(t, tasks, 4)
}
