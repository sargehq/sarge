package control

import (
	"context"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleCommentResolutionTask handles a scheduled comment resolution check.
// Returns nil on success, error on failure (caller handles retry/completion).
func HandleCommentResolutionTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil || work == nil || work.PRURL == "" {
		// Don't reschedule - scheduling happens when PR is created
		return nil
	}

	// Check and resolve comments
	if err := feedback.CheckAndResolveComments(ctx, proj, workID); err != nil {
		return fmt.Errorf("failed to check and resolve comments: %w", err)
	}

	// Schedule next check using configured interval
	nextInterval := proj.Config.Scheduler.GetCommentResolutionInterval()
	nextCheck := time.Now().Add(nextInterval)
	_, err = proj.DB.ScheduleOrUpdateTask(ctx, workID, db.TaskTypeCommentResolution, nextCheck)
	if err != nil {
		logging.Warn("failed to schedule next comment resolution check", "error", err)
	}

	return nil
}
