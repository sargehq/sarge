package control_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncTaskExecutor_BasicExecution(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	executed := make(chan string, 10)
	handlers := map[string]control.TaskHandler{
		"test_async": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			executed <- task.ID
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	task := &db.ScheduledTask{
		ID:       "task-1",
		WorkID:   "w-test",
		TaskType: "test_async",
	}

	submitted := executor.Submit(ctx, task)
	require.True(t, submitted, "task should be submitted")

	// Wait for completion
	executor.Wait()

	select {
	case id := <-executed:
		assert.Equal(t, "task-1", id)
	default:
		t.Fatal("expected task to be executed")
	}
}

func TestAsyncTaskExecutor_MultipleConcurrentTasks(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	const numTasks = 3
	var running atomic.Int32
	var maxConcurrent atomic.Int32
	allStarted := make(chan struct{})
	var startedCount atomic.Int32

	handlers := map[string]control.TaskHandler{
		"concurrent_test": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			current := running.Add(1)
			// Track max concurrent
			for {
				old := maxConcurrent.Load()
				if current <= old || maxConcurrent.CompareAndSwap(old, current) {
					break
				}
			}

			// Signal that this task started
			if startedCount.Add(1) == numTasks {
				close(allStarted)
			}

			// Wait a bit to ensure overlap
			time.Sleep(50 * time.Millisecond)

			running.Add(-1)
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	// Submit multiple tasks
	for i := 0; i < numTasks; i++ {
		task := &db.ScheduledTask{
			ID:       "task-" + string(rune('0'+i)),
			WorkID:   "w-test",
			TaskType: "concurrent_test",
		}
		executor.Submit(ctx, task)
	}

	// Wait for all to start
	select {
	case <-allStarted:
		// All tasks started concurrently
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for tasks to start")
	}

	executor.Wait()

	// All tasks should have run concurrently
	assert.Equal(t, int32(numTasks), maxConcurrent.Load(), "all tasks should run concurrently")
}

func TestAsyncTaskExecutor_ConcurrencyLimitRespected(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	const maxConcurrent = 2
	const numTasks = 5
	var running atomic.Int32
	var maxObserved atomic.Int32

	proceed := make(chan struct{})

	handlers := map[string]control.TaskHandler{
		"limited_test": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			current := running.Add(1)
			// Track max observed
			for {
				old := maxObserved.Load()
				if current <= old || maxObserved.CompareAndSwap(old, current) {
					break
				}
			}

			<-proceed // Wait for signal

			running.Add(-1)
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, maxConcurrent)

	// Submit more tasks than concurrency limit
	for i := 0; i < numTasks; i++ {
		task := &db.ScheduledTask{
			ID:       "task-" + string(rune('a'+i)),
			WorkID:   "w-test",
			TaskType: "limited_test",
		}
		executor.Submit(ctx, task)
	}

	// Give tasks time to start
	time.Sleep(100 * time.Millisecond)

	// Should have exactly maxConcurrent running
	assert.Equal(t, int32(maxConcurrent), running.Load(), "should have exactly maxConcurrent tasks running")
	assert.LessOrEqual(t, maxObserved.Load(), int32(maxConcurrent), "should never exceed maxConcurrent")

	// Release all tasks
	close(proceed)
	executor.Wait()

	// Verify we never exceeded limit
	assert.LessOrEqual(t, maxObserved.Load(), int32(maxConcurrent), "should never have exceeded concurrency limit")
}

func TestAsyncTaskExecutor_GracefulShutdownWaitsForTasks(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	taskCompleted := make(chan struct{})
	taskStarted := make(chan struct{})

	handlers := map[string]control.TaskHandler{
		"slow_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			close(taskStarted)
			time.Sleep(100 * time.Millisecond)
			close(taskCompleted)
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	task := &db.ScheduledTask{
		ID:       "slow-1",
		WorkID:   "w-test",
		TaskType: "slow_task",
	}
	executor.Submit(ctx, task)

	// Wait for task to start
	<-taskStarted

	// Wait should block until task completes
	done := make(chan struct{})
	go func() {
		executor.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Wait completed
	case <-time.After(5 * time.Second):
		t.Fatal("Wait() should complete after task finishes")
	}

	// Verify task actually completed
	select {
	case <-taskCompleted:
		// Good
	default:
		t.Fatal("task should have completed before Wait() returned")
	}
}

func TestAsyncTaskExecutor_WaitWithContextTimeout(t *testing.T) {
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	handlers := map[string]control.TaskHandler{
		"blocking_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			// Block forever (until context cancelled)
			<-ctx.Done()
			return ctx.Err()
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	// Use a parent context for the task
	taskCtx, taskCancel := context.WithCancel(context.Background())
	defer taskCancel()

	task := &db.ScheduledTask{
		ID:       "blocking-1",
		WorkID:   "w-test",
		TaskType: "blocking_task",
	}
	executor.Submit(taskCtx, task)

	// Give task time to start
	time.Sleep(50 * time.Millisecond)

	// WaitWithContext should timeout
	waitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := executor.WaitWithContext(waitCtx)
	assert.ErrorIs(t, err, context.DeadlineExceeded, "WaitWithContext should timeout")

	// Cleanup: cancel task and wait for it
	taskCancel()
	executor.Wait()
}

func TestAsyncTaskExecutor_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	createTestWork(ctx, t, proj.DB, "w-error-test", "branch", "root-1")
	defer proj.DB.DeleteWork(ctx, "w-error-test")

	expectedErr := errors.New("task failed intentionally")

	handlers := map[string]control.TaskHandler{
		"failing_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			return expectedErr
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	// Track completion via callback
	var completedErr error
	completionCalled := make(chan struct{})
	executor.SetCompletionCallback(func(task *db.ScheduledTask, err error) {
		completedErr = err
		close(completionCalled)
	})

	// Schedule the task first so the database knows about it
	err := proj.DB.ScheduleTaskWithRetry(ctx, "w-error-test", "failing_task", time.Now(), nil, "error-test", 1)
	require.NoError(t, err)

	// Get the task
	task, err := proj.DB.GetNextScheduledTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, task)

	// Mark as executing and submit
	err = proj.DB.MarkTaskExecuting(ctx, task.ID)
	require.NoError(t, err)

	executor.Submit(ctx, task)

	// Wait for completion callback
	select {
	case <-completionCalled:
		assert.Equal(t, expectedErr, completedErr, "error should be passed to callback")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for completion callback")
	}

	executor.Wait()
}

func TestAsyncTaskExecutor_PanicRecovery(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	createTestWork(ctx, t, proj.DB, "w-panic-test", "branch", "root-1")
	defer proj.DB.DeleteWork(ctx, "w-panic-test")

	handlers := map[string]control.TaskHandler{
		"panicking_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			panic("intentional panic for testing")
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	// Track completion via callback
	var completedErr error
	completionCalled := make(chan struct{})
	executor.SetCompletionCallback(func(task *db.ScheduledTask, err error) {
		completedErr = err
		close(completionCalled)
	})

	// Schedule the task
	err := proj.DB.ScheduleTaskWithRetry(ctx, "w-panic-test", "panicking_task", time.Now(), nil, "panic-test", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetNextScheduledTask(ctx)
	require.NoError(t, err)
	require.NotNil(t, task)

	err = proj.DB.MarkTaskExecuting(ctx, task.ID)
	require.NoError(t, err)

	executor.Submit(ctx, task)

	// Wait for completion callback (should be called even on panic)
	select {
	case <-completionCalled:
		assert.Error(t, completedErr, "panic should result in error")
		assert.Contains(t, completedErr.Error(), "panic", "error should indicate panic")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for completion callback after panic")
	}

	// Executor should still be usable after panic
	executor.Wait()
}

func TestAsyncTaskExecutor_DuplicateTaskPrevention(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	taskStarted := make(chan struct{})
	canProceed := make(chan struct{})

	handlers := map[string]control.TaskHandler{
		"blocking_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			close(taskStarted)
			<-canProceed
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	task := &db.ScheduledTask{
		ID:       "dup-task",
		WorkID:   "w-test",
		TaskType: "blocking_task",
	}

	// First submission should succeed
	submitted1 := executor.Submit(ctx, task)
	assert.True(t, submitted1, "first submission should succeed")

	// Wait for task to start
	<-taskStarted

	// Second submission with same ID should be rejected
	submitted2 := executor.Submit(ctx, task)
	assert.False(t, submitted2, "duplicate submission should be rejected")

	// Verify only one task is running
	assert.Equal(t, 1, executor.RunningCount())
	assert.True(t, executor.IsRunning("dup-task"))

	// Let task complete
	close(canProceed)
	executor.Wait()

	// After completion, same ID can be submitted again
	canProceed = make(chan struct{})
	taskStarted = make(chan struct{})

	// Create new handlers for second submission
	handlers2 := map[string]control.TaskHandler{
		"blocking_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			close(taskStarted)
			<-canProceed
			return nil
		},
	}
	executor2 := control.NewAsyncTaskExecutor(proj, handlers2, 5)

	submitted3 := executor2.Submit(ctx, task)
	assert.True(t, submitted3, "should be able to resubmit after completion")
	close(canProceed)
	executor2.Wait()
}

func TestAsyncTaskExecutor_ContextCancellationWhileWaitingForSemaphore(t *testing.T) {
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	var taskExecuted atomic.Bool
	blocker := make(chan struct{})

	handlers := map[string]control.TaskHandler{
		"blocking": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			<-blocker
			return nil
		},
		"test_task": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			taskExecuted.Store(true)
			return nil
		},
	}

	// Create executor with concurrency limit of 1
	executor := control.NewAsyncTaskExecutor(proj, handlers, 1)

	// Fill the semaphore with a blocking task
	blockingTask := &db.ScheduledTask{
		ID:       "blocker",
		WorkID:   "w-test",
		TaskType: "blocking",
	}
	executor.Submit(context.Background(), blockingTask)

	// Give blocker time to acquire semaphore
	time.Sleep(50 * time.Millisecond)

	// Submit a task with a context that gets cancelled
	ctx, cancel := context.WithCancel(context.Background())

	task := &db.ScheduledTask{
		ID:       "cancelled-task",
		WorkID:   "w-test",
		TaskType: "test_task",
	}
	executor.Submit(ctx, task)

	// Cancel the context while waiting for semaphore
	cancel()

	// Give time for cancellation to be detected
	time.Sleep(50 * time.Millisecond)

	// Release the blocker
	close(blocker)
	executor.Wait()

	// Task should not have executed due to context cancellation
	assert.False(t, taskExecuted.Load(), "task should not execute after context cancellation")
}

func TestAsyncTaskExecutor_AsyncDoesNotBlockSync(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	// Create work for task
	createTestWork(ctx, t, proj.DB, "w-block-test", "branch", "root-1")
	defer proj.DB.DeleteWork(ctx, "w-block-test")

	// Track execution order
	var execOrder []string
	var mu sync.Mutex
	asyncStarted := make(chan struct{})
	asyncCanProceed := make(chan struct{})

	// Set up control plane mocks
	mocks := setupControlPlane()

	// Fast sync handler
	mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
		mu.Lock()
		execOrder = append(execOrder, "sync")
		mu.Unlock()
		return nil
	}

	// Create async handler
	asyncHandlers := map[string]control.TaskHandler{
		"slow_async": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			close(asyncStarted)
			<-asyncCanProceed // Block until told to proceed
			mu.Lock()
			execOrder = append(execOrder, "async")
			mu.Unlock()
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, asyncHandlers, 5)

	// Submit async task first
	asyncTask := &db.ScheduledTask{
		ID:       "async-task",
		WorkID:   "w-block-test",
		TaskType: "slow_async",
	}
	executor.Submit(ctx, asyncTask)

	// Wait for async task to start
	<-asyncStarted

	// Schedule and process a sync task
	_, err := proj.DB.ScheduleTask(ctx, "w-block-test", db.TaskTypeGitPush, time.Now(), map[string]string{
		"branch": "branch",
		"dir":    "/work/dir",
	})
	require.NoError(t, err)

	// Process sync task directly (simulating what the main loop does)
	control.ProcessAllDueTasksWithControlPlane(ctx, proj, mocks.CP)

	// Verify sync task completed while async is still running
	mu.Lock()
	syncCompleted := len(execOrder) == 1 && execOrder[0] == "sync"
	mu.Unlock()
	assert.True(t, syncCompleted, "sync task should complete before async")

	// Let async task finish
	close(asyncCanProceed)
	executor.Wait()

	mu.Lock()
	assert.Equal(t, []string{"sync", "async"}, execOrder, "sync should complete before async")
	mu.Unlock()
}

func TestAsyncTaskExecutor_RunningTaskMethods(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	started := make(chan struct{})
	proceed := make(chan struct{})

	handlers := map[string]control.TaskHandler{
		"test": func(ctx context.Context, _ *project.Project, task *db.ScheduledTask) error {
			close(started)
			<-proceed
			return nil
		},
	}

	executor := control.NewAsyncTaskExecutor(proj, handlers, 5)

	// Initially no tasks running
	assert.Equal(t, 0, executor.RunningCount())
	assert.False(t, executor.IsRunning("test-task"))

	task := &db.ScheduledTask{
		ID:       "test-task",
		WorkID:   "w-test",
		TaskType: "test",
	}
	executor.Submit(ctx, task)

	// Wait for task to start
	<-started

	// Task should be running
	assert.Equal(t, 1, executor.RunningCount())
	assert.True(t, executor.IsRunning("test-task"))
	assert.False(t, executor.IsRunning("other-task"))

	// Let task complete
	close(proceed)
	executor.Wait()

	// Task should no longer be running
	assert.Equal(t, 0, executor.RunningCount())
	assert.False(t, executor.IsRunning("test-task"))
}
