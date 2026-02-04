package work

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sargehq/sarge/internal/db"
)

// AddBeadsToWorkResult contains the result of adding beads to a work.
type AddBeadsToWorkResult struct {
	BeadsAdded int
}

// RemoveBeadsResult contains the result of removing beads from a work.
type RemoveBeadsResult struct {
	BeadsRemoved int
}

// CreateWorkAsyncResult contains the result of creating a work unit asynchronously.
type CreateWorkAsyncResult struct {
	WorkID      string
	WorkerName  string
	BranchName  string
	BaseBranch  string
	RootIssueID string
}

// CreateWorkAsyncOptions contains options for creating a work unit asynchronously.
type CreateWorkAsyncOptions struct {
	BranchName        string
	BaseBranch        string
	RootIssueID       string
	Auto              bool
	UseExistingBranch bool
	BeadIDs           []string // Beads to add to the work (added immediately, not by control plane)
}

// CreateWorkFromBeadOptions contains options for creating a work from a bead.
// This is the high-level API that handles bead expansion, work creation, and control plane initialization.
type CreateWorkFromBeadOptions struct {
	BeadID            string // Root bead ID to create work from
	BranchName        string
	BaseBranch        string
	Auto              bool
	UseExistingBranch bool
}

// CreateWorkFromBeadResult contains the result of creating a work from a bead.
type CreateWorkFromBeadResult struct {
	WorkID     string
	WorkerName string
	BranchName string
	BaseBranch string
	BeadIDs    []string // All expanded bead IDs
}

// CreateWorkFromBead creates a work unit from a bead, handling all common steps:
// 1. Expands the bead to collect all issue IDs (epics, transitive deps)
// 2. Creates the work asynchronously via CreateWorkAsyncWithOptions
//
// This is the shared implementation used by both CLI and TUI.
// Callers are responsible for ensuring the control plane is running via control.EnsureControlPlane.
func (s *WorkService) CreateWorkFromBead(ctx context.Context, opts CreateWorkFromBeadOptions) (*CreateWorkFromBeadResult, error) {
	// 1. Collect issue IDs (handles epics and transitive deps)
	allIssueIDs, err := CollectIssueIDsForAutomatedWorkflow(ctx, opts.BeadID, s.BeadsReader)
	if err != nil {
		return nil, fmt.Errorf("failed to expand bead %s: %w", opts.BeadID, err)
	}
	if len(allIssueIDs) == 0 {
		return nil, fmt.Errorf("no beads found for %s", opts.BeadID)
	}

	// 2. Create work asynchronously (DB operations + schedule control plane task)
	createOpts := CreateWorkAsyncOptions{
		BranchName:        opts.BranchName,
		BaseBranch:        opts.BaseBranch,
		RootIssueID:       opts.BeadID,
		Auto:              opts.Auto,
		UseExistingBranch: opts.UseExistingBranch,
		BeadIDs:           allIssueIDs,
	}
	result, err := s.CreateWorkAsyncWithOptions(ctx, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create work: %w", err)
	}

	return &CreateWorkFromBeadResult{
		WorkID:     result.WorkID,
		WorkerName: result.WorkerName,
		BranchName: result.BranchName,
		BaseBranch: result.BaseBranch,
		BeadIDs:    allIssueIDs,
	}, nil
}

// ImportPRAsyncOptions contains options for importing a PR asynchronously.
type ImportPRAsyncOptions struct {
	PRURL       string
	BranchName  string
	RootIssueID string
}

// ImportPRAsyncResult contains the result of scheduling a PR import.
type ImportPRAsyncResult struct {
	WorkID      string
	WorkerName  string
	BranchName  string
	RootIssueID string
}

