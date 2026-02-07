package control

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/procmon"
	"github.com/sargehq/sarge/internal/project"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
)

// RunControlPlaneLoop runs the main control plane event loop with default dependencies.
func RunControlPlaneLoop(ctx context.Context, proj *project.Project, procManager *procmon.Manager) error {
	cp := NewControlPlane(proj)
	return RunControlPlaneLoopWithControlPlane(ctx, proj, procManager, cp)
}

// RunControlPlaneLoopWithControlPlane runs the main control plane event loop with provided dependencies.
// This allows testing with mock dependencies.
func RunControlPlaneLoopWithControlPlane(ctx context.Context, proj *project.Project, procManager *procmon.Manager, cp *ControlPlane) error {
	// Reset any scheduled tasks stuck in 'executing' status from a previous crash.
	// This must happen before we start processing tasks to avoid leaving them orphaned.
	resetCount, err := proj.DB.ResetExecutingTasksToPending(ctx)
	if err != nil {
		logging.Warn("Failed to reset executing tasks on startup", "error", err)
	} else if resetCount > 0 {
		logging.Info("Reset stuck executing tasks to pending on startup", "count", resetCount)
		fmt.Printf("Recovered %d task(s) stuck in executing state from previous crash\n", resetCount)
	}

	// Initialize tracking database watcher
	trackingDBPath := filepath.Join(proj.Root, ".co", "tracking.db")
	watcher, err := trackingwatcher.New(trackingwatcher.DefaultConfig(trackingDBPath))
	if err != nil {
		return fmt.Errorf("failed to create tracking watcher: %w", err)
	}

	if err := watcher.Start(); err != nil {
		_ = watcher.Stop()
		return fmt.Errorf("failed to start tracking watcher: %w", err)
	}
	defer watcher.Stop()

	// Create async task executor for long-running tasks
	asyncExecutor := NewAsyncTaskExecutor(proj, cp.GetTaskHandlers(), DefaultMaxConcurrentAsyncTasks)
	defer func() {
		logging.Debug("Waiting for async tasks to complete...")
		asyncExecutor.Wait()
		logging.Debug("All async tasks completed")
	}()

	logging.Info("Control plane started with database events",
		"max_concurrent_async", DefaultMaxConcurrentAsyncTasks)

	// Subscribe to watcher events
	sub := watcher.Broker().Subscribe(ctx)

	// Set up periodic check timer (safety net)
	checkInterval := 30 * time.Second
	checkTimer := time.NewTimer(checkInterval)
	defer checkTimer.Stop()

	// Set up periodic cleanup timer for stale processes
	cleanupInterval := 60 * time.Second
	cleanupTimer := time.NewTimer(cleanupInterval)
	defer cleanupTimer.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.Debug("Control plane stopping due to context cancellation")
			if runningCount := asyncExecutor.RunningCount(); runningCount > 0 {
				fmt.Printf("\nWaiting for %d async task(s) to complete...\n", runningCount)
			}
			fmt.Println("\nControl plane stopped.")
			return nil

		case <-procManager.EvictedCh():
			logging.Info("Control plane evicted by new process, shutting down gracefully")
			if runningCount := asyncExecutor.RunningCount(); runningCount > 0 {
				fmt.Printf("\nWaiting for %d async task(s) to complete...\n", runningCount)
			}
			fmt.Println("\nControl plane evicted, stopped.")
			return nil

		case event, ok := <-sub:
			if !ok {
				logging.Debug("Watcher subscription closed")
				return nil
			}

			// Handle database change event
			if event.Payload.Type == trackingwatcher.DBChanged {
				logging.Debug("Database changed, checking scheduled tasks")
				processAllDueTasksWithExecutor(ctx, proj, cp, asyncExecutor)
			}

		case <-checkTimer.C:
			// Periodic check as a safety net
			logging.Debug("Control plane periodic check")
			processAllDueTasksWithExecutor(ctx, proj, cp, asyncExecutor)
			checkTimer.Reset(checkInterval)

		case <-cleanupTimer.C:
			// Periodic cleanup of stale processes
			logging.Debug("Control plane cleaning up stale processes")
			if err := procManager.CleanupStaleProcessRecords(ctx); err != nil {
				logging.Warn("failed to cleanup stale processes", "error", err)
			}
			cleanupTimer.Reset(cleanupInterval)
		}
	}
}

