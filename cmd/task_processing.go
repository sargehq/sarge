package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/sargehq/sarge/internal/agents"
	agenttypes "github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/worktree"
)

// TaskInput contains the params and any associated resources for a task.
type TaskInput struct {
	Params       agenttypes.TaskParams
	TempFilePath string // Path to temp file that should be cleaned up after task execution
}

// taskInputForTask builds the appropriate TaskInput for a task based on its type.
// This centralizes param building logic for different task types.
// Returns a TaskInput which includes the params and any temp files that need cleanup.
func taskInputForTask(ctx context.Context, proj *project.Project, task *db.Task, work *db.Work) (*TaskInput, error) {
	baseBranch := work.BaseBranch
	if baseBranch == "" {
		baseBranch = proj.Config.Repo.GetBaseBranch()
	}

	// Log analysis has its own metadata-fetching flow and temp file.
	if task.TaskType == "log_analysis" {
		return logAnalysisInputFromMetadata(ctx, proj, task, work)
	}

	params := agenttypes.TaskParams{
		TaskID:     task.ID,
		WorkID:     work.ID,
		BranchName: work.BranchName,
		BaseBranch: baseBranch,
		BeansPath:  proj.BeansPath(),
	}

	switch task.TaskType {
	case "estimate":
		beanIDs, err := beanIDsForTask(ctx, proj, task.ID)
		if err != nil {
			return nil, err
		}
		params.Type = agenttypes.TaskTypeEstimate
		params.BeanIDs = beanIDs

	case "implement":
		beanIDs, err := beanIDsForTask(ctx, proj, task.ID)
		if err != nil {
			return nil, err
		}
		params.Type = agenttypes.TaskTypeImplement
		params.BeanIDs = beanIDs

	case "review":
		params.Type = agenttypes.TaskTypeReview
		params.RootIssueID = work.RootIssueID

	case "pr":
		params.Type = agenttypes.TaskTypePR

	case "update-pr-description":
		if work.PRURL == "" {
			return nil, fmt.Errorf("work %s has no PR URL set", work.ID)
		}
		params.Type = agenttypes.TaskTypeUpdatePRDescription
		params.PRURL = work.PRURL

	default:
		return nil, fmt.Errorf("unknown task type: %s", task.TaskType)
	}

	return &TaskInput{Params: params}, nil
}

// logAnalysisInputFromMetadata builds a log analysis TaskInput from task metadata.
// The metadata is stored by the feedback processor when creating log_analysis tasks.
// Returns a TaskInput including the path to the temp file that should be cleaned up.
func logAnalysisInputFromMetadata(ctx context.Context, proj *project.Project, task *db.Task, work *db.Work) (*TaskInput, error) {
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

	// Fetch existing open beans for this work to help Claude match against them
	existingBeans := fetchExistingBeanSummaries(ctx, proj, work.ID)

	return &TaskInput{
		Params: agenttypes.TaskParams{
			Type:          agenttypes.TaskTypeLogAnalysis,
			TaskID:        task.ID,
			WorkID:        work.ID,
			BranchName:    branchName,
			RootIssueID:   rootIssueID,
			WorkflowName:  workflowName,
			JobName:       jobName,
			LogFilePath:   logFile.Name(),
			ExistingBeans: existingBeans,
			BeansPath:     proj.BeansPath(),
		},
		TempFilePath: logFile.Name(),
	}, nil
}

// fetchExistingBeanSummaries fetches open beans for the given work and converts them to summaries for matching.
func fetchExistingBeanSummaries(ctx context.Context, proj *project.Project, workID string) []agenttypes.BeanSummary {
	// Get bean IDs assigned to this work
	workBeans, err := proj.DB.GetWorkBeans(ctx, workID)
	if err != nil {
		fmt.Printf("Warning: failed to fetch work beans for deduplication: %v\n", err)
		return nil
	}

	if len(workBeans) == 0 {
		return nil
	}

	// Extract bean IDs
	beanIDs := make([]string, len(workBeans))
	for i, wb := range workBeans {
		beanIDs[i] = wb.BeanID
	}

	// Fetch bean details from beans database
	result, err := proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		fmt.Printf("Warning: failed to fetch bean details for deduplication: %v\n", err)
		return nil
	}

	// Filter to only open beans and convert to summaries
	summaries := make([]agenttypes.BeanSummary, 0, len(beanIDs))
	for _, beanID := range beanIDs {
		if bwd := result.GetBean(beanID); bwd != nil && bwd.Bean.Status == beans.StatusTodo {
			summaries = append(summaries, agenttypes.BeanSummary{
				ID:          bwd.Bean.ID,
				Title:       bwd.Bean.Title,
				Body: bwd.Bean.Body,
			})
		}
	}
	return summaries
}

// beanIDsForTask returns the bean IDs associated with a task, resolving them through
// the beans database to get full details and extracting just the IDs.
func beanIDsForTask(ctx context.Context, proj *project.Project, taskID string) ([]string, error) {
	beanList, err := getBeansForTask(ctx, proj, taskID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(beanList))
	for i, b := range beanList {
		ids[i] = b.ID
	}
	return ids, nil
}

// getBeansForTask retrieves the beans associated with a task.
func getBeansForTask(ctx context.Context, proj *project.Project, taskID string) ([]beans.Bean, error) {
	beanIDs, err := proj.DB.GetTaskBeans(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}

	// Get beans with dependencies
	result, err := proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beans: %w", err)
	}

	// Convert map to slice in order of beanIDs
	var beanList []beans.Bean
	for _, beanID := range beanIDs {
		if b, ok := result.Beans[beanID]; ok {
			beanList = append(beanList, b)
		} else {
			fmt.Printf("Warning: bean %s not found\n", beanID)
		}
	}

	return beanList, nil
}

// processTask processes a single task by ID using inline execution.
// This blocks until the task is complete.
func processTask(proj *project.Project, taskID string, agent agents.Agent) error {
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

	// Build params for agent based on task type
	taskInput, err := taskInputForTask(ctx, proj, dbTask, work)
	if err != nil {
		return err
	}

	// Clean up temp file after execution (if any)
	if taskInput.TempFilePath != "" {
		defer func() { _ = os.Remove(taskInput.TempFilePath) }()
	}

	// Execute agent inline (blocking)
	if err := agent.Run(ctx, proj.DB, taskID, taskInput.Params, work.WorktreePath, proj.Config); err != nil {
		return fmt.Errorf("task %s failed: %w", taskID, err)
	}

	fmt.Printf("\n=== Task %s completed ===\n", taskID)
	return nil
}