// ImportPRAsync imports a PR asynchronously by scheduling a control plane task.
// This creates the work record and schedules the import task - the actual
// worktree setup happens in the control plane.
func (s *WorkService) ImportPRAsync(ctx context.Context, opts ImportPRAsyncOptions) (*ImportPRAsyncResult, error) {
	baseBranch := s.Config.Repo.GetBaseBranch()

	// Generate work ID
	workID, err := s.DB.GenerateWorkID(ctx, opts.BranchName, s.Config.Project.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate work ID: %w", err)
	}

	// Get a human-readable name for this worker
	workerName, err := s.NameGenerator.GetNextAvailableName(ctx, s.DB.DB)
	if err != nil {
		workerName = "" // Non-fatal
	}

	// Create work record in DB (without worktree path - control plane will set it)
	if err := s.DB.CreateWork(ctx, workID, workerName, "", opts.BranchName, baseBranch, opts.RootIssueID, false); err != nil {
		return nil, fmt.Errorf("failed to create work record: %w", err)
	}

	// Add root issue to work_beads immediately (before control plane runs)
	if opts.RootIssueID != "" {
		if err := s.addBeadsInternal(ctx, workID, []string{opts.RootIssueID}); err != nil {
			_ = s.DB.DeleteWork(ctx, workID)
			return nil, fmt.Errorf("failed to add bead to work: %w", err)
		}
	}

	// Schedule the PR import task for the control plane
	err = s.DB.ScheduleTaskWithRetry(ctx, workID, db.TaskTypeImportPR, time.Now(), map[string]string{
		"pr_url":      opts.PRURL,
		"branch":      opts.BranchName,
		"worker_name": workerName,
	}, fmt.Sprintf("import-pr-%s", workID), db.DefaultMaxAttempts)
	if err != nil {
		// Work record created but task scheduling failed - cleanup
		_ = s.DB.DeleteWork(ctx, workID)
		return nil, fmt.Errorf("failed to schedule PR import: %w", err)
	}

	return &ImportPRAsyncResult{
		WorkID:      workID,
		WorkerName:  workerName,
		BranchName:  opts.BranchName,
		RootIssueID: opts.RootIssueID,
	}, nil
}

// CreateWorkAsyncWithOptions creates a work unit asynchronously by scheduling tasks.
// This is the async work creation for the control plane architecture:
// 1. Creates work record in DB (without worktree path)
// 2. Schedules TaskTypeCreateWorktree task for the control plane
// The control plane will handle worktree creation, git push, and orchestrator spawning.
func (s *WorkService) CreateWorkAsyncWithOptions(ctx context.Context, opts CreateWorkAsyncOptions) (*CreateWorkAsyncResult, error) {
	baseBranch := opts.BaseBranch
	if baseBranch == "" {
		baseBranch = s.Config.Repo.GetBaseBranch()
	}

	branchName := opts.BranchName

	// For new branches, ensure unique name; for existing branches, use as-is
	if !opts.UseExistingBranch {
		var err error
		branchName, err = EnsureUniqueBranchName(ctx, s.Git, s.MainRepoPath, branchName)
		if err != nil {
			return nil, fmt.Errorf("failed to find unique branch name: %w", err)
		}
	}

	// Generate work ID
	workID, err := s.DB.GenerateWorkID(ctx, branchName, s.Config.Project.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate work ID: %w", err)
	}

	// Get a human-readable name for this worker
	workerName, err := s.NameGenerator.GetNextAvailableName(ctx, s.DB.DB)
	if err != nil {
		workerName = "" // Non-fatal
	}

	// Create work record in DB (without worktree path - control plane will set it)
	if err := s.DB.CreateWork(ctx, workID, workerName, "", branchName, baseBranch, opts.RootIssueID, opts.Auto); err != nil {
		return nil, fmt.Errorf("failed to create work record: %w", err)
	}

	// Add beads to work_beads (done immediately, not by control plane)
	if len(opts.BeadIDs) > 0 {
		if err := s.addBeadsInternal(ctx, workID, opts.BeadIDs); err != nil {
			_ = s.DB.DeleteWork(ctx, workID)
			return nil, fmt.Errorf("failed to add beads to work: %w", err)
		}
	}

	// Schedule the worktree creation task for the control plane
	autoStr := "false"
	if opts.Auto {
		autoStr = "true"
	}
	useExistingStr := "false"
	if opts.UseExistingBranch {
		useExistingStr = "true"
	}
	err = s.DB.ScheduleTaskWithRetry(ctx, workID, db.TaskTypeCreateWorktree, time.Now(), map[string]string{
		"branch":        branchName,
		"base_branch":   baseBranch,
		"root_issue_id": opts.RootIssueID,
		"worker_name":   workerName,
		"auto":          autoStr,
		"use_existing":  useExistingStr,
	}, fmt.Sprintf("create-worktree-%s", workID), db.DefaultMaxAttempts)
	if err != nil {
		// Work record created but task scheduling failed - cleanup
		_ = s.DB.DeleteWork(ctx, workID)
		return nil, fmt.Errorf("failed to schedule worktree creation: %w", err)
	}

	return &CreateWorkAsyncResult{
		WorkID:      workID,
		WorkerName:  workerName,
		BranchName:  branchName,
		BaseBranch:  baseBranch,
		RootIssueID: opts.RootIssueID,
	}, nil
}

