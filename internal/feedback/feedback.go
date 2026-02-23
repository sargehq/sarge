package feedback

import (
	"context"
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/project"
)

// processPRFeedbackQuiet processes PR feedback without outputting to stdout.
// This is used by the scheduler to avoid interfering with the TUI.
// Returns the number of beans created and any error.
func processPRFeedbackQuiet(ctx context.Context, proj *project.Project, database *db.DB, workID string) (int, error) {
	return processPRFeedbackInternal(ctx, proj, database, workID, true)
}

// ProcessPRFeedback processes PR feedback for a work and creates beans.
// This is an internal function that can be called directly.
// Returns the number of beans created and any error.
func ProcessPRFeedback(ctx context.Context, proj *project.Project, database *db.DB, workID string) (int, error) {
	return processPRFeedbackInternal(ctx, proj, database, workID, false)
}

// processPRFeedbackInternal is the actual implementation with output control
func processPRFeedbackInternal(ctx context.Context, proj *project.Project, database *db.DB, workID string, quiet bool) (int, error) {
	// Get work details
	work, err := database.GetWork(ctx, workID)
	if err != nil {
		return 0, fmt.Errorf("failed to get work %s: %w", workID, err)
	}

	if work.PRURL == "" {
		return 0, fmt.Errorf("work %s does not have an associated PR URL", workID)
	}

	if work.RootIssueID == "" {
		return 0, fmt.Errorf("work %s does not have a root issue ID", workID)
	}

	if !quiet {
		fmt.Printf("Processing PR feedback for work %s\n", workID)
		fmt.Printf("PR URL: %s\n", work.PRURL)
		fmt.Printf("Root issue: %s\n", work.RootIssueID)
	}

	// Create integration with project context to enable Claude-based log analysis
	integration := NewIntegrationWithProject(proj, workID)

	// Extract and store PR status (CI status, approval status)
	if !quiet {
		fmt.Println("\nExtracting PR status...")
	}
	prStatusInfo, err := integration.ExtractPRStatus(ctx, work.PRURL)
	if err != nil {
		if !quiet {
			fmt.Printf("Warning: failed to extract PR status: %v\n", err)
		}
	} else {
		// Check if status has changed and update the database
		statusChanged := UpdatePRStatusIfChanged(ctx, database, work, prStatusInfo, quiet)
		if statusChanged && !quiet {
			fmt.Println("PR status has changed, marked as unseen")
		}
	}

	// Fetch and process PR feedback
	if !quiet {
		fmt.Println("\nFetching PR feedback...")
	}
	feedbackItems, err := integration.FetchAndStoreFeedback(ctx, work.PRURL)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch PR feedback: %w", err)
	}

	if len(feedbackItems) == 0 {
		if !quiet {
			fmt.Println("No actionable feedback found.")
		}
		return 0, nil
	}

	if !quiet {
		fmt.Printf("Found %d actionable feedback items:\n\n", len(feedbackItems))
	}

	// Store feedback in database and create beans
	createdBeans := []string{}
	beansPath := proj.BeansPath()

	for i, item := range feedbackItems {
		// Check if this is a reply to an existing comment
		if inReplyToID := item.GetInReplyToID(); inReplyToID != "" {
			// This is a reply - find the original comment's bean and add this as a comment
			parentFeedback, err := database.GetFeedbackBySourceID(ctx, workID, inReplyToID)
			if err != nil {
				if !quiet {
					fmt.Printf("%d. [ERROR] Failed to look up parent comment %s: %v\n", i+1, inReplyToID, err)
				}
				continue
			}

			if parentFeedback == nil || parentFeedback.BeanID == nil {
				if !quiet {
					fmt.Printf("%d. [SKIP - Reply to untracked comment] %s\n", i+1, item.Title)
				}
				continue
			}

			// Check if this reply was already processed
			if sourceID := item.GetSourceID(); sourceID != "" {
				exists, err := database.HasExistingFeedbackBySourceID(ctx, workID, sourceID)
				if err == nil && exists {
					if !quiet {
						fmt.Printf("%d. [SKIP - Reply already processed] %s\n", i+1, item.Title)
					}
					continue
				}
			}

			// Add the reply as a comment to the existing bean
			commentText := fmt.Sprintf("Reply from %s:\n\n%s", item.GetSourceName(), item.Description)
			if err := beans.NewCLI(beansPath).AddComment(ctx, *parentFeedback.BeanID, commentText); err != nil {
				if !quiet {
					fmt.Printf("%d. [ERROR] Failed to add comment to bean %s: %v\n", i+1, *parentFeedback.BeanID, err)
				}
				continue
			}

			if !quiet {
				fmt.Printf("%d. [REPLY] Added comment to bean %s\n", i+1, *parentFeedback.BeanID)
			}

			// Store this reply in the database to track it was processed
			// (using the same bean_id as the parent)
			prFeedback, err := database.CreatePRFeedbackFromParams(ctx, db.CreatePRFeedbackParams{
				WorkID:       workID,
				PRURL:        work.PRURL,
				FeedbackType: string(item.Type),
				Title:        item.Title,
				Description:  item.Description,
				Source:       item.Source,
				Context:      item.ToFeedbackContext(),
				Priority:     item.Priority,
			})
			if err == nil {
				_ = database.MarkFeedbackProcessed(ctx, prFeedback.ID, *parentFeedback.BeanID)
			}

			continue
		}

		// Check if feedback already exists
		// Prefer checking by source_id (unique comment ID) if available
		var exists bool
		var checkErr error

		if sourceID := item.GetSourceID(); sourceID != "" {
			// Use the unique source ID for deduplication
			exists, checkErr = database.HasExistingFeedbackBySourceID(ctx, workID, sourceID)
		} else {
			// Fallback to title + source_type + source_name check (less reliable)
			exists, checkErr = database.HasExistingFeedback(ctx, workID, item.Title, item.Source.Type, item.Source.Name)
		}

		if checkErr != nil {
			if !quiet {
				fmt.Printf("Error checking existing feedback: %v\n", checkErr)
			}
			continue
		}

		if exists {
			if !quiet {
				fmt.Printf("%d. [SKIP - Already processed] %s\n", i+1, item.Title)
			}
			continue
		}

		if !quiet {
			fmt.Printf("%d. %s\n", i+1, item.Title)
			fmt.Printf("   Type: %s | Priority: P%d | Source: %s\n", item.Type, item.Priority, item.GetSourceName())
		}

		// Store feedback in database using structured source info
		prFeedback, err := database.CreatePRFeedbackFromParams(ctx, db.CreatePRFeedbackParams{
			WorkID:       workID,
			PRURL:        work.PRURL,
			FeedbackType: string(item.Type),
			Title:        item.Title,
			Description:  item.Description,
			Source:       item.Source,
			Context:      item.ToFeedbackContext(),
			Priority:     item.Priority,
		})
		if err != nil {
			if !quiet {
				fmt.Printf("   Error storing feedback: %v\n", err)
			}
			continue
		}

		beanInfo := BeanInfo{
			Title:       item.Title,
			Description: item.Description,
			Type:        GetBeanType(item.Type),
			Priority:    beans.PriorityFromInt(item.Priority),
			ParentID:    work.RootIssueID,
			Labels:      []string{"from-pr-feedback"},
			SourceURL:   item.Source.URL,
		}

		// Create bean using beans package
		beanID, err := integration.CreateBeanFromFeedback(ctx, beansPath, beanInfo)
		if err != nil {
			if !quiet {
				fmt.Printf("   Error creating bean: %v\n", err)
			}
			continue
		}

		if !quiet {
			fmt.Printf("   Created bean: %s\n", beanID)
		}
		createdBeans = append(createdBeans, beanID)

		// Post back to GitHub comment if this feedback came from a comment
		// Use typed context to determine comment type and ID
		if item.Source.ID != "" {
			var commentID int64
			isReviewComment := false

			// Check for review comment context
			if item.Review != nil && item.Review.CommentID != 0 {
				commentID = item.Review.CommentID
				isReviewComment = true
			} else if item.IssueComment != nil && item.IssueComment.CommentID != 0 {
				// Check for issue comment context
				commentID = item.IssueComment.CommentID
			}

			if commentID != 0 {
				// Create the acknowledgment message with marker to prevent feedback loop
				// The sarge-bot marker is filtered out by isActionableComment() in processor.go
				ackMessage := fmt.Sprintf("<!-- sarge-bot -->✅ Created tracking issue **%s** for this feedback.\n\nTitle: %s\nPriority: P%d",
					beanID, item.Title, item.Priority)

				client := github.NewClient()
				var postErr error
				if isReviewComment {
					postErr = client.PostReviewReply(ctx, work.PRURL, int(commentID), ackMessage)
				} else {
					postErr = client.PostReplyToComment(ctx, work.PRURL, int(commentID), ackMessage)
				}

				if postErr != nil {
					if !quiet {
						fmt.Printf("   Warning: Failed to post acknowledgment to GitHub: %v\n", postErr)
					}
				} else {
					if !quiet {
						fmt.Printf("   Posted acknowledgment to GitHub comment\n")
					}
				}
			}
		}

		// Mark feedback as processed
		if err := database.MarkFeedbackProcessed(ctx, prFeedback.ID, beanID); err != nil {
			if !quiet {
				fmt.Printf("   Warning: Failed to mark feedback as processed: %v\n", err)
			}
		}
	}

	// Summary
	if !quiet {
		fmt.Printf("\n=== Summary ===\n")
		fmt.Printf("Total feedback items: %d\n", len(feedbackItems))
		fmt.Printf("Beans created: %d\n", len(createdBeans))

		if len(createdBeans) > 0 {
			fmt.Println("\nTo add these beans to the work, run:")
			fmt.Printf("  sarge work add %s\n", strings.Join(createdBeans, " "))
		}
	}

	return len(createdBeans), nil
}
