package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/agent"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/worktree"
)

// TaskPrompt contains the prompt and any associated resources for a task.
type TaskPrompt struct {
	Prompt       string
	TempFilePath string // Path to temp file that should be cleaned up after task execution
}

// buildPromptForTask builds the appropriate prompt for a task based on its type.
// This centralizes prompt building logic for different task types.
// Returns a TaskPrompt which includes the prompt and any temp files that need cleanup.
func buildPromptForTask(ctx context.Context, proj *project.Project, task *db.Task, work *db.Work) (*TaskPrompt, error) {
	baseBranch := work.BaseBranch
	if baseBranch == "" {
		baseBranch = proj.Config.Repo.GetBaseBranch()
	}
	agentType := agent.AgentTypeFromConfig(proj.Config)

	switch task.TaskType {
	case "estimate":
		issues, err := getBeadsForTask(ctx, proj, task.ID)
		if err != nil {
			return nil, err
		}
		return &TaskPrompt{Prompt: agent.BuildEstimatePrompt(task.ID, issues, agentType)}, nil

	case "implement":
		issues, err := getBeadsForTask(ctx, proj, task.ID)
		if err != nil {
			return nil, err
		}
		return &TaskPrompt{Prompt: agent.BuildTaskPrompt(task.ID, issues, work.BranchName, baseBranch, agentType)}, nil

	case "review":
		return &TaskPrompt{Prompt: agent.BuildReviewPrompt(task.ID, work.ID, work.BranchName, baseBranch, work.RootIssueID, agentType)}, nil

	case "pr":
		return &TaskPrompt{Prompt: agent.BuildPRPrompt(task.ID, work.ID, work.BranchName, baseBranch, agentType)}, nil

	case "update-pr-description":
		if work.PRURL == "" {
			return nil, fmt.Errorf("work %s has no PR URL set", work.ID)
		}
		return &TaskPrompt{Prompt: agent.BuildUpdatePRDescriptionPrompt(task.ID, work.ID, work.PRURL, work.BranchName, baseBranch, agentType)}, nil

	case "log_analysis":
		// Log analysis tasks have metadata with log content stored by the feedback processor
		return buildLogAnalysisPromptFromMetadata(ctx, proj, task, work)

	default:
		return nil, fmt.Errorf("unknown task type: %s", task.TaskType)
	}
}

// buildLogAnalysisPromptFromMetadata builds a log analysis prompt from task metadata.
// The metadata is stored by the feedback processor when creating log_analysis tasks.
// Returns a TaskPrompt including the path to the temp file that should be cleaned up.
func buildLogAnalysisPromptFromMetadata(ctx context.Context, proj *project.Project, task *db.Task, work *db.Work) (*TaskPrompt, error) {
	// Retrieve metadata stored by the feedback processor
	workflowName, err := proj.DB.GetTaskMetadata(ctx, task.ID, "workflow_name")
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow_name metadata: %w", err)
	}

	jobName, err := proj.DB.GetTaskMetadata(ctx, task.ID, "job_name")
	if err != nil {
		return nil, fmt.Errorf("failed to get job_name metadata: %w", err)
	}

	branchName, err := proj.DB.GetTaskMetadata(ctx, task.ID, "branch_name")
	if err != nil {
		return nil, fmt.Errorf("failed to get branch_name metadata: %w", err)
	}
	if branchName == "" {
		branchName = work.BranchName
	}

	rootIssueID, err := proj.DB.GetTaskMetadata(ctx, task.ID, "root_issue_id")
	if err != nil {
		return nil, fmt.Errorf("failed to get root_issue_id metadata: %w", err)
	}
	if rootIssueID == "" {
		rootIssueID = work.RootIssueID
	}

	logContent, err := proj.DB.GetTaskMetadata(ctx, task.ID, "log_content")
	if err != nil {
		return nil, fmt.Errorf("failed to get log_content metadata: %w", err)
	}
	if logContent == "" {
		return nil, fmt.Errorf("log_content metadata is missing for task %s", task.ID)
	}

	// Write log content to a temp file for Claude to read
	// This keeps the prompt small and lets Claude read only what it needs
	logFile, err := os.CreateTemp("", "ci-log-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file for log content: %w", err)
	}
	if _, err := logFile.WriteString(logContent); err != nil {
		_ = logFile.Close()
		_ = os.Remove(logFile.Name())
		return nil, fmt.Errorf("failed to write log content to temp file: %w", err)
	}
	_ = logFile.Close()

	// Fetch existing open beads for this work to help Claude match against them
	existingBeads := fetchExistingBeadSummaries(ctx, proj, work.ID)

	params := agent.LogAnalysisParams{
		TaskID:        task.ID,
		WorkID:        work.ID,
		BranchName:    branchName,
		RootIssueID:   rootIssueID,
		WorkflowName:  workflowName,
		JobName:       jobName,
		LogFilePath:   logFile.Name(),
		ExistingBeads: existingBeads,
	}

	agentType := agent.AgentTypeFromConfig(proj.Config)
	return &TaskPrompt{
		Prompt:       agent.BuildLogAnalysisPrompt(params, agentType),
		TempFilePath: logFile.Name(),
	}, nil
}

