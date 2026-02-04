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

	logging.Info("Control plane started with database events")

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
			fmt.Println("\nControl plane stopped.")
			return nil

		case event, ok := <-sub:
			if !ok {
				logging.Debug("Watcher subscription closed")
				return nil
			}

			// Handle database change event
			if event.Payload.Type == trackingwatcher.DBChanged {
				logging.Debug("Database changed, checking scheduled tasks")
				ProcessAllDueTasksWithControlPlane(ctx, proj, cp)
			}

		case <-checkTimer.C:
			// Periodic check as a safety net
			logging.Debug("Control plane periodic check")
			ProcessAllDueTasksWithControlPlane(ctx, proj, cp)
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
