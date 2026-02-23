package feedback

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// CheckAndResolveComments checks for feedback items and resolves those with closed beads.
func CheckAndResolveComments(ctx context.Context, proj *project.Project, workID string) error {
	// Get unresolved feedback items for this work
	feedbacks, err := proj.DB.GetUnresolvedFeedbackForWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get unresolved feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil
	}

	logging.Debug("checking feedback items for resolution", "count", len(feedbacks))

	var resolvedCount int
	for _, fb := range feedbacks {
		if fb.BeanID == nil || fb.SourceID == nil {
			continue
		}

		// Check if the bead is actually closed
		bead, err := proj.Beads.GetBean(ctx, *fb.BeanID)
		if err != nil {
			logging.Error("failed to get bead", "bead_id", *fb.BeanID, "error", err)
			continue
		}

		if bead == nil || bead.Status != beans.StatusCompleted {
			continue
		}

		// Resolve this feedback item
		if err := resolveFeedbackItem(ctx, proj.DB, workID, fb, ""); err != nil {
			logging.Error("failed to resolve feedback", "feedback_id", fb.ID, "error", err)
			continue
		}
		resolvedCount++
	}

	if resolvedCount > 0 {
		logging.Debug("resolved feedback items", "count", resolvedCount)
	}
	return nil
}

// ResolveFeedbackForBeans posts resolution comments on GitHub for closed beads.
// It checks the provided beads and posts resolution comments for any associated
// unresolved feedback items. Uses the transactional outbox pattern: atomically marks
// feedback as resolved and schedules comment tasks, then attempts optimistic execution.
func ResolveFeedbackForBeans(ctx context.Context, database *db.DB, beadClient *beans.Client, workID string, closedBeanIDs []string) error {
	if len(closedBeanIDs) == 0 {
		return nil
	}

	// Get unresolved feedback for the closed beads
	feedbacks, err := database.GetUnresolvedFeedbackForBeans(ctx, closedBeanIDs)
	if err != nil {
		return fmt.Errorf("failed to get unresolved feedback: %w", err)
	}

	if len(feedbacks) == 0 {
		return nil
	}

	fmt.Printf("\nResolving %d GitHub feedback items for closed beads...\n", len(feedbacks))

	for _, fb := range feedbacks {
		if fb.BeanID == nil || fb.SourceID == nil {
			continue
		}

		// Get bead details for close reason
		bead, err := beadClient.GetBean(ctx, *fb.BeanID)
		if err != nil {
			fmt.Printf("Warning: failed to get bead %s: %v\n", *fb.BeanID, err)
			continue
		}

		if bead == nil {
			continue
		}

		if err := resolveFeedbackItem(ctx, database, workID, fb, ""); err != nil {
			fmt.Printf("Warning: failed to resolve feedback %s: %v\n", fb.ID, err)
			continue
		}
	}

	return nil
}

