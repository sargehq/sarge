package control

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleGitPushTask handles a scheduled git push task with retry support.
// Returns nil on success, error on failure (caller handles retry/completion).
func (cp *ControlPlane) HandleGitPushTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	// Get branch and directory from metadata
	branch := task.Metadata["branch"]
	dir := task.Metadata["dir"]

	if branch == "" {
		// Try to get from work
		work, err := proj.DB.GetWork(ctx, workID)
		if err != nil || work == nil {
			return fmt.Errorf("failed to get work for git push: work not found")
		}
		branch = work.BranchName
		dir = work.WorktreePath
	}

	if branch == "" || dir == "" {
		return fmt.Errorf("git push task missing branch or dir metadata")
	}

	logging.Info("Executing git push", "branch", branch, "dir", dir, "attempt", task.AttemptCount+1)

	if err := cp.Git.PushSetUpstream(ctx, branch, dir); err != nil {
		return err
	}

	logging.Info("Git push succeeded", "branch", branch, "work_id", workID)

	return nil
}
