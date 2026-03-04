package control

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleCreateWorktreeTask handles a scheduled worktree creation task
func (cp *ControlPlane) HandleCreateWorktreeTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	branchName := task.Metadata["branch"]
	baseBranch := task.Metadata["base_branch"]
	useExisting := task.Metadata["use_existing"] == "true"

	if baseBranch == "" {
		baseBranch = proj.Config.Repo.GetBaseBranch()
	}

	logging.Info("Creating worktree for work",
		"work_id", workID,
		"branch", branchName,
		"base_branch", baseBranch,
		"use_existing", useExisting,
		"attempt", task.AttemptCount+1)

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		// Work was deleted - nothing to do
		logging.Info("Work not found, task will be marked completed", "work_id", workID)
		return nil
	}

	mainRepoPath := proj.MainRepoPath()
	var branchExistsOnRemote bool

	// For existing branches, check if branch exists on remote
	if useExisting {
		_, branchExistsOnRemote, _ = cp.Git.ValidateExistingBranch(ctx, mainRepoPath, branchName)
	}

	// If worktree path is already set and exists, skip creation
	if work.WorktreePath != "" {
		// Worktree already created - just need to ensure git push
		logging.Info("Worktree already exists, skipping creation", "work_id", workID, "path", work.WorktreePath)
	} else {
		// Create the worktree
		workDir := filepath.Join(proj.Root, workID)
		worktreePath := filepath.Join(workDir, "tree")

		// Create work directory
		if err := os.Mkdir(workDir, 0750); err != nil && !os.IsExist(err) {
			return fmt.Errorf("failed to create work directory: %w", err)
		}

		if useExisting {
			// For existing branches, fetch the branch first
			if err := cp.Git.FetchBranch(ctx, mainRepoPath, branchName); err != nil {
				// Ignore fetch errors - branch might only exist locally
				logging.Debug("Could not fetch branch from origin (may only exist locally)", "branch", branchName)
			}

			// Create worktree from existing branch
			if err := cp.Worktree.CreateFromExisting(ctx, mainRepoPath, worktreePath, branchName); err != nil {
				_ = os.RemoveAll(workDir)
				return fmt.Errorf("failed to create worktree from existing branch: %w", err)
			}
		} else {
			// Fetch latest from origin for the base branch
			if err := cp.Git.FetchBranch(ctx, mainRepoPath, baseBranch); err != nil {
				_ = os.RemoveAll(workDir)
				return fmt.Errorf("failed to fetch base branch: %w", err)
			}

			// Create git worktree with new branch based on origin/<baseBranch>
			if err := cp.Worktree.Create(ctx, mainRepoPath, worktreePath, branchName, "origin/"+baseBranch); err != nil {
				_ = os.RemoveAll(workDir)
				return fmt.Errorf("failed to create worktree: %w", err)
			}
		}

		// Initialize mise if configured
		miseOps := cp.Mise(worktreePath)
		if err := miseOps.InitializeWithOutput(io.Discard); err != nil {
			logging.Warn("mise initialization failed", "error", err)
			// Non-fatal, continue
		}

		// Update work with worktree path
		if err := proj.DB.UpdateWorkWorktreePath(ctx, workID, worktreePath); err != nil {
			return fmt.Errorf("failed to update work worktree path: %w", err)
		}
	}

	// Attempt git push (skip for existing branches that already exist on remote)
	work, _ = proj.DB.GetWork(ctx, workID) // Refresh work
	if work != nil && work.WorktreePath != "" {
		if useExisting && branchExistsOnRemote {
			logging.Info("Skipping git push - branch already exists on remote", "branch", branchName)
		} else {
			if err := cp.Git.PushSetUpstream(ctx, branchName, work.WorktreePath); err != nil {
				return fmt.Errorf("git push failed: %w", err)
			}
		}
	}

	logging.Info("Worktree created and pushed successfully", "work_id", workID)

	// Task execution is now handled by the in-process task sequencer.
	// No need to spawn a separate orchestrator process.

	return nil
}