// fetchExistingBeadSummaries fetches open beads for the given work and converts them to summaries for matching.
func fetchExistingBeadSummaries(ctx context.Context, proj *project.Project, workID string) []agent.BeadSummary {
	// Get bead IDs assigned to this work
	workBeads, err := proj.DB.GetWorkBeads(ctx, workID)
	if err != nil {
		fmt.Printf("Warning: failed to fetch work beads for deduplication: %v\n", err)
		return nil
	}

	if len(workBeads) == 0 {
		return nil
	}

	// Extract bead IDs
	beadIDs := make([]string, len(workBeads))
	for i, wb := range workBeads {
		beadIDs[i] = wb.BeadID
	}

	// Fetch bead details from beads database
	result, err := proj.Beads.GetBeadsWithDeps(ctx, beadIDs)
	if err != nil {
		fmt.Printf("Warning: failed to fetch bead details for deduplication: %v\n", err)
		return nil
	}

	// Filter to only open beads and convert to summaries
	summaries := make([]agent.BeadSummary, 0, len(beadIDs))
	for _, beadID := range beadIDs {
		if bwd := result.GetBead(beadID); bwd != nil && bwd.Bead.Status == beads.StatusOpen {
			summaries = append(summaries, agent.BeadSummary{
				ID:          bwd.Bead.ID,
				Title:       bwd.Bead.Title,
				Description: bwd.Bead.Description,
			})
		}
	}
	return summaries
}

// getBeadsForTask retrieves the beads associated with a task.
func getBeadsForTask(ctx context.Context, proj *project.Project, taskID string) ([]beads.Bead, error) {
	beadIDs, err := proj.DB.GetTaskBeads(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads: %w", err)
	}

	// Get beads with dependencies
	result, err := proj.Beads.GetBeadsWithDeps(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beads: %w", err)
	}

	// Convert map to slice in order of beadIDs
	var beadList []beads.Bead
	for _, beadID := range beadIDs {
		if b, ok := result.Beads[beadID]; ok {
			beadList = append(beadList, b)
		} else {
			fmt.Printf("Warning: bead %s not found\n", beadID)
		}
	}

	return beadList, nil
}

// processTask processes a single task by ID using inline execution.
// This blocks until the task is complete.
func processTask(proj *project.Project, taskID string, runner agent.Runner) error {
	ctx := GetContext()

	// Get the task
	dbTask, err := proj.DB.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if dbTask == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Check task status
	if dbTask.Status == db.StatusCompleted {
		fmt.Printf("Task %s is already completed\n", taskID)
		return nil
	}

	// Get the associated work
	if dbTask.WorkID == "" {
		return fmt.Errorf("task %s has no associated work", taskID)
	}

	work, err := proj.DB.GetWork(ctx, dbTask.WorkID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found for task %s", dbTask.WorkID, taskID)
	}

	// Transition work to processing if needed
	switch work.Status {
	case db.StatusPending:
		// First time starting - use StartWork
		if err := proj.DB.StartWork(ctx, work.ID, "", ""); err != nil {
			fmt.Printf("Warning: failed to start work: %v\n", err)
		}
	case db.StatusIdle, db.StatusCompleted:
		// Work was idle/completed but new task is starting - resume it
		if err := proj.DB.ResumeWork(ctx, work.ID); err != nil {
			fmt.Printf("Warning: failed to resume work: %v\n", err)
		}
	case db.StatusFailed:
		// Work is failed - user needs to explicitly restart
		return fmt.Errorf("work %s is in failed state - use 'sarge work restart %s' to resume", work.ID, work.ID)
	}

	fmt.Printf("\n=== Processing task %s ===\n", taskID)
	fmt.Printf("Work: %s\n", work.ID)
	fmt.Printf("Branch: %s\n", work.BranchName)
	fmt.Printf("Worktree: %s\n", work.WorktreePath)

	// Check if worktree exists
	if work.WorktreePath == "" {
		return fmt.Errorf("work %s has no worktree path configured", work.ID)
	}

	if !worktree.NewOperations().ExistsPath(work.WorktreePath) {
		return fmt.Errorf("work %s worktree does not exist at %s", work.ID, work.WorktreePath)
	}

	// Build prompt for Claude based on task type
	taskPrompt, err := buildPromptForTask(ctx, proj, dbTask, work)
	if err != nil {
		return err
	}

	// Clean up temp file after execution (if any)
	if taskPrompt.TempFilePath != "" {
		defer func() { _ = os.Remove(taskPrompt.TempFilePath) }()
	}

	// Execute Claude inline (blocking)
	if err := runner.Run(ctx, proj.DB, taskID, taskPrompt.Prompt, work.WorktreePath, proj.Config); err != nil {
		return fmt.Errorf("task %s failed: %w", taskID, err)
	}

	fmt.Printf("\n=== Task %s completed ===\n", taskID)
	return nil
}
