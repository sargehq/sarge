package work_test

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskExecution_SuccessfulCompletion(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Task 1")
	h.CreateBean("bean-2", "Task 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

	// Create tasks
	task1 := h.CreateTask("w-test.1", "w-test", []string{"bean-1"})
	task2 := h.CreateTask("w-test.2", "w-test", []string{"bean-2"})

	// Verify initial state
	assert.Equal(t, db.StatusPending, task1.Status)
	assert.Equal(t, db.StatusPending, task2.Status)
	assert.Equal(t, db.StatusPending, workRecord.Status)

	// Simulate task execution: start task 1
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Verify task 1 is processing
	task1After, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusProcessing, task1After.Status)
	assert.NotNil(t, task1After.StartedAt)

	// Simulate completing beans within task 1
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)

	// Check and complete task if all beans done
	completed, err := h.DB.CheckAndCompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)
	assert.True(t, completed, "task should auto-complete when all beans done")

	// Verify task 1 is completed
	task1Final, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusCompleted, task1Final.Status)
	assert.NotNil(t, task1Final.CompletedAt)

	// Start and complete task 2
	err = h.DB.StartTask(ctx, "w-test.2", workRecord.WorktreePath)
	require.NoError(t, err)

	err = h.DB.CompleteTaskBean(ctx, "w-test.2", "bean-2")
	require.NoError(t, err)

	completed, err = h.DB.CheckAndCompleteTask(ctx, "w-test.2", "")
	require.NoError(t, err)
	assert.True(t, completed)

	// Verify all tasks completed
	isCompleted, err := h.DB.IsWorkCompleted("w-test")
	require.NoError(t, err)
	assert.True(t, isCompleted, "work should be completed when all tasks are done")
}

func TestTaskExecution_Failure(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Will fail")
	h.CreateBean("bean-2", "Will not run")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

	// Create tasks
	h.CreateTask("w-test.1", "w-test", []string{"bean-1"})
	h.CreateTask("w-test.2", "w-test", []string{"bean-2"})

	// Start task 1
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Simulate failure
	err = h.DB.FailTask(ctx, "w-test.1", "Compilation error in bean-1")
	require.NoError(t, err)

	// Verify task 1 is failed
	task1, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, task1.Status)
	assert.Equal(t, "Compilation error in bean-1", task1.ErrorMessage)
	assert.NotNil(t, task1.CompletedAt, "failed task should have completed_at set")

	// Mark work as failed
	err = h.DB.FailWork(ctx, "w-test", "Task w-test.1 failed")
	require.NoError(t, err)

	// Verify work is failed
	work, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, work.Status)
	assert.Equal(t, "Task w-test.1 failed", work.ErrorMessage)

	// Task 2 should still be pending (never started)
	task2, err := h.DB.GetTask(ctx, "w-test.2")
	require.NoError(t, err)
	assert.Equal(t, db.StatusPending, task2.Status)
}

func TestTaskExecution_PartialBeanCompletion(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "First bean")
	h.CreateBean("bean-2", "Second bean")
	h.CreateBean("bean-3", "Third bean")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Create one task with multiple beans
	h.CreateTask("w-test.1", "w-test", []string{"bean-1", "bean-2", "bean-3"})

	// Start task
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Complete first bean
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)

	// Task should NOT be complete yet
	completed, err := h.DB.CheckAndCompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)
	assert.False(t, completed, "task should not be complete with only 1/3 beans done")

	// Verify bean statuses
	total, completedCount, err := h.DB.CountTaskBeanStatuses(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 1, completedCount)

	// Complete second bean
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-2")
	require.NoError(t, err)

	completed, err = h.DB.CheckAndCompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)
	assert.False(t, completed, "task should not be complete with only 2/3 beans done")

	// Complete third bean
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-3")
	require.NoError(t, err)

	// Now task should be complete
	completed, err = h.DB.CheckAndCompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)
	assert.True(t, completed, "task should be complete when all beans are done")

	// Verify final task status
	task, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusCompleted, task.Status)
}

