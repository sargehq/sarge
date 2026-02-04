package control

import (
	"context"
	"io"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleDestroyWorktreeTask handles a scheduled worktree destruction task
func (cp *ControlPlane) HandleDestroyWorktreeTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID

	logging.Info("Destroying worktree for work",
		"work_id", workID,
		"attempt", task.AttemptCount+1)

	// Check if work still exists
	workRecord, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		logging.Error("Failed to get work record", "work_id", workID, "error", err)
		return err
	}
	if workRecord == nil {
		// Work was already deleted - nothing to do
		logging.Info("Work not found, task will be marked completed", "work_id", workID)
		return nil
	}

	logging.Debug("Starting DestroyWork",
		"work_id", workID,
		"worktree_path", workRecord.WorktreePath,
		"root_issue_id", workRecord.RootIssueID)

	// Delegate to the shared DestroyWork function
	if err := cp.WorkDestroyer.DestroyWork(ctx, workID, io.Discard); err != nil {
		logging.Error("DestroyWork failed", "work_id", workID, "error", err)
		return err
	}

	logging.Info("Worktree destroyed successfully", "work_id", workID)

	return nil
}
