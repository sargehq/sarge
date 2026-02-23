package orchestration

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// ResetStuckProcessingTasks resets any processing tasks back to pending.
// This is called when the orchestrator starts and finds tasks that were
// marked as processing from a previous run. When the orchestrator is killed
// while a task is running - the Claude process is also killed, but the task
// remains marked as processing in the database.
//
// This function preserves partial bean progress by checking the actual bean
// status in beans.jsonl before resetting. Beans that are already closed are
// marked as completed in the task, not reset to pending.
func ResetStuckProcessingTasks(ctx context.Context, proj *project.Project, workID string) error {
	// Get all tasks for this work
	tasks, err := proj.DB.GetWorkTasks(ctx, workID)
	if err != nil {
		return err
	}

	resetCount := 0
	for _, t := range tasks {
		if t.Status == db.StatusProcessing {
			fmt.Printf("Resetting stuck task %s from processing to pending...\n", t.ID)

			// Preserve partial bean progress by checking actual bean status
			preservedCount, resetBeanCount, err := ResetTaskBeansWithProgress(ctx, proj, t.ID, workID)
			if err != nil {
				return fmt.Errorf("failed to reset task beans for %s: %w", t.ID, err)
			}

			if preservedCount > 0 {
				fmt.Printf("  Preserved %d already-completed bean(s), reset %d bean(s)\n", preservedCount, resetBeanCount)
				logging.Info("preserved partial bean progress during task reset",
					"task_id", t.ID,
					"preserved_count", preservedCount,
					"reset_count", resetBeanCount,
				)
			}

			if err := proj.DB.ResetTaskStatus(ctx, t.ID); err != nil {
				return fmt.Errorf("failed to reset task %s: %w", t.ID, err)
			}

			// Log task reset event
			logging.Debug("task reset from processing to pending on orchestrator startup",
				"event_type", "task_reset",
				"task_id", t.ID,
				"work_id", workID,
				"preserved_beans", preservedCount,
				"reset_beans", resetBeanCount,
			)

			resetCount++
		}
	}

	if resetCount > 0 {
		fmt.Printf("Reset %d stuck task(s)\n", resetCount)
	}

	return nil
}

// ResetTaskBeansWithProgress resets task bean statuses while preserving progress.
// It checks the actual bean status in beans.jsonl and only resets beans that
// are not already closed. Returns (preserved count, reset count, error).
// Also logs recovery events for audit trail.
func ResetTaskBeansWithProgress(ctx context.Context, proj *project.Project, taskID, workID string) (int, int, error) {
	// Get all beans in this task with their current status
	taskBeans, err := proj.DB.GetTaskBeansWithStatus(ctx, taskID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get task beans: %w", err)
	}

	if len(taskBeans) == 0 {
		return 0, 0, nil
	}

	// Collect bean IDs to check their actual status
	beanIDs := make([]string, len(taskBeans))
	for i, tb := range taskBeans {
		beanIDs[i] = tb.BeanID
	}

	// Get actual bean status from beans.jsonl
	beansResult, err := proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		// If we can't check bean status, fall back to resetting all
		logging.Warn("could not check bean status, falling back to full reset",
			"task_id", taskID,
			"error", err,
		)
		if err := proj.DB.ResetTaskBeanStatuses(ctx, taskID); err != nil {
			return 0, 0, err
		}
		return 0, len(taskBeans), nil
	}

	preservedCount := 0
	resetCount := 0

	for _, tb := range taskBeans {
		// Check if the bean is closed in beans.jsonl
		actualBean, found := beansResult.Beans[tb.BeanID]
		if found && actualBean.Status == beans.StatusCompleted {
			// Bean is closed in beans.jsonl - mark it as completed in task_beans
			if tb.Status != db.StatusCompleted {
				if err := proj.DB.CompleteTaskBean(ctx, taskID, tb.BeanID); err != nil {
					logging.Warn("failed to mark closed bean as completed",
						"task_id", taskID,
						"bean_id", tb.BeanID,
						"error", err,
					)
				} else {
					preservedCount++
					logging.Debug("bean already closed in beans.jsonl, preserving completed status",
						"event_type", "bean_preserved",
						"task_id", taskID,
						"work_id", workID,
						"bean_id", tb.BeanID,
						"previous_task_status", tb.Status,
					)
				}
			} else {
				// Already marked as completed
				preservedCount++
			}
		} else {
			// Bean is not closed - reset to pending
			if tb.Status != db.StatusPending {
				if err := proj.DB.ResetTaskBeanStatus(ctx, taskID, tb.BeanID); err != nil {
					logging.Warn("failed to reset bean status",
						"task_id", taskID,
						"bean_id", tb.BeanID,
						"error", err,
					)
				} else {
					resetCount++
					logging.Debug("bean not closed, resetting to pending",
						"event_type", "bean_reset",
						"task_id", taskID,
						"work_id", workID,
						"bean_id", tb.BeanID,
						"previous_task_status", tb.Status,
					)
				}
			}
		}
	}

	return preservedCount, resetCount, nil
}