// resolveFeedbackItem handles the resolution of a single feedback item.
// It schedules GitHub comment/thread resolution tasks and attempts optimistic execution.
func resolveFeedbackItem(ctx context.Context, database *db.DB, workID string, fb db.PRFeedback, closeReason string) error {
	// Construct resolution message with marker to prevent feedback loop
	// The sarge-bot marker is filtered out by isActionableComment() in processor.go
	resolutionMessage := fmt.Sprintf("<!-- sarge-bot -->✅ Resolved in work %s (issue %s)", workID, *fb.BeanID)
	if closeReason != "" {
		resolutionMessage = fmt.Sprintf("<!-- sarge-bot -->✅ Resolved in work %s (issue %s): %s", workID, *fb.BeanID, closeReason)
	}

	// Parse PR URL to get owner/repo/pr_number
	// Expected format: https://github.com/owner/repo/pull/123
	parts := strings.Split(fb.PRURL, "/")
	if len(parts) < 7 || parts[5] != "pull" {
		return fmt.Errorf("invalid PR URL format: %s", fb.PRURL)
	}

	isReviewComment := fb.IsReviewComment()
	scheduledAt := time.Now().Add(db.OptimisticExecutionDelay)

	// Build the list of tasks to schedule
	var tasksToSchedule []db.ScheduledTaskParams

	// GitHub comment task
	commentIdempotencyKey := fmt.Sprintf("github-comment-%s-%s", workID, fb.ID)
	commentMetadata := map[string]string{
		"pr_url": fb.PRURL,
		"body":   resolutionMessage,
	}
	if isReviewComment {
		commentMetadata["reply_to_id"] = *fb.SourceID
	}
	tasksToSchedule = append(tasksToSchedule, db.ScheduledTaskParams{
		WorkID:         workID,
		TaskType:       db.TaskTypeGitHubComment,
		ScheduledAt:    scheduledAt,
		Metadata:       commentMetadata,
		IdempotencyKey: commentIdempotencyKey,
		MaxAttempts:    db.DefaultMaxAttempts,
	})

	// For review comments, also add thread resolution task
	var resolveIdempotencyKey string
	var commentID int
	if isReviewComment {
		var err error
		commentID, err = strconv.Atoi(*fb.SourceID)
		if err != nil {
			return fmt.Errorf("invalid comment ID %s: %w", *fb.SourceID, err)
		}

		resolveIdempotencyKey = fmt.Sprintf("github-resolve-%s-%s", workID, fb.ID)
		tasksToSchedule = append(tasksToSchedule, db.ScheduledTaskParams{
			WorkID:      workID,
			TaskType:    db.TaskTypeGitHubResolveThread,
			ScheduledAt: scheduledAt,
			Metadata: map[string]string{
				"pr_url":     fb.PRURL,
				"comment_id": *fb.SourceID,
			},
			IdempotencyKey: resolveIdempotencyKey,
			MaxAttempts:    db.DefaultMaxAttempts,
		})
	}

	// Atomically mark feedback resolved and schedule all tasks in a single transaction
	if err := database.MarkFeedbackResolvedAndScheduleTasks(ctx, fb.ID, tasksToSchedule); err != nil {
		return fmt.Errorf("failed to mark feedback as resolved and schedule tasks: %w", err)
	}

	// Attempt immediate comment post (optimistic execution)
	ghClient := github.NewClient()
	var commentErr error
	if isReviewComment {
		sourceID, err := strconv.Atoi(*fb.SourceID)
		if err != nil {
			return fmt.Errorf("invalid source ID %s: %w", *fb.SourceID, err)
		}
		commentErr = ghClient.PostReviewReply(ctx, fb.PRURL, sourceID, resolutionMessage)
	} else {
		commentErr = ghClient.PostPRComment(ctx, fb.PRURL, resolutionMessage)
	}
	if commentErr != nil {
		logging.Warn("Initial GitHub comment failed, scheduler will retry", "error", commentErr, "work_id", workID, "bead_id", *fb.BeanID)
		fmt.Printf("Warning: initial comment post failed, will retry in background: %v\n", commentErr)
	} else {
		// Success - mark scheduled task as completed
		if markErr := database.MarkTaskCompletedByIdempotencyKey(ctx, commentIdempotencyKey); markErr != nil {
			logging.Warn("failed to mark github comment task as completed", "error", markErr, "work_id", workID)
		}
		fmt.Printf("Successfully posted resolution comment for bead %s on GitHub\n", *fb.BeanID)
	}

	// For review comments, also attempt immediate thread resolution
	if isReviewComment {
		if err := ghClient.ResolveReviewThread(ctx, fb.PRURL, commentID); err != nil {
			logging.Warn("Initial GitHub thread resolution failed, scheduler will retry", "error", err, "work_id", workID, "comment_id", commentID)
			fmt.Printf("Warning: initial thread resolution failed, will retry in background: %v\n", err)
		} else {
			// Success - mark scheduled task as completed
			if err := database.MarkTaskCompletedByIdempotencyKey(ctx, resolveIdempotencyKey); err != nil {
				logging.Warn("failed to mark github resolve thread task as completed", "error", err, "work_id", workID)
			}
			fmt.Printf("Successfully resolved review thread for bead %s\n", *fb.BeanID)
		}
	}

	return nil
}