func TestTaskExecution_WorkStatusTransitions(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Task bean")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")

	// Verify initial work status
	assert.Equal(t, db.StatusPending, workRecord.Status)

	// Start work (transition to processing)
	err := h.DB.StartWork(ctx, "w-test", "session-1", "tab-1")
	require.NoError(t, err)

	work, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusProcessing, work.Status)
	assert.Equal(t, "session-1", work.ZellijSession)
	assert.Equal(t, "tab-1", work.ZellijTab)
	assert.NotNil(t, work.StartedAt)

	// Create and complete a task
	h.CreateTask("w-test.1", "w-test", []string{"bean-1"})
	err = h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)
	_, err = h.DB.CheckAndCompleteTask(ctx, "w-test.1", "")
	require.NoError(t, err)

	// All tasks complete -> transition to idle
	err = h.DB.IdleWork(ctx, "w-test")
	require.NoError(t, err)

	work, err = h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, work.Status)

	// Resume work (back to processing when new tasks added)
	err = h.DB.ResumeWork(ctx, "w-test")
	require.NoError(t, err)

	work, err = h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusProcessing, work.Status)

	// Mark as idle again, then complete
	err = h.DB.IdleWork(ctx, "w-test")
	require.NoError(t, err)

	err = h.DB.CompleteWork(ctx, "w-test", "https://github.com/test/repo/pull/123")
	require.NoError(t, err)

	work, err = h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusCompleted, work.Status)
	assert.Equal(t, "https://github.com/test/repo/pull/123", work.PRURL)
	assert.NotNil(t, work.CompletedAt)
}

func TestTaskExecution_WorkFailureAndRestart(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Task bean")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")

	// Start work
	err := h.DB.StartWork(ctx, "w-test", "session-1", "tab-1")
	require.NoError(t, err)

	// Create and fail a task
	h.CreateTask("w-test.1", "w-test", []string{"bean-1"})
	err = h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)
	err = h.DB.FailTask(ctx, "w-test.1", "Test failure")
	require.NoError(t, err)

	// Mark work as failed
	err = h.DB.FailWork(ctx, "w-test", "Task failed")
	require.NoError(t, err)

	work, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, work.Status)

	// Restart work (only valid from failed status)
	err = h.DB.RestartWork(ctx, "w-test")
	require.NoError(t, err)

	work, err = h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusProcessing, work.Status)
}

func TestTaskExecution_BeanStatusTracking(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Bean 1")
	h.CreateBean("bean-2", "Bean 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

	// Create task with multiple beans
	h.CreateTask("w-test.1", "w-test", []string{"bean-1", "bean-2"})

	// Get initial bean statuses
	beans, err := h.DB.GetTaskBeansWithStatus(ctx, "w-test.1")
	require.NoError(t, err)
	require.Len(t, beans, 2)
	for _, bean := range beans {
		assert.Equal(t, db.StatusPending, bean.Status)
	}

	// Start task
	err = h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Complete first bean
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)

	// Check individual bean status
	status1, err := h.DB.GetTaskBeanStatus(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusCompleted, status1)

	status2, err := h.DB.GetTaskBeanStatus(ctx, "w-test.1", "bean-2")
	require.NoError(t, err)
	assert.Equal(t, db.StatusPending, status2)

	// Fail second bean
	err = h.DB.FailTaskBean(ctx, "w-test.1", "bean-2")
	require.NoError(t, err)

	status2, err = h.DB.GetTaskBeanStatus(ctx, "w-test.1", "bean-2")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, status2)
}

func TestTaskExecution_TaskReset(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "Bean 1")
	h.CreateBean("bean-2", "Bean 2")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")

	// Create and start task
	h.CreateTask("w-test.1", "w-test", []string{"bean-1", "bean-2"})
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)

	// Complete one bean, fail the other
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)
	err = h.DB.FailTaskBean(ctx, "w-test.1", "bean-2")
	require.NoError(t, err)

	// Fail the task
	err = h.DB.FailTask(ctx, "w-test.1", "bean-2 failed")
	require.NoError(t, err)

	// Verify failed state
	task, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, task.Status)

	// Reset task status
	err = h.DB.ResetTaskStatus(ctx, "w-test.1")
	require.NoError(t, err)

	// Verify reset task is pending
	task, err = h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, db.StatusPending, task.Status)

	// Reset all bean statuses
	err = h.DB.ResetTaskBeanStatuses(ctx, "w-test.1")
	require.NoError(t, err)

	// Verify beans are reset to pending
	beans, err := h.DB.GetTaskBeansWithStatus(ctx, "w-test.1")
	require.NoError(t, err)
	for _, bean := range beans {
		assert.Equal(t, db.StatusPending, bean.Status)
	}
}

