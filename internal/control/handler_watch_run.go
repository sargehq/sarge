package control

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleWatchWorkflowRunTask watches a GitHub Actions workflow run until completion.
// When the run completes (success or failure), it schedules a PRFeedback task to run immediately.
// This replaces polling for workflow runs that are in-progress.
//
// Required metadata:
// - run_id: The GitHub Actions workflow run ID to watch
// - repo: The repository in owner/repo format
func (cp *ControlPlane) HandleWatchWorkflowRunTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	runIDStr := task.Metadata["run_id"]
	repo := task.Metadata["repo"]

	if runIDStr == "" || repo == "" {
		return fmt.Errorf("watch_workflow_run task missing run_id or repo metadata")
	}

	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid run_id: %w", err)
	}

	logging.Info("Starting gh run watch",
		"work_id", workID,
		"run_id", runID,
		"repo", repo)

	// Use the GitHub client to watch the workflow run
	// This blocks until the run completes
	err = cp.GitHubClient.WatchWorkflowRun(ctx, repo, runID)
	if err != nil {
		// Check if it's a context cancellation (work was destroyed)
		if errors.Is(ctx.Err(), context.Canceled) {
			logging.Info("Workflow watch cancelled (context cancelled)",
				"work_id", workID,
				"run_id", runID)
			return nil // Don't reschedule - work is being destroyed
		}

		// gh run watch returns non-zero when the run fails
		// This is expected behavior - we still want to fetch feedback
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Info("Workflow run completed with failure",
				"work_id", workID,
				"run_id", runID,
				"exit_code", exitErr.ExitCode())
		} else {
			// Actual error (network, auth, invalid run, etc.)
			return fmt.Errorf("workflow watch failed: %w", err)
		}
	} else {
		logging.Info("Workflow run completed successfully",
			"work_id", workID,
			"run_id", runID)
	}

	// Schedule PRFeedback task to run immediately
	_, err = proj.DB.ScheduleOrUpdateTask(ctx, workID, db.TaskTypePRFeedback, time.Now())
	if err != nil {
		logging.Warn("Failed to schedule PRFeedback task after workflow watch",
			"work_id", workID,
			"error", err)
		// Don't return error - the watch succeeded, just scheduling failed
	} else {
		logging.Info("Scheduled PRFeedback task after workflow completion",
			"work_id", workID,
			"run_id", runID)
	}

	return nil
}

// ScheduleWatchWorkflowRun schedules a task to watch a workflow run.
// Uses the run_id as the idempotency key to prevent duplicate watchers.
func ScheduleWatchWorkflowRun(ctx context.Context, proj *project.Project, workID string, runID int64, repo string) error {
	// Use run_id as idempotency key to prevent duplicate watchers for the same run
	idempotencyKey := fmt.Sprintf("watch_run_%d", runID)

	metadata := map[string]string{
		"run_id": strconv.FormatInt(runID, 10),
		"repo":   repo,
	}

	err := proj.DB.ScheduleTaskWithRetry(ctx, workID, db.TaskTypeWatchWorkflowRun, time.Now(), metadata, idempotencyKey, 1)
	if err != nil {
		return fmt.Errorf("failed to schedule watch_workflow_run task: %w", err)
	}

	logging.Info("Scheduled watch_workflow_run task",
		"work_id", workID,
		"run_id", runID,
		"repo", repo)

	return nil
}

// SpawnWorkflowWatchers checks for in-progress workflow runs and spawns watchers for them.
// This can be called immediately when a PR is created to catch fast CI runs that would
// otherwise complete before the first PR feedback poll.
// Returns the number of watchers spawned.
func SpawnWorkflowWatchers(ctx context.Context, proj *project.Project, ghClient github.ClientInterface, workID, prURL string) (int, error) {
	// Fetch PR status to get workflow run information
	status, err := ghClient.GetPRStatus(ctx, prURL)
	if err != nil {
		return 0, fmt.Errorf("failed to get PR status: %w", err)
	}

	// Extract repo from PR URL
	repo, err := github.ExtractRepoFromPRURL(prURL)
	if err != nil {
		return 0, fmt.Errorf("failed to extract repo from PR URL: %w", err)
	}

	// Check each workflow run for in-progress status
	watcherCount := 0
	for _, workflow := range status.Workflows {
		// Only watch runs that are in progress or queued
		if workflow.Status != "in_progress" && workflow.Status != "queued" {
			continue
		}

		// Schedule a watcher for this run
		err := ScheduleWatchWorkflowRun(ctx, proj, workID, workflow.ID, repo)
		if err != nil {
			// Log but continue - idempotency key prevents duplicates
			logging.Debug("failed to schedule workflow watcher",
				"run_id", workflow.ID,
				"error", err)
			continue
		}
		watcherCount++
	}

	if watcherCount > 0 {
		logging.Info("Spawned workflow watchers",
			"count", watcherCount,
			"work_id", workID)
	}

	return watcherCount, nil
}
