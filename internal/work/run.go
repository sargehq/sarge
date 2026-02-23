package work

import (
	"context"
	"fmt"
	"io"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/task"
)

// RunWorkResult contains the result of running work.
type RunWorkResult struct {
	WorkID              string
	TasksCreated        int
	OrchestratorSpawned bool
}

// RunWorkAutoResult contains the result of running work in auto mode.
type RunWorkAutoResult struct {
	WorkID              string
	EstimateTaskCreated bool
	OrchestratorSpawned bool
}

// PlanWorkTasksResult contains the result of planning work tasks.
type PlanWorkTasksResult struct {
	TasksCreated int
}

// RunWorkOptions contains options for running work.
type RunWorkOptions struct {
	UsePlan       bool
	ForceEstimate bool
}

// RunWork creates tasks from unassigned beads and ensures an orchestrator is running.
// This is the core logic used by both the CLI `sarge run` command and the TUI.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (s *WorkService) RunWork(ctx context.Context, workID string, usePlan bool, w io.Writer) (*RunWorkResult, error) {
	return s.RunWorkWithOptions(ctx, workID, RunWorkOptions{UsePlan: usePlan}, w)
}

// RunWorkWithOptions creates tasks from unassigned beads and ensures an orchestrator is running.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (s *WorkService) RunWorkWithOptions(ctx context.Context, workID string, opts RunWorkOptions, w io.Writer) (*RunWorkResult, error) {
	// Get work details to verify it exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Check if worktree exists
	if work.WorktreePath == "" {
		return nil, fmt.Errorf("work %s has no worktree path configured", work.ID)
	}

	if !s.Worktree.ExistsPath(work.WorktreePath) {
		return nil, fmt.Errorf("work %s worktree does not exist at %s", work.ID, work.WorktreePath)
	}

	// Create tasks from unassigned work beads
	tasksCreated, err := s.createTasksFromWorkBeans(ctx, workID, opts.UsePlan, opts.ForceEstimate, w)
	if err != nil {
		return nil, fmt.Errorf("failed to create tasks: %w", err)
	}

	// Ensure orchestrator is running
	spawned, err := s.OrchestratorManager.EnsureWorkOrchestrator(ctx, workID, s.Config.Project.Name, work.WorktreePath, work.Name, w)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure orchestrator: %w", err)
	}

	return &RunWorkResult{
		WorkID:              workID,
		TasksCreated:        tasksCreated,
		OrchestratorSpawned: spawned,
	}, nil
}

// RunWorkAuto creates an estimate task and spawns the orchestrator for automated workflow.
// This mirrors the 'sarge run --auto' behavior: create estimate task, let orchestrator handle
// estimation and create implement tasks afterward.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (s *WorkService) RunWorkAuto(ctx context.Context, workID string, w io.Writer) (*RunWorkAutoResult, error) {
	// Get work details to verify it exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Check if worktree exists
	if work.WorktreePath == "" {
		return nil, fmt.Errorf("work %s has no worktree path configured", work.ID)
	}

	if !s.Worktree.ExistsPath(work.WorktreePath) {
		return nil, fmt.Errorf("work %s worktree does not exist at %s", work.ID, work.WorktreePath)
	}

	// Create estimate task from unassigned work beads (post-estimation will create implement tasks)
	err = s.CreateEstimateTaskFromWorkBeans(ctx, workID, w)
	if err != nil {
		return nil, fmt.Errorf("failed to create estimate task: %w", err)
	}

	// Ensure orchestrator is running
	spawned, err := s.OrchestratorManager.EnsureWorkOrchestrator(ctx, workID, s.Config.Project.Name, work.WorktreePath, work.Name, w)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure orchestrator: %w", err)
	}

	return &RunWorkAutoResult{
		WorkID:              workID,
		EstimateTaskCreated: true,
		OrchestratorSpawned: spawned,
	}, nil
}

// PlanWorkTasks creates tasks from unassigned beads in a work unit without spawning an orchestrator.
// If autoGroup is true, uses LLM complexity estimation to group beads into tasks.
// Otherwise, uses existing group assignments from work_beads (one task per bead or group).
// Progress messages are written to w. Pass io.Discard to suppress output.
func (s *WorkService) PlanWorkTasks(ctx context.Context, workID string, autoGroup bool, w io.Writer) (*PlanWorkTasksResult, error) {
	// Get work details to verify it exists
	work, err := s.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Create tasks from unassigned work beads
	tasksCreated, err := s.createTasksFromWorkBeans(ctx, workID, autoGroup, false, w)
	if err != nil {
		return nil, fmt.Errorf("failed to create tasks: %w", err)
	}

	return &PlanWorkTasksResult{
		TasksCreated: tasksCreated,
	}, nil
}

// CreateEstimateTaskFromWorkBeans creates an estimate task from unassigned work beads.
// This is used in --auto mode where the full automated workflow includes estimation.
// After the estimate task completes, handlePostEstimation creates implement tasks.
func (s *WorkService) CreateEstimateTaskFromWorkBeans(ctx context.Context, workID string, w io.Writer) error {
	// Get unassigned beads
	unassigned, err := s.DB.GetUnassignedWorkBeans(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get unassigned beads: %w", err)
	}

	if len(unassigned) == 0 {
		return fmt.Errorf("no unassigned beads found for work %s", workID)
	}

	fmt.Fprintf(w, "\nFound %d unassigned bead(s)\n", len(unassigned))

	// Collect bead IDs
	var beanIDs []string
	for _, wb := range unassigned {
		beanIDs = append(beanIDs, wb.BeanID)
	}

	// Get next task number
	taskNum, err := s.DB.GetNextTaskNumber(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get next task number: %w", err)
	}

	// Create the estimate task
	taskID := fmt.Sprintf("%s.%d", workID, taskNum)
	if err := s.DB.CreateTask(ctx, taskID, "estimate", beanIDs, 0, workID, taskNum); err != nil {
		return fmt.Errorf("failed to create estimate task: %w", err)
	}

	fmt.Fprintf(w, "  Created estimate task %s with %d bead(s)\n", taskID, len(beanIDs))
	fmt.Fprintln(w, "  Implement tasks will be created after estimation completes.")

	return nil
}

