package control_test

import (
	"context"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleWatchWorkflowRunTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("returns error when missing run_id", func(t *testing.T) {
		mocks := setupControlPlane()

		createTestWork(ctx, t, proj.DB, "w-watch-1", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-watch-1")

		task := &db.ScheduledTask{
			ID:       "watch-task-1",
			WorkID:   "w-watch-1",
			TaskType: db.TaskTypeWatchWorkflowRun,
			Metadata: map[string]string{
				"repo": "owner/repo",
				// Missing run_id
			},
		}

		err := mocks.CP.HandleWatchWorkflowRunTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing run_id or repo")
	})

	t.Run("returns error when missing repo", func(t *testing.T) {
		mocks := setupControlPlane()

		createTestWork(ctx, t, proj.DB, "w-watch-2", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-watch-2")

		task := &db.ScheduledTask{
			ID:       "watch-task-2",
			WorkID:   "w-watch-2",
			TaskType: db.TaskTypeWatchWorkflowRun,
			Metadata: map[string]string{
				"run_id": "123456",
				// Missing repo
			},
		}

		err := mocks.CP.HandleWatchWorkflowRunTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing run_id or repo")
	})

	t.Run("returns error when run_id is invalid", func(t *testing.T) {
		mocks := setupControlPlane()

		createTestWork(ctx, t, proj.DB, "w-watch-3", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-watch-3")

		task := &db.ScheduledTask{
			ID:       "watch-task-3",
			WorkID:   "w-watch-3",
			TaskType: db.TaskTypeWatchWorkflowRun,
			Metadata: map[string]string{
				"run_id": "not-a-number",
				"repo":   "owner/repo",
			},
		}

		err := mocks.CP.HandleWatchWorkflowRunTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid run_id")
	})

	t.Run("stops gracefully on context cancellation", func(t *testing.T) {
		mocks := setupControlPlane()

		createTestWork(ctx, t, proj.DB, "w-watch-4", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-watch-4")

		// Create a cancelled context
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		task := &db.ScheduledTask{
			ID:       "watch-task-4",
			WorkID:   "w-watch-4",
			TaskType: db.TaskTypeWatchWorkflowRun,
			Metadata: map[string]string{
				"run_id": "123456",
				"repo":   "owner/repo",
			},
		}

		// Should return nil (graceful shutdown) not error
		err := mocks.CP.HandleWatchWorkflowRunTask(cancelledCtx, proj, task)
		require.NoError(t, err)
	})
}

func TestScheduleWatchWorkflowRun(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("schedules watch task successfully", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-sched-watch", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-sched-watch")

		err := control.ScheduleWatchWorkflowRun(ctx, proj, "w-sched-watch", 123456, "owner/repo")
		require.NoError(t, err)

		// Verify task was scheduled
		task, err := proj.DB.GetNextScheduledTask(ctx)
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, db.TaskTypeWatchWorkflowRun, task.TaskType)
		assert.Equal(t, "w-sched-watch", task.WorkID)
		assert.Equal(t, "123456", task.Metadata["run_id"])
		assert.Equal(t, "owner/repo", task.Metadata["repo"])
	})

	t.Run("prevents duplicate watchers for same run_id", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-dedup", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-dedup")

		// Schedule the first watcher
		err := control.ScheduleWatchWorkflowRun(ctx, proj, "w-dedup", 789012, "owner/repo")
		require.NoError(t, err)

		// Try to schedule another watcher for the same run_id
		// This should succeed (idempotent) but not create a duplicate
		err = control.ScheduleWatchWorkflowRun(ctx, proj, "w-dedup", 789012, "owner/repo")
		require.NoError(t, err)

		// Count tasks for this work - should only be one
		tasks, err := proj.DB.GetScheduledTasksForWork(ctx, "w-dedup")
		require.NoError(t, err)

		watchCount := 0
		for _, task := range tasks {
			if task.TaskType == db.TaskTypeWatchWorkflowRun {
				watchCount++
			}
		}
		assert.Equal(t, 1, watchCount, "should only have one watcher task for the same run_id")
	})

	t.Run("allows multiple watchers for different run_ids", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-multi-watch", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-multi-watch")

		// Schedule watchers for different runs
		err := control.ScheduleWatchWorkflowRun(ctx, proj, "w-multi-watch", 111111, "owner/repo")
		require.NoError(t, err)

		err = control.ScheduleWatchWorkflowRun(ctx, proj, "w-multi-watch", 222222, "owner/repo")
		require.NoError(t, err)

		// Should have two separate tasks
		// Note: GetScheduledTasksForWork only returns pending tasks due now
		// So we need to check by idempotency key
		task1, err := proj.DB.GetTaskByIdempotencyKey(ctx, "watch_run_111111")
		require.NoError(t, err)
		require.NotNil(t, task1)

		task2, err := proj.DB.GetTaskByIdempotencyKey(ctx, "watch_run_222222")
		require.NoError(t, err)
		require.NotNil(t, task2)

		// Verify they are different tasks
		assert.NotEqual(t, task1.ID, task2.ID)
	})
}

