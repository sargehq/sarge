package control

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/mise"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/work"
)

// HandleImportPRTask handles a scheduled PR import task.
// This sets up a worktree from an existing GitHub PR.
func HandleImportPRTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	prURL := task.Metadata["pr_url"]
	branchName := task.Metadata["branch"]

	logging.Info("Importing PR into work",
		"work_id", workID,
		"pr_url", prURL,
		"branch", branchName,
		"attempt", task.AttemptCount+1)

	// Get work details
	workRecord, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if workRecord == nil {
		// Work was deleted - nothing to do
		logging.Info("Work not found, task will be marked completed", "work_id", workID)
		return nil
	}

	// Create work service for PR operations
	workSvc := work.NewWorkService(proj)

	// If worktree path is already set and exists, skip creation
	if workRecord.WorktreePath != "" && workSvc.Worktree.ExistsPath(workRecord.WorktreePath) {
		logging.Info("Worktree already exists, skipping creation", "work_id", workID, "path", workRecord.WorktreePath)
		return nil
	}

	mainRepoPath := proj.MainRepoPath()

	// Create work subdirectory
	workDir := filepath.Join(proj.Root, workID)
	if err := os.Mkdir(workDir, 0750); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// Set up worktree from PR using the work service
	_, worktreePath, err := workSvc.SetupWorktreeFromPR(ctx, mainRepoPath, prURL, "", workDir, branchName)
	if err != nil {
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("failed to set up worktree from PR: %w", err)
	}

	// Set up upstream tracking
	if err := workSvc.Git.PushSetUpstream(ctx, branchName, worktreePath); err != nil {
		_ = workSvc.Worktree.RemoveForce(ctx, mainRepoPath, worktreePath)
		_ = os.RemoveAll(workDir)
		return fmt.Errorf("failed to set upstream: %w", err)
	}

	// Initialize mise if configured (output discarded)
	if err := mise.InitializeWithOutput(worktreePath, io.Discard); err != nil {
		logging.Warn("mise initialization failed", "error", err)
		// Non-fatal, continue
	}

	// Update work with worktree path
	if err := proj.DB.UpdateWorkWorktreePath(ctx, workID, worktreePath); err != nil {
		return fmt.Errorf("failed to update work worktree path: %w", err)
	}

	// Add root issue to work_beans if set and not already added
	// (ImportPRAsync now adds beans immediately, so this is a fallback)
	if workRecord.RootIssueID != "" {
		workBeans, err := proj.DB.GetWorkBeans(ctx, workID)
		if err != nil {
			logging.Warn("failed to get work beans", "error", err)
		} else {
			beanExists := false
			for _, wb := range workBeans {
				if wb.BeanID == workRecord.RootIssueID {
					beanExists = true
					break
				}
			}
			if !beanExists {
				if err := proj.DB.AddWorkBeans(ctx, workID, []string{workRecord.RootIssueID}); err != nil {
					logging.Warn("failed to add bean to work", "error", err, "bean_id", workRecord.RootIssueID)
				}
			}
		}
	}

	// Set PR URL on the work and schedule feedback polling
	prFeedbackInterval := proj.Config.Scheduler.GetPRFeedbackInterval()
	commentResolutionInterval := proj.Config.Scheduler.GetCommentResolutionInterval()
	if err := proj.DB.SetWorkPRURLAndScheduleFeedback(ctx, workID, prURL, prFeedbackInterval, commentResolutionInterval); err != nil {
		logging.Warn("failed to set PR URL on work", "error", err)
	}

	logging.Info("PR imported successfully", "work_id", workID, "worktree", worktreePath)

	// Schedule orchestrator spawn task (but it won't auto-start since auto=false)
	_, err = proj.DB.ScheduleTask(ctx, workID, db.TaskTypeSpawnOrchestrator, time.Now(), map[string]string{
		"worker_name": workRecord.Name,
	})
	if err != nil {
		logging.Warn("failed to schedule orchestrator spawn", "error", err, "work_id", workID)
	}

	return nil
}
