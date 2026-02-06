package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/agents"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/orchestration"
	"github.com/sargehq/sarge/internal/procmon"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/work"
	"github.com/spf13/cobra"
)

var (
	flagOrchestrateWork string
)

var orchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "[Agent] Execute tasks for a work unit",
	Long: `[Agent Command - Spawned automatically by the system, not for direct user invocation]

Internal command that polls for ready tasks and executes them. Runs in a zellij tab
and is spawned automatically when a work unit is created or restarted.`,
	Hidden: true,
	RunE:   runOrchestrate,
}

func init() {
	orchestrateCmd.Flags().StringVar(&flagOrchestrateWork, "work", "", "work ID to orchestrate")
}

func runOrchestrate(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// Apply hooks.env to current process - inherited by child processes (Claude)
	applyHooksEnv(proj.Config.Hooks.Env)

	// Set BEADS_DIR so bd commands work in Claude
	_ = os.Setenv("BEADS_DIR", proj.BeadsPath())

	// Get theWork ID
	workID := flagOrchestrateWork
	if workID == "" {
		return fmt.Errorf("--theWork is required")
	}

	// Get theWork to verify it exists
	theWork, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get theWork: %w", err)
	}
	if theWork == nil {
		return fmt.Errorf("theWork %s not found", workID)
	}

	fmt.Printf("=== Orchestrating theWork: %s ===\n", workID)
	fmt.Printf("Worktree: %s\n", theWork.WorktreePath)
	fmt.Printf("Branch: %s (base: %s)\n", theWork.BranchName, theWork.BaseBranch)
	if theWork.Auto {
		fmt.Printf("Mode: Automated workflow\n")
	}

	// If this is an automated workflow theWork and no tasks exist yet, set up the automated workflow
	if theWork.Auto {
		tasks, err := proj.DB.GetWorkTasks(ctx, workID)
		if err != nil {
			return fmt.Errorf("failed to check for existing tasks: %w", err)
		}

		// Only set up automated workflow if no tasks exist yet
		if len(tasks) == 0 {
			fmt.Println("\nSetting up automated workflow...")

			// Create estimate task from unassigned theWork beads (post-estimation will create implement tasks)
			workSvc := work.NewWorkService(proj)
			err := workSvc.CreateEstimateTaskFromWorkBeads(ctx, workID, os.Stdout)
			if err != nil {
				return fmt.Errorf("failed to create estimate task: %w", err)
			}

			// The orchestrator loop will handle executing the tasks
			fmt.Println("Automated workflow tasks created. Starting execution...")
		}
	}

	// Reset any stuck processing tasks from a previous run
	// When the orchestrator restarts, any tasks that were processing are now orphaned
	// since the Claude process was killed along with the orchestrator
	if err := orchestration.ResetStuckProcessingTasks(ctx, proj, workID); err != nil {
		return fmt.Errorf("failed to reset stuck tasks: %w", err)
	}

	// Register this orchestrator process for heartbeat monitoring
	procManager := procmon.NewManager(proj.DB, db.DefaultHeartbeatInterval)
	if err := procManager.RegisterOrchestrator(ctx, workID); err != nil {
		return fmt.Errorf("failed to register orchestrator: %w", err)
	}
	defer procManager.Stop()

	// NOTE: Scheduler watching is now handled by the control plane globally.
	// The control plane watches for scheduled tasks across ALL works and handles
	// git push retries, PR feedback polling, etc. This allows scheduled tasks
	// to be processed even when no orchestrator is running for a theWork.

	// Create agent once for all tasks
	agent := agents.NewAgent(proj.Config)

	// Main orchestration loop: poll for ready tasks and execute them
	for {

		// Check if theWork still exists (may have been destroyed)
		theWork, err = proj.DB.GetWork(ctx, workID)
		if err != nil {
			return fmt.Errorf("failed to check theWork status: %w", err)
		}
		if theWork == nil {
			fmt.Println("Work has been destroyed, exiting orchestrator.")
			return nil
		}

		// Get ready tasks (pending with all dependencies completed)
		readyTasks, err := proj.DB.GetReadyTasksForWork(ctx, workID)
		if err != nil {
			return fmt.Errorf("failed to get ready tasks: %w", err)
		}

		if len(readyTasks) == 0 {
			// No ready tasks - check if we're done or blocked
			allTasks, err := proj.DB.GetWorkTasks(ctx, workID)
			if err != nil {
				return fmt.Errorf("failed to get theWork tasks: %w", err)
			}

			pendingCount := 0
			processingCount := 0
			failedCount := 0
			completedCount := 0

			for _, t := range allTasks {
				switch t.Status {
				case db.StatusPending:
					pendingCount++
				case db.StatusProcessing:
					processingCount++
				case db.StatusFailed:
					failedCount++
				case db.StatusCompleted:
					completedCount++
				}
			}

			// If tasks are processing, wait and retry
			if processingCount > 0 {
				msg := fmt.Sprintf("Waiting for %d processing task(s)...", processingCount)
				orchestration.SpinnerWait(msg, 5*time.Second)
				continue
			}

			// If there are failures, mark theWork as failed and halt
			if failedCount > 0 {
				if theWork.Status != db.StatusFailed {
					if err := proj.DB.FailWork(ctx, workID, fmt.Sprintf("%d task(s) failed", failedCount)); err != nil {
						fmt.Printf("Warning: failed to mark theWork as failed: %v\n", err)
					} else {
						fmt.Printf("\n=== Work %s failed ===\n", workID)
						fmt.Printf("%d task(s) failed. Resolve failures and restart the theWork.\n", failedCount)
						_, _ = proj.DB.GetWork(ctx, workID)
					}
				}
				// Wait for user to resolve and restart
				msg := fmt.Sprintf("Work failed: %d task(s) failed. Waiting for restart...", failedCount)
				orchestration.SpinnerWait(msg, 10*time.Second)
				continue
			}

			// If pending tasks exist but none are ready, they're blocked
			if pendingCount > 0 {
				msg := fmt.Sprintf("Waiting: %d pending task(s) blocked by dependencies...", pendingCount)
				orchestration.SpinnerWait(msg, 5*time.Second)
				continue
			}

			// All tasks completed - transition theWork to idle status (waiting for more tasks)
			if completedCount > 0 && theWork.Status != db.StatusIdle {
				// Find PR URL from the PR task (if one exists, and we don't already have one)
				prURL := theWork.PRURL // Start with existing PR URL
				if prURL == "" {
					for _, t := range allTasks {
						if t.TaskType == "pr" && t.Status == db.StatusCompleted && t.PRURL != "" {
							prURL = t.PRURL
							break
						}
					}
				}

				// Transition to idle state and update PR URL if available
				if err := proj.DB.IdleWorkWithPR(ctx, workID, prURL); err != nil {
					fmt.Printf("Warning: failed to mark theWork as idle: %v\n", err)
				} else {
					fmt.Printf("\n=== Work %s idle ===\n", workID)
					if prURL != "" {
						fmt.Printf("PR: %s\n", prURL)
					}
					fmt.Println("All tasks completed. Waiting for new tasks or explicit completion.")
					// Refresh theWork status
					_, _ = proj.DB.GetWork(ctx, workID)
				}
			}

			// Wait for new tasks with spinner
			var msg string
			if completedCount > 0 {
				msg = fmt.Sprintf("Idle: %d task(s) completed. Waiting for new tasks...", completedCount)
			} else {
				msg = "No tasks yet. Waiting for tasks to be created..."
			}
			orchestration.SpinnerWait(msg, 5*time.Second)
			continue
		}

		// Execute the first ready task
		task := readyTasks[0]
		fmt.Printf("\n=== Executing task: %s (type: %s) ===\n", task.ID, task.TaskType)

		// Update activity when starting execution
		if err := proj.DB.UpdateTaskActivity(ctx, task.ID, time.Now()); err != nil {
			fmt.Printf("Warning: failed to update task activity at start: %v\n", err)
		}

		if err := executeTask(proj, task, theWork, agent); err != nil {
			return fmt.Errorf("task %s failed: %w", task.ID, err)
		}
	}
}