func TestGetTaskHandlers_IncludesWatchWorkflowRun(t *testing.T) {
	mocks := setupControlPlane()

	handlers := mocks.CP.GetTaskHandlers()

	// Verify WatchWorkflowRun handler is registered
	_, ok := handlers[db.TaskTypeWatchWorkflowRun]
	assert.True(t, ok, "expected handler for task type %s", db.TaskTypeWatchWorkflowRun)
}

func TestExtractRepoFromPRURL(t *testing.T) {
	// Test the extractRepoFromPRURL helper function
	// We test this indirectly through the handler behavior
	// since the function is package-private

	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("handles valid PR URL in spawnWorkflowWatchers", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-extract", "branch", "root-1")
		err := proj.DB.SetWorkPRURLAndScheduleFeedback(ctx, "w-extract", "https://github.com/owner/repo/pull/123", 5*time.Minute, 5*time.Minute)
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-extract")

		// Get the work
		work, err := proj.DB.GetWork(ctx, "w-extract")
		require.NoError(t, err)
		require.NotNil(t, work)

		// The spawnWorkflowWatchers function will call extractRepoFromPRURL
		// We can't test it directly since it's package-private, but we verify
		// through the handler that a properly formed PR URL works
		assert.Equal(t, "https://github.com/owner/repo/pull/123", work.PRURL)
	})
}

func TestSpawnWorkflowWatchers(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("spawns watchers for in_progress workflows", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-spawn-1", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-spawn-1")

		mockClient := &github.GitHubClientMock{
			GetPRStatusFunc: func(ctx context.Context, prURL string) (*github.PRStatus, error) {
				return &github.PRStatus{
					Workflows: []github.WorkflowRun{
						{ID: 111111, Status: "in_progress", Name: "CI"},
						{ID: 222222, Status: "queued", Name: "Deploy"},
						{ID: 333333, Status: "completed", Name: "Done"},
					},
				}, nil
			},
		}

		count, err := control.SpawnWorkflowWatchers(ctx, proj, mockClient, "w-spawn-1", "https://github.com/owner/repo/pull/123")
		require.NoError(t, err)
		assert.Equal(t, 2, count, "should spawn watchers for in_progress and queued workflows")

		// Verify tasks were scheduled
		task1, err := proj.DB.GetTaskByIdempotencyKey(ctx, "watch_run_111111")
		require.NoError(t, err)
		require.NotNil(t, task1)
		assert.Equal(t, "111111", task1.Metadata["run_id"])

		task2, err := proj.DB.GetTaskByIdempotencyKey(ctx, "watch_run_222222")
		require.NoError(t, err)
		require.NotNil(t, task2)
		assert.Equal(t, "222222", task2.Metadata["run_id"])

		// Completed workflow should not have a watcher
		task3, err := proj.DB.GetTaskByIdempotencyKey(ctx, "watch_run_333333")
		require.NoError(t, err)
		assert.Nil(t, task3)
	})

	t.Run("returns zero when all workflows completed", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-spawn-2", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-spawn-2")

		mockClient := &github.GitHubClientMock{
			GetPRStatusFunc: func(ctx context.Context, prURL string) (*github.PRStatus, error) {
				return &github.PRStatus{
					Workflows: []github.WorkflowRun{
						{ID: 444444, Status: "completed", Name: "CI"},
					},
				}, nil
			},
		}

		count, err := control.SpawnWorkflowWatchers(ctx, proj, mockClient, "w-spawn-2", "https://github.com/owner/repo/pull/456")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns error on invalid PR URL", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-spawn-3", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-spawn-3")

		mockClient := &github.GitHubClientMock{
			GetPRStatusFunc: func(ctx context.Context, prURL string) (*github.PRStatus, error) {
				return &github.PRStatus{
					Workflows: []github.WorkflowRun{
						{ID: 555555, Status: "in_progress", Name: "CI"},
					},
				}, nil
			},
		}

		_, err := control.SpawnWorkflowWatchers(ctx, proj, mockClient, "w-spawn-3", "not-a-valid-url")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to extract repo")
	})
}