// addBeadsInternal adds beads to work_beads table without validation.
func (s *WorkService) addBeadsInternal(ctx context.Context, workID string, beadIDs []string) error {
	if len(beadIDs) == 0 {
		return nil
	}
	if err := s.DB.AddWorkBeads(ctx, workID, beadIDs); err != nil {
		return fmt.Errorf("failed to add beads: %w", err)
	}
	return nil
}

// AddBeads adds beads to an existing work.
// This is the core logic for adding beads that can be called from both the CLI and TUI.
// Each bead is added as its own group (no grouping).
func (s *WorkService) AddBeads(ctx context.Context, workID string, beadIDs []string) (*AddBeadsToWorkResult, error) {
	if len(beadIDs) == 0 {
		return nil, fmt.Errorf("no beads specified")
	}

	// Verify work exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Check if any bead is already in a task
	for _, beadID := range beadIDs {
		inTask, err := s.DB.IsBeadInTask(ctx, workID, beadID)
		if err != nil {
			return nil, fmt.Errorf("failed to check bead %s: %w", beadID, err)
		}
		if inTask {
			return nil, fmt.Errorf("bead %s is already assigned to a task", beadID)
		}
	}

	// Add beads to work
	if err := s.DB.AddWorkBeads(ctx, workID, beadIDs); err != nil {
		return nil, fmt.Errorf("failed to add beads: %w", err)
	}

	return &AddBeadsToWorkResult{
		BeadsAdded: len(beadIDs),
	}, nil
}

// RemoveBeads removes beads from an existing work.
// Beads that are already assigned to a task cannot be removed.
func (s *WorkService) RemoveBeads(ctx context.Context, workID string, beadIDs []string) (*RemoveBeadsResult, error) {
	if len(beadIDs) == 0 {
		return nil, fmt.Errorf("no beads specified")
	}

	// Verify work exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Check if any bead is assigned to a task and remove those that aren't
	removed := 0
	for _, beadID := range beadIDs {
		inTask, err := s.DB.IsBeadInTask(ctx, workID, beadID)
		if err != nil {
			return nil, fmt.Errorf("failed to check bead %s: %w", beadID, err)
		}
		if inTask {
			return nil, fmt.Errorf("bead %s is assigned to a task and cannot be removed", beadID)
		}

		// Remove the bead
		if err := s.DB.RemoveWorkBead(ctx, workID, beadID); err != nil {
			return nil, fmt.Errorf("failed to remove bead %s: %w", beadID, err)
		}
		removed++
	}

	return &RemoveBeadsResult{
		BeadsRemoved: removed,
	}, nil
}

// DestroyWork destroys a work unit and all its resources.
// This is the core work destruction logic that can be called from both the CLI and TUI.
// It does not perform interactive confirmation - that should be handled by the caller.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (s *WorkService) DestroyWork(ctx context.Context, workID string, w io.Writer) error {
	// Get work to verify it exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	// Close the root issue if it exists
	if work.RootIssueID != "" {
		fmt.Fprintf(w, "Closing root issue %s...\n", work.RootIssueID)
		if err := s.BeadsCLI.Close(ctx, work.RootIssueID); err != nil {
			// Warn but continue - issue might already be closed or deleted
			fmt.Fprintf(w, "Warning: failed to close root issue %s: %v\n", work.RootIssueID, err)
		}
	}

	// Terminate any running zellij tabs (orchestrator, task, console, and claude tabs) for this work
	// Only if configured to do so (defaults to true)
	if s.Config.Zellij.ShouldKillTabsOnDestroy() {
		if err := s.OrchestratorManager.TerminateWorkTabs(ctx, workID, s.Config.Project.Name, w); err != nil {
			// Warn but continue - tab termination is non-fatal
			fmt.Fprintf(w, "Warning: failed to terminate work tabs: %v\n", err)
		}
	}

	// Remove git worktree if it exists
	if work.WorktreePath != "" {
		if err := s.Worktree.RemoveForce(ctx, s.MainRepoPath, work.WorktreePath); err != nil {
			fmt.Fprintf(w, "Warning: failed to remove worktree: %v\n", err)
		}
	}

	// Remove work directory
	workDir := filepath.Join(s.ProjectRoot, workID)
	if err := os.RemoveAll(workDir); err != nil {
		fmt.Fprintf(w, "Warning: failed to remove work directory %s: %v\n", workDir, err)
	}

	// Delete work from database (also deletes associated tasks and relationships)
	if err := s.DB.DeleteWork(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete work from database: %w", err)
	}

	return nil
}