// executeTask executes a single task inline based on its type.
func executeTask(proj *project.Project, t *db.Task, work *db.Work, agent agents.Agent) error {
	ctx := GetContext()

	// Create a context with timeout from configuration
	timeout := proj.Config.Claude.GetTaskTimeout()
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fmt.Printf("Task timeout: %v\n", timeout)

	// Build params for Claude based on task type
	taskInput, err := taskInputForTask(taskCtx, proj, t, work)
	if err != nil {
		return err
	}

	// Clean up temp file after execution (if any)
	if taskInput.TempFilePath != "" {
		defer func() { _ = os.Remove(taskInput.TempFilePath) }()
	}

	// Execute Claude inline with timeout context
	if err = agent.Run(taskCtx, proj.DB, t.ID, taskInput.Params, work.WorktreePath, proj.Config); err != nil {
		// Check if it was a timeout error
		if errors.Is(err, context.DeadlineExceeded) {
			// Mark the task as failed due to timeout
			// Use context.Background() since the original context is cancelled
			if dbErr := proj.DB.FailTask(context.Background(), t.ID, fmt.Sprintf("Task timed out after %v", timeout)); dbErr != nil {
				fmt.Printf("Warning: failed to mark timed out task as failed: %v\n", dbErr)
			}
			return fmt.Errorf("task %s timed out after %v", t.ID, timeout)
		}
		return err
	}

	// Defensive: auto-complete log_analysis tasks if Claude exited without calling sarge complete.
	// Log analysis tasks are observational (they create beads but don't modify code), so it's
	// safe to auto-complete them. This prevents the orchestrator from spinning forever.
	if t.TaskType == "log_analysis" {
		updatedTask, err := proj.DB.GetTask(ctx, t.ID)
		if err != nil {
			fmt.Printf("Warning: failed to re-read task %s status: %v\n", t.ID, err)
		} else if updatedTask != nil && updatedTask.Status == db.StatusProcessing {
			logging.Warn("auto-completing log_analysis task that Claude did not mark complete",
				"task_id", t.ID)
			fmt.Printf("Warning: Claude exited without completing log_analysis task %s, auto-completing\n", t.ID)
			if err := proj.DB.CompleteTask(ctx, t.ID, ""); err != nil {
				fmt.Printf("Warning: failed to auto-complete task %s: %v\n", t.ID, err)
			}
		}
	}

	// Post-execution handling based on task type
	switch t.TaskType {
	case "estimate":
		if err := handlePostEstimation(proj, t, work); err != nil {
			return fmt.Errorf("failed to create post-estimation tasks: %w", err)
		}
	case "review":
		if err := handleReviewFixLoop(proj, t, work); err != nil {
			return fmt.Errorf("failed to handle review completion: %w", err)
		}
	}

	return nil
}