// TaskHandler is the signature for all scheduled task handlers.
type TaskHandler func(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error

// ProcessAllDueTasks checks for and executes any scheduled tasks that are due across all works.
// This uses the default ControlPlane with production dependencies.
func ProcessAllDueTasks(ctx context.Context, proj *project.Project) {
	cp := NewControlPlane(proj)
	ProcessAllDueTasksWithControlPlane(ctx, proj, cp)
}

// ProcessAllDueTasksWithControlPlane checks for and executes any scheduled tasks with provided dependencies.
// This function runs all tasks synchronously and is used for testing and simple scenarios.
// For production use with async support, use processAllDueTasksWithExecutor.
func ProcessAllDueTasksWithControlPlane(ctx context.Context, proj *project.Project, cp *ControlPlane) {
	taskHandlers := cp.GetTaskHandlers()

	// Get the next due task globally (not work-specific)
	for {
		task, err := proj.DB.GetNextScheduledTask(ctx)
		if err != nil {
			logging.Warn("failed to get next scheduled task", "error", err)
			return
		}

		if task == nil {
			// No more due tasks
			return
		}

		logging.Info("Executing scheduled task",
			"task_id", task.ID,
			"task_type", task.TaskType,
			"work_id", task.WorkID,
			"scheduled_at", task.ScheduledAt.Format(time.RFC3339))

		// Print to stdout
		fmt.Printf("[%s] Executing %s for %s\n", time.Now().Format("15:04:05"), task.TaskType, task.WorkID)

		// Mark as executing
		if err := proj.DB.MarkTaskExecuting(ctx, task.ID); err != nil {
			logging.Warn("failed to mark task as executing", "error", err)
			continue
		}

		// Execute based on task type
		if handler, ok := taskHandlers[task.TaskType]; ok {
			err = handler(ctx, proj, task)
		} else {
			err = fmt.Errorf("unknown task type: %s", task.TaskType)
		}

		// Handle task result
		if err != nil {
			fmt.Printf("[%s] Task failed: %s\n", time.Now().Format("15:04:05"), err)
			HandleTaskError(ctx, proj, task, err.Error())
		} else {
			fmt.Printf("[%s] Task completed: %s\n", time.Now().Format("15:04:05"), task.TaskType)
			if err := proj.DB.MarkTaskCompleted(ctx, task.ID); err != nil {
				logging.Warn("failed to mark task as completed", "error", err, "task_id", task.ID)
			}
		}
	}
}

// processAllDueTasksWithExecutor checks for and executes scheduled tasks, using the async executor
// for long-running tasks. Sync tasks run directly, while async tasks are submitted to the executor
// to run in goroutines without blocking the main loop.
func processAllDueTasksWithExecutor(ctx context.Context, proj *project.Project, cp *ControlPlane, executor *AsyncTaskExecutor) {
	taskHandlers := cp.GetTaskHandlers()

	// Get the next due task globally (not work-specific)
	for {
		task, err := proj.DB.GetNextScheduledTask(ctx)
		if err != nil {
			logging.Warn("failed to get next scheduled task", "error", err)
			return
		}

		if task == nil {
			// No more due tasks
			return
		}

		logging.Info("Processing scheduled task",
			"task_id", task.ID,
			"task_type", task.TaskType,
			"work_id", task.WorkID,
			"is_async", IsAsyncTask(task.TaskType),
			"scheduled_at", task.ScheduledAt.Format(time.RFC3339))

		// Mark as executing before dispatching
		if err := proj.DB.MarkTaskExecuting(ctx, task.ID); err != nil {
			logging.Warn("failed to mark task as executing", "error", err)
			continue
		}

		// Check if this is an async task
		if IsAsyncTask(task.TaskType) {
			// Submit to async executor (non-blocking)
			if !executor.Submit(ctx, task) {
				// Task already running, it was marked executing but we couldn't submit
				// This shouldn't happen normally, but handle it gracefully
				logging.Warn("failed to submit async task (already running?)",
					"task_id", task.ID,
					"task_type", task.TaskType)
			}
			// Continue processing other tasks immediately
			continue
		}

		// Sync task: execute directly
		fmt.Printf("[%s] Executing %s for %s\n", time.Now().Format("15:04:05"), task.TaskType, task.WorkID)

		var execErr error
		if handler, ok := taskHandlers[task.TaskType]; ok {
			execErr = handler(ctx, proj, task)
		} else {
			execErr = fmt.Errorf("unknown task type: %s", task.TaskType)
		}

		// Handle task result
		if execErr != nil {
			fmt.Printf("[%s] Task failed: %s\n", time.Now().Format("15:04:05"), execErr)
			HandleTaskError(ctx, proj, task, execErr.Error())
		} else {
			fmt.Printf("[%s] Task completed: %s\n", time.Now().Format("15:04:05"), task.TaskType)
			if markErr := proj.DB.MarkTaskCompleted(ctx, task.ID); markErr != nil {
				logging.Warn("failed to mark task as completed", "error", markErr, "task_id", task.ID)
			}
		}
	}
}
