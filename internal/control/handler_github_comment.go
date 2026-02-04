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

// HandleGitHubCommentTask handles a scheduled GitHub comment posting task.
// Returns nil on success, error on failure (caller handles retry/completion).
func HandleGitHubCommentTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	// Get comment details from metadata
	prURL := task.Metadata["pr_url"]
	body := task.Metadata["body"]
	replyToID := task.Metadata["reply_to_id"]

	if prURL == "" || body == "" {
		return fmt.Errorf("GitHub comment task missing pr_url or body metadata")
	}

	logging.Info("Posting GitHub comment", "pr_url", prURL, "attempt", task.AttemptCount+1)

	ghClient := github.NewClient()
	var err error
	if replyToID != "" {
		// Reply to a specific review comment thread
		commentID, convErr := strconv.Atoi(replyToID)
		if convErr != nil {
			return fmt.Errorf("invalid reply_to_id: %s", replyToID)
		}
		err = ghClient.PostReviewReply(ctx, prURL, commentID, body)
	} else {
		// Post a general PR comment
		err = ghClient.PostPRComment(ctx, prURL, body)
	}

	if err != nil {
		return fmt.Errorf("GitHub comment failed: %w", err)
	}

	logging.Info("GitHub comment posted successfully", "pr_url", prURL, "work_id", workID)

	return nil
}