// handlePostEstimation creates implement, review, and PR tasks after estimation completes.
// Uses bin-packing to group beads based on their complexity estimates.
func handlePostEstimation(proj *project.Project, estimateTask *db.Task, work *db.Work) error {
	ctx := GetContext()

	fmt.Println("Creating implement, review, and PR tasks based on complexity estimates...")

	// Get the beads that were estimated
	beadIDs, err := proj.DB.GetTaskBeads(ctx, estimateTask.ID)
	if err != nil {
		return fmt.Errorf("failed to get task beads: %w", err)
	}

	if len(beadIDs) == 0 {
		return fmt.Errorf("no beads found for estimate task %s", estimateTask.ID)
	}

	// Get issues with dependencies for planning
	issuesResult, err := proj.Beads.GetBeadsWithDeps(ctx, beadIDs)
	if err != nil {
		return fmt.Errorf("failed to get bead details: %w", err)
	}

	// Verify all beads were found
	for _, beadID := range beadIDs {
		if _, found := issuesResult.Beads[beadID]; !found {
			return fmt.Errorf("bead %s not found", beadID)
		}
	}

	// Convert map to slice
	beadList := make([]beads.Bead, 0, len(issuesResult.Beads))
	for _, b := range issuesResult.Beads {
		beadList = append(beadList, b)
	}

	// Create planner with cached complexity estimator
	estimator := task.NewLLMEstimator(proj.DB, work.WorktreePath, proj.Config.Project.Name, work.ID)
	planner := task.NewDefaultPlanner(estimator)

	// Use token budget of 120K for bin-packing (context window is 200K, leave headroom)
	const tokenBudget = 120000
	fmt.Printf("Planning tasks with token budget %dK...\n", tokenBudget/1000)

	tasks, err := planner.Plan(ctx, beadList, issuesResult.Dependencies, tokenBudget)
	if err != nil {
		return fmt.Errorf("failed to plan tasks: %w", err)
	}

	if len(tasks) == 0 {
		return fmt.Errorf("planner returned no tasks for %d beads", len(beadIDs))
	}

	// Create implement tasks and track beadID → taskID mapping
	var implementTaskIDs []string
	beadToTask := make(map[string]string) // beadID → taskID
	for _, t := range tasks {
		nextNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
		if err != nil {
			return fmt.Errorf("failed to get next task number: %w", err)
		}
		taskID := fmt.Sprintf("%s.%d", work.ID, nextNum)

		if err := proj.DB.CreateTask(ctx, taskID, "implement", t.BeadIDs, t.Complexity, work.ID); err != nil {
			return fmt.Errorf("failed to create implement task: %w", err)
		}

		// Add dependency: implement depends on estimate
		if err := proj.DB.AddTaskDependency(ctx, taskID, estimateTask.ID); err != nil {
			return fmt.Errorf("failed to add dependency for %s: %w", taskID, err)
		}

		// Track which task each bead is in
		for _, beadID := range t.BeadIDs {
			beadToTask[beadID] = taskID
		}

		implementTaskIDs = append(implementTaskIDs, taskID)
		fmt.Printf("Created implement task %s (complexity: %d) with %d bead(s): %v\n",
			taskID, t.Complexity, len(t.BeadIDs), t.BeadIDs)
	}

	// Compute inter-task dependencies from bead dependencies.
	// If bead A (in task X) depends on bead B (in task Y where Y != X), task X depends on task Y.
	interTaskDeps := make(map[string]map[string]bool) // taskID → set of dependent taskIDs
	for beadID, deps := range issuesResult.Dependencies {
		taskID, ok := beadToTask[beadID]
		if !ok {
			continue // bead not in our task set
		}
		for _, dep := range deps {
			depTaskID, ok := beadToTask[dep.DependsOnID]
			if !ok {
				continue // dependency not in our task set
			}
			if taskID == depTaskID {
				continue // same task, no inter-task dependency
			}
			// taskID depends on depTaskID
			if interTaskDeps[taskID] == nil {
				interTaskDeps[taskID] = make(map[string]bool)
			}
			interTaskDeps[taskID][depTaskID] = true
		}
	}

	// Add inter-task dependencies to database
	for taskID, depTaskIDs := range interTaskDeps {
		for depTaskID := range depTaskIDs {
			if err := proj.DB.AddTaskDependency(ctx, taskID, depTaskID); err != nil {
				return fmt.Errorf("failed to add inter-task dependency %s -> %s: %w", taskID, depTaskID, err)
			}
			fmt.Printf("Added inter-task dependency: %s depends on %s\n", taskID, depTaskID)
		}
	}

	// Create review task (depends on all implement tasks)
	// PR task is NOT created here - it will be created after review passes
	reviewTaskNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for review: %w", err)
	}
	reviewTaskID := fmt.Sprintf("%s.%d", work.ID, reviewTaskNum)
	if err := proj.DB.CreateTask(ctx, reviewTaskID, "review", nil, 0, work.ID); err != nil {
		return fmt.Errorf("failed to create review task: %w", err)
	}
	for _, implID := range implementTaskIDs {
		if err := proj.DB.AddTaskDependency(ctx, reviewTaskID, implID); err != nil {
			return fmt.Errorf("failed to add dependency for review: %w", err)
		}
	}
	fmt.Printf("Created review task: %s (depends on %d implement tasks)\n", reviewTaskID, len(implementTaskIDs))

	fmt.Printf("Successfully created %d implement task(s) and 1 review task\n", len(implementTaskIDs))
	fmt.Println("PR task will be created after review passes.")
	return nil
}