// createTasksFromWorkBeans creates tasks from unassigned beads in work_beads.
// If usePlan is true, uses LLM complexity estimation to group beads.
// Returns the number of tasks created.
func (s *WorkService) createTasksFromWorkBeans(ctx context.Context, workID string, usePlan bool, forceEstimate bool, w io.Writer) (int, error) {
	// Get unassigned beads
	unassigned, err := s.DB.GetUnassignedWorkBeans(ctx, workID)
	if err != nil {
		return 0, fmt.Errorf("failed to get unassigned beads: %w", err)
	}

	if len(unassigned) == 0 {
		return 0, nil
	}

	fmt.Fprintf(w, "\nFound %d unassigned bead(s)\n", len(unassigned))

	// Collect bead IDs from unassigned work_beads
	beanIDs := make([]string, len(unassigned))
	for i, wb := range unassigned {
		beanIDs[i] = wb.BeanID
	}

	// Get all issues with dependencies in one call
	issuesResult, err := s.BeadsReader.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		return 0, fmt.Errorf("failed to get bead details: %w", err)
	}

	// Verify all beads were found
	for _, beanID := range beanIDs {
		if _, found := issuesResult.Beans[beanID]; !found {
			return 0, fmt.Errorf("bead %s not found", beanID)
		}
	}

	// Group beads into tasks
	var taskGroups [][]string // Each inner slice is a group of bead IDs for one task

	if usePlan {
		// Use LLM complexity estimation to group beads
		fmt.Fprintln(w, "Using LLM complexity estimation to group beads...")
		taskGroups, err = s.planBeadsWithComplexity(ctx, issuesResult, workID, forceEstimate)
		if err != nil {
			return 0, fmt.Errorf("failed to plan beads: %w", err)
		}
	} else {
		// Each bead becomes its own task
		for _, wb := range unassigned {
			taskGroups = append(taskGroups, []string{wb.BeanID})
		}
	}

	// Create tasks from groups
	tasksCreated := 0
	for _, groupBeanIDs := range taskGroups {
		if len(groupBeanIDs) == 0 {
			continue
		}

		// Get next task number
		taskNum, err := s.DB.GetNextTaskNumber(ctx, workID)
		if err != nil {
			return tasksCreated, fmt.Errorf("failed to get next task number: %w", err)
		}

		taskID := fmt.Sprintf("%s.%d", workID, taskNum)
		if err := s.DB.CreateTask(ctx, taskID, "implement", groupBeanIDs, 0, workID, taskNum); err != nil {
			return tasksCreated, fmt.Errorf("failed to create task: %w", err)
		}

		fmt.Fprintf(w, "  Created task %s with %d bead(s)\n", taskID, len(groupBeanIDs))
		tasksCreated++
	}

	return tasksCreated, nil
}

// planBeadsWithComplexity uses LLM complexity estimation to group beads.
// If forceEstimate is true, re-estimates complexity even if cached values exist.
// If s.TaskPlanner is set, uses it directly; otherwise creates a default planner.
func (s *WorkService) planBeadsWithComplexity(ctx context.Context, issuesResult *beans.BeansWithDepsResult, workID string, forceEstimate bool) ([][]string, error) {
	// Convert map to slice of beads
	beadList := make([]beans.Bean, 0, len(issuesResult.Beans))
	for _, b := range issuesResult.Beans {
		beadList = append(beadList, b)
	}

	// If a task planner is configured, use it directly
	if s.TaskPlanner != nil {
		// Plan tasks using token budget of 120K (context window is 200K, leave headroom)
		const tokenBudget = 120000
		planned, err := s.TaskPlanner.Plan(ctx, beadList, issuesResult.Dependencies, tokenBudget)
		if err != nil {
			return nil, fmt.Errorf("failed to plan tasks: %w", err)
		}

		// Convert planned tasks to bead ID groups
		var groups [][]string
		for _, p := range planned {
			groups = append(groups, p.BeanIDs)
		}
		return groups, nil
	}

	// Fall back to creating LLM estimator and planner inline
	estimator := task.NewLLMEstimator(s.DB, s.MainRepoPath, s.Config.Project.Name, workID)
	planner := task.NewDefaultPlanner(estimator)

	// Estimate complexity for each bead
	result, err := estimator.EstimateBatch(ctx, beadList, forceEstimate)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate complexity: %w", err)
	}

	// If a task was spawned for estimation, we need to wait for it
	if result.TaskSpawned {
		return nil, fmt.Errorf("estimation task %s spawned - re-run after it completes", result.TaskID)
	}

	// Plan tasks using token budget of 120K (context window is 200K, leave headroom)
	const tokenBudget = 120000
	planned, err := planner.Plan(ctx, beadList, issuesResult.Dependencies, tokenBudget)
	if err != nil {
		return nil, fmt.Errorf("failed to plan tasks: %w", err)
	}

	// Convert planned tasks to bead ID groups
	var groups [][]string
	for _, p := range planned {
		groups = append(groups, p.BeanIDs)
	}

	return groups, nil
}