func TestTaskExecution_WorkMergedTransition(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create work
	h.CreateWork("w-test", "feat/test-branch")

	// Start work
	err := h.DB.StartWork(ctx, "w-test", "session-1", "tab-1")
	require.NoError(t, err)

	// Mark work as idle with PR URL
	err = h.DB.IdleWorkWithPR(ctx, "w-test", "https://github.com/test/repo/pull/123")
	require.NoError(t, err)

	work, err := h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, work.Status)
	assert.Equal(t, "https://github.com/test/repo/pull/123", work.PRURL)

	// Simulate PR merge detection
	err = h.DB.MergeWork(ctx, "w-test")
	require.NoError(t, err)

	work, err = h.DB.GetWork(ctx, "w-test")
	require.NoError(t, err)
	assert.Equal(t, db.StatusMerged, work.Status)
	assert.NotNil(t, work.CompletedAt)
}

func TestTaskExecution_MultiTaskSequentialExecution(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "First")
	h.CreateBean("bean-2", "Second")
	h.CreateBean("bean-3", "Third")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Create tasks in sequence
	h.CreateTask("w-test.1", "w-test", []string{"bean-1"})
	h.CreateTask("w-test.2", "w-test", []string{"bean-2"})
	h.CreateTask("w-test.3", "w-test", []string{"bean-3"})

	// Start work
	err := h.DB.StartWork(ctx, "w-test", "session-1", "tab-1")
	require.NoError(t, err)

	// Execute tasks sequentially
	for i := 1; i <= 3; i++ {
		taskID := taskID("w-test", i)
		beanID := beanID(i)

		// Start task
		err = h.DB.StartTask(ctx, taskID, workRecord.WorktreePath)
		require.NoError(t, err)

		// Complete bean
		err = h.DB.CompleteTaskBean(ctx, taskID, beanID)
		require.NoError(t, err)

		// Complete task
		completed, err := h.DB.CheckAndCompleteTask(ctx, taskID, "")
		require.NoError(t, err)
		assert.True(t, completed, "task %d should complete", i)
	}

	// Verify all tasks completed
	tasks, err := h.DB.GetWorkTasks(ctx, "w-test")
	require.NoError(t, err)
	for _, task := range tasks {
		assert.Equal(t, db.StatusCompleted, task.Status)
	}

	// Work should be completable
	isComplete, err := h.DB.IsWorkCompleted("w-test")
	require.NoError(t, err)
	assert.True(t, isComplete)
}

func TestTaskExecution_GetTaskBeansForWork(t *testing.T) {
	h := testutil.NewTestHarness(t)
	defer h.Cleanup()

	ctx := context.Background()

	// Create beans and work
	h.CreateBean("bean-1", "First")
	h.CreateBean("bean-2", "Second")
	h.CreateBean("bean-3", "Third")
	workRecord := h.CreateWork("w-test", "feat/test-branch")
	h.AddBeanToWork("w-test", "bean-1")
	h.AddBeanToWork("w-test", "bean-2")
	h.AddBeanToWork("w-test", "bean-3")

	// Create tasks
	h.CreateTask("w-test.1", "w-test", []string{"bean-1", "bean-2"})
	h.CreateTask("w-test.2", "w-test", []string{"bean-3"})

	// Start first task and complete one bean
	err := h.DB.StartTask(ctx, "w-test.1", workRecord.WorktreePath)
	require.NoError(t, err)
	err = h.DB.CompleteTaskBean(ctx, "w-test.1", "bean-1")
	require.NoError(t, err)

	// Get all task beans for work
	taskBeans, err := h.DB.GetTaskBeansForWork(ctx, "w-test")
	require.NoError(t, err)
	require.Len(t, taskBeans, 3)

	// Verify statuses
	statusMap := make(map[string]string)
	for _, tb := range taskBeans {
		statusMap[tb.BeanID] = tb.Status
	}

	assert.Equal(t, db.StatusCompleted, statusMap["bean-1"])
	assert.Equal(t, db.StatusPending, statusMap["bean-2"])
	assert.Equal(t, db.StatusPending, statusMap["bean-3"])
}

// Helper functions for generating IDs
func taskID(workID string, num int) string {
	return workID + "." + string(rune('0'+num))
}

func beanID(num int) string {
	return "bean-" + string(rune('0'+num))
}