// handleReviewFixLoop checks if a review task found issues and creates fix tasks.
// If review passes (no issues), creates the PR task.
// If review finds issues, creates fix tasks and a new review task.
func handleReviewFixLoop(proj *project.Project, reviewTask *db.Task, work *db.Work) error {
	ctx := GetContext()

	// Check if this is a manual review task (auto_workflow=false)
	// Manual review tasks should not trigger automated workflow (fix tasks or PR creation)
	autoWorkflow, err := proj.DB.GetTaskMetadata(ctx, reviewTask.ID, "auto_workflow")
	if err == nil && autoWorkflow == "false" {
		fmt.Println("Manual review task completed - skipping automated workflow")
		return nil
	}

	// Count how many review iterations we've had
	reviewCount := orchestration.CountReviewIterations(ctx, proj.DB, work.ID)
	maxIterations := proj.Config.Workflow.GetMaxReviewIterations()
	if reviewCount >= maxIterations {
		fmt.Printf("Warning: Maximum review iterations (%d) reached, proceeding to PR\n", maxIterations)
		return createPRTask(proj, work, reviewTask.ID)
	}

	// Check if the review created any issue beads under the root issue
	var beadsToFix []beads.Bead
	if work.RootIssueID != "" {
		// Get all children of the root issue
		rootChildrenIssues, err := proj.Beads.GetBeadWithChildren(ctx, work.RootIssueID)
		if err != nil {
			return fmt.Errorf("failed to get children of root issue %s: %w", work.RootIssueID, err)
		}

		// Filter to only ready beads that were created by this review task
		// (excluding the root issue itself)
		expectedExternalRef := fmt.Sprintf("review-%s", reviewTask.ID)
		for _, issue := range rootChildrenIssues {
			if issue.ID != work.RootIssueID &&
				beads.IsWorkableStatus(issue.Status) &&
				issue.ExternalRef == expectedExternalRef {
				beadsToFix = append(beadsToFix, issue)
			}
		}
	}

	// If work has a PR URL, also check for PR feedback
	if len(beadsToFix) == 0 && work.PRURL != "" {
		fmt.Println("Review passed - checking for PR feedback...")

		// Process PR feedback - creates beads but doesn't add them to work
		_, err := feedback.ProcessPRFeedback(ctx, proj, proj.DB, work.ID)
		if err != nil {
			fmt.Printf("Warning: failed to check PR feedback: %v\n", err)
		} else {
			// Re-check for new beads from PR feedback
			if work.RootIssueID != "" {
				// Re-fetch children of root issue to see if feedback created new beads
				rootChildrenIssues, err := proj.Beads.GetBeadWithChildren(ctx, work.RootIssueID)
				if err == nil {
					// Filter for any new open beads (not just review-created ones)
					for _, issue := range rootChildrenIssues {
						if issue.ID != work.RootIssueID &&
							beads.IsWorkableStatus(issue.Status) {
							// Check if this bead is already in a task
							inTask, _ := proj.DB.IsBeadInTask(ctx, work.ID, issue.ID)
							if !inTask {
								beadsToFix = append(beadsToFix, issue)
							}
						}
					}
				}
			}
		}
	}

	if len(beadsToFix) == 0 {
		fmt.Println("Review passed and no PR feedback issues found!")
		return createPRTask(proj, work, reviewTask.ID)
	}

	fmt.Printf("Review found %d issue(s) - creating fix tasks...\n", len(beadsToFix))

	// Create fix tasks for each bead
	var fixTaskIDs []string
	for _, b := range beadsToFix {
		nextNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
		if err != nil {
			return fmt.Errorf("failed to get next task number: %w", err)
		}
		taskID := fmt.Sprintf("%s.%d", work.ID, nextNum)

		if err := proj.DB.CreateTask(ctx, taskID, "implement", []string{b.ID}, 0, work.ID); err != nil {
			return fmt.Errorf("failed to create fix task: %w", err)
		}

		// Fix task depends on the current review task
		if err := proj.DB.AddTaskDependency(ctx, taskID, reviewTask.ID); err != nil {
			return fmt.Errorf("failed to add dependency for fix task %s: %w", taskID, err)
		}

		fixTaskIDs = append(fixTaskIDs, taskID)
		fmt.Printf("Created fix task %s for bead %s: %s\n", taskID, b.ID, b.Title)
	}

	// Create a new review task that depends on all fix tasks
	newReviewTaskNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for review: %w", err)
	}
	newReviewTaskID := fmt.Sprintf("%s.%d", work.ID, newReviewTaskNum)
	if err := proj.DB.CreateTask(ctx, newReviewTaskID, "review", nil, 0, work.ID); err != nil {
		return fmt.Errorf("failed to create new review task: %w", err)
	}
	for _, fixID := range fixTaskIDs {
		if err := proj.DB.AddTaskDependency(ctx, newReviewTaskID, fixID); err != nil {
			return fmt.Errorf("failed to add dependency for new review task: %w", err)
		}
	}
	fmt.Printf("Created new review task: %s (depends on %d fix tasks)\n", newReviewTaskID, len(fixTaskIDs))

	return nil
}

