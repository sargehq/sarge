package control

import (
	"context"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandlePRFeedbackTask handles a scheduled PR feedback check.
// Returns nil on success, error on failure (caller handles retry/completion).
func (cp *ControlPlane) HandlePRFeedbackTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	logging.Debug("Starting PR feedback check task", "task_id", task.ID, "work_id", workID)

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil || work == nil || work.PRURL == "" {
		logging.Debug("No PR URL for work, not rescheduling", "work_id", workID, "has_pr", work != nil && work.PRURL != "")
		// Don't reschedule - scheduling happens when PR is created
		return nil
	}

	logging.Debug("Checking PR feedback", "pr_url", work.PRURL, "work_id", workID)

	// Process PR feedback - creates beads but doesn't add them to work
	createdCount, err := cp.FeedbackProcessor.ProcessPRFeedback(ctx, proj, proj.DB, workID)
	if err != nil {
		return fmt.Errorf("failed to check PR feedback: %w", err)
	}

	if createdCount > 0 {
		logging.Info("Created beads from PR feedback", "count", createdCount, "work_id", workID)
	} else {
		logging.Debug("No new PR feedback found", "work_id", workID)
	}

	// Spawn watchers for in-progress workflow runs
	if err := cp.spawnWorkflowWatchers(ctx, proj, work); err != nil {
		// Log but don't fail the task - watchers are an optimization
		logging.Warn("failed to spawn workflow watchers", "error", err, "work_id", workID)
	}

	// Schedule next check using configured interval
	nextInterval := proj.Config.Scheduler.GetPRFeedbackInterval()
	nextCheck := time.Now().Add(nextInterval)
	_, err = proj.DB.ScheduleOrUpdateTask(ctx, workID, db.TaskTypePRFeedback, nextCheck)
	if err != nil {
		logging.Warn("failed to schedule next PR feedback check", "error", err, "work_id", workID)
	} else {
		logging.Info("Scheduled next PR feedback check", "work_id", workID, "next_check", nextCheck.Format(time.RFC3339), "interval", nextInterval)
	}

	return nil
}

// spawnWorkflowWatchers checks for in-progress workflow runs and spawns watchers for them.
// This enables immediate notification when CI completes instead of waiting for the next poll.
func (cp *ControlPlane) spawnWorkflowWatchers(ctx context.Context, proj *project.Project, work *db.Work) error {
	// Fetch PR status to get workflow run information
	status, err := cp.GitHubClient.GetPRStatus(ctx, work.PRURL)
	if err != nil {
		return fmt.Errorf("failed to get PR status: %w", err)
	}

	// Extract repo from PR URL
	repo, err := github.ExtractRepoFromPRURL(work.PRURL)
	if err != nil {
		return fmt.Errorf("failed to extract repo from PR URL: %w", err)
	}

	// Check each workflow run for in-progress status
	watcherCount := 0
	for _, workflow := range status.Workflows {
		// Only watch runs that are in progress or queued
		if workflow.Status != "in_progress" && workflow.Status != "queued" {
			continue
		}

		// Schedule a watcher for this run
		err := ScheduleWatchWorkflowRun(ctx, proj, work.ID, workflow.ID, repo)
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
			"work_id", work.ID)
	}

	return nil
}
