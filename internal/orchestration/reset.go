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
// This function preserves partial bead progress by checking the actual bead
// status in beads.jsonl before resetting. Beads that are already closed are
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

			// Preserve partial bead progress by checking actual bead status
			preservedCount, resetBeadCount, err := ResetTaskBeadsWithProgress(ctx, proj, t.ID, workID)
			if err != nil {
				return fmt.Errorf("failed to reset task beads for %s: %w", t.ID, err)
			}

			if preservedCount > 0 {
				fmt.Printf("  Preserved %d already-completed bead(s), reset %d bead(s)\n", preservedCount, resetBeadCount)
				logging.Info("preserved partial bead progress during task reset",
					"task_id", t.ID,
					"preserved_count", preservedCount,
					"reset_count", resetBeadCount,
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
				"preserved_beads", preservedCount,
				"reset_beads", resetBeadCount,
			)

			resetCount++
		}
	}

	if resetCount > 0 {
		fmt.Printf("Reset %d stuck task(s)\n", resetCount)
	}

	return nil
}

// ResetTaskBeadsWithProgress resets task bead statuses while preserving progress.
// It checks the actual bead status in beads.jsonl and only resets beads that
// are not already closed. Returns (preserved count, reset count, error).
// Also logs recovery events for audit trail.
func ResetTaskBeadsWithProgress(ctx context.Context, proj *project.Project, taskID, workID string) (int, int, error) {
	// Get all beads in this task with their current status
	taskBeads, err := proj.DB.GetTaskBeansWithStatus(ctx, taskID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get task beads: %w", err)
	}

	if len(taskBeads) == 0 {
		return 0, 0, nil
	}

	// Collect bead IDs to check their actual status
	beanIDs := make([]string, len(taskBeads))
	for i, tb := range taskBeads {
		beanIDs[i] = tb.BeanID
	}

	// Get actual bead status from beads.jsonl
	beadsResult, err := proj.Beads.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		// If we can't check bead status, fall back to resetting all
		logging.Warn("could not check bead status, falling back to full reset",
			"task_id", taskID,
			"error", err,
		)
		if err := proj.DB.ResetTaskBeanStatuses(ctx, taskID); err != nil {
			return 0, 0, err
		}
		return 0, len(taskBeads), nil
	}

	preservedCount := 0
	resetCount := 0

	for _, tb := range taskBeads {
		// Check if the bead is closed in beads.jsonl
		actualBead, found := beadsResult.Beans[tb.BeanID]
		if found && actualBead.Status == beans.StatusCompleted {
			// Bead is closed in beads.jsonl - mark it as completed in task_beads
			if tb.Status != db.StatusCompleted {
				if err := proj.DB.CompleteTaskBean(ctx, taskID, tb.BeanID); err != nil {
					logging.Warn("failed to mark closed bead as completed",
						"task_id", taskID,
						"bead_id", tb.BeanID,
						"error", err,
					)
				} else {
					preservedCount++
					logging.Debug("bead already closed in beads.jsonl, preserving completed status",
						"event_type", "bead_preserved",
						"task_id", taskID,
						"work_id", workID,
						"bead_id", tb.BeanID,
						"previous_task_status", tb.Status,
					)
				}
			} else {
				// Already marked as completed
				preservedCount++
			}
		} else {
			// Bead is not closed - reset to pending
			if tb.Status != db.StatusPending {
				if err := proj.DB.ResetTaskBeanStatus(ctx, taskID, tb.BeanID); err != nil {
					logging.Warn("failed to reset bead status",
						"task_id", taskID,
						"bead_id", tb.BeanID,
						"error", err,
					)
				} else {
					resetCount++
					logging.Debug("bead not closed, resetting to pending",
						"event_type", "bead_reset",
						"task_id", taskID,
						"work_id", workID,
						"bead_id", tb.BeanID,
						"previous_task_status", tb.Status,
					)
				}
			}
		}
	}

	return preservedCount, resetCount, nil
}
