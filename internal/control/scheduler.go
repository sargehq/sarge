package control

import (
	"context"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// ScheduleDestroyWorktree schedules a worktree destruction task for the control plane.
// This is the preferred way to destroy a worktree as it runs asynchronously with retry support.
func ScheduleDestroyWorktree(ctx context.Context, proj *project.Project, workID string) error {
	idempotencyKey := fmt.Sprintf("destroy-worktree-%s", workID)
	err := proj.DB.ScheduleTaskWithRetry(ctx, workID, db.TaskTypeDestroyWorktree, time.Now(), nil, idempotencyKey, db.DefaultMaxAttempts)
	if err != nil {
		return fmt.Errorf("failed to schedule destroy worktree task: %w", err)
	}
	logging.Info("Scheduled destroy worktree task", "work_id", workID)
	return nil
}

// TriggerPRFeedbackCheck schedules an immediate PR feedback check.
// This is called from the TUI when the user presses 'f' in the work details panel.
func TriggerPRFeedbackCheck(ctx context.Context, proj *project.Project, workID string) error {
	logging.Debug("triggering immediate PR feedback check", "work_id", workID)

	// Schedule the task to run now
	_, err := proj.DB.TriggerTaskNow(ctx, workID, db.TaskTypePRFeedback)
	if err != nil {
		return fmt.Errorf("failed to trigger PR feedback check: %w", err)
	}

	logging.Debug("PR feedback check scheduled", "work_id", workID)
	return nil
}