// createPRTask creates the PR task (or update-pr-description task) that depends on a review task.
// If a PR task already exists and is completed (PR was created), creates an update-pr-description task instead.
// If a PR task exists but is pending/processing, skips creation.
func createPRTask(proj *project.Project, work *db.Work, reviewTaskID string) error {
	ctx := GetContext()

	// Check if a PR task already exists for this work
	existingPRTask, err := proj.DB.GetPRTaskForWork(ctx, work.ID)
	if err != nil {
		return fmt.Errorf("failed to check for existing PR task: %w", err)
	}

	if existingPRTask != nil {
		switch existingPRTask.Status {
		case db.StatusPending, db.StatusProcessing:
			// PR task exists and is still pending/processing, skip creation
			fmt.Printf("PR task %s already exists (status: %s), skipping creation\n",
				existingPRTask.ID, existingPRTask.Status)
			return nil
		case db.StatusCompleted:
			// PR was created, create an update-pr-description task instead
			return createUpdatePRDescriptionTask(proj, work, reviewTaskID)
		}
	}

	// No existing PR task, create one
	prTaskNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for PR: %w", err)
	}
	prTaskID := fmt.Sprintf("%s.%d", work.ID, prTaskNum)

	if err := proj.DB.CreateTask(ctx, prTaskID, "pr", nil, 0, work.ID); err != nil {
		return fmt.Errorf("failed to create PR task: %w", err)
	}
	if err := proj.DB.AddTaskDependency(ctx, prTaskID, reviewTaskID); err != nil {
		return fmt.Errorf("failed to add dependency for PR: %w", err)
	}
	fmt.Printf("Created PR task: %s (depends on %s)\n", prTaskID, reviewTaskID)
	return nil
}

