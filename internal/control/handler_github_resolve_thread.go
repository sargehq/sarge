package control

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleGitHubResolveThreadTask handles a scheduled GitHub thread resolution task.
// Returns nil on success, error on failure (caller handles retry/completion).
func HandleGitHubResolveThreadTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	// Get thread details from metadata
	prURL := task.Metadata["pr_url"]
	commentIDStr := task.Metadata["comment_id"]

	if prURL == "" || commentIDStr == "" {
		return fmt.Errorf("GitHub resolve thread task missing pr_url or comment_id metadata")
	}

	commentID, err := strconv.Atoi(commentIDStr)
	if err != nil {
		return fmt.Errorf("invalid comment_id: %s", commentIDStr)
	}

	logging.Info("Resolving GitHub thread", "pr_url", prURL, "comment_id", commentID, "attempt", task.AttemptCount+1)

	ghClient := github.NewClient()
	if err := ghClient.ResolveReviewThread(ctx, prURL, commentID); err != nil {
		return fmt.Errorf("GitHub resolve thread failed: %w", err)
	}

	logging.Info("GitHub thread resolved successfully", "pr_url", prURL, "comment_id", commentID, "work_id", workID)

	return nil
}