// createUpdatePRDescriptionTask creates a task to update the PR description.
// This is used when a PR already exists and we need to update it after subsequent reviews.
func createUpdatePRDescriptionTask(proj *project.Project, work *db.Work, reviewTaskID string) error {
	ctx := GetContext()

	taskNum, err := proj.DB.GetNextTaskNumber(ctx, work.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for update-pr-description: %w", err)
	}
	taskID := fmt.Sprintf("%s.%d", work.ID, taskNum)

	if err := proj.DB.CreateTask(ctx, taskID, "update-pr-description", nil, 0, work.ID); err != nil {
		return fmt.Errorf("failed to create update-pr-description task: %w", err)
	}
	if err := proj.DB.AddTaskDependency(ctx, taskID, reviewTaskID); err != nil {
		return fmt.Errorf("failed to add dependency for update-pr-description: %w", err)
	}
	fmt.Printf("Created update-pr-description task: %s (depends on %s)\n", taskID, reviewTaskID)
	fmt.Printf("PR URL: %s\n", work.PRURL)
	return nil
}

// applyHooksEnv sets environment variables from the hooks.env config.
// Variables are set on the current process and inherited by child processes.
// Format: ["KEY=value", "PATH=/a/b:$PATH"]
func applyHooksEnv(env []string) {
	for _, e := range env {
		// Split on first '=' only
		parts := splitEnvVar(e)
		if len(parts) == 2 {
			// Expand any environment variable references in the value
			expandedValue := os.ExpandEnv(parts[1])
			os.Setenv(parts[0], expandedValue)
		}
	}
}

// splitEnvVar splits "KEY=value" into ["KEY", "value"], handling values with '='
func splitEnvVar(s string) []string {
	idx := -1
	for i, c := range s {
		if c == '=' {
			idx = i
			break
		}
	}
	if idx < 0 {
		return []string{s}
	}
	return []string{s[:idx], s[idx+1:]}
}
