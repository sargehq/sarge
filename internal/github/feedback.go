package github

import (
	"fmt"
)

// FeedbackType categorizes the type of feedback.
type FeedbackType string

const (
	FeedbackTypeCI       FeedbackType = "ci_failure"
	FeedbackTypeTest     FeedbackType = "test_failure"
	FeedbackTypeLint     FeedbackType = "lint_error"
	FeedbackTypeBuild    FeedbackType = "build_error"
	FeedbackTypeReview   FeedbackType = "review_comment"
	FeedbackTypeSecurity FeedbackType = "security_issue"
	FeedbackTypeGeneral  FeedbackType = "general"
	FeedbackTypeConflict FeedbackType = "merge_conflict"
)

// FeedbackItem represents a piece of feedback from GitHub that could become a bean.
type FeedbackItem struct {
	Type        FeedbackType
	Title       string
	Description string
	Source      SourceInfo // Structured source information
	Priority    int        // 0-4 (0=critical, 4=backlog)

	// Typed context fields (only one will be set based on Source.Type)
	CICheck      *CICheckContext      // Set when Source.Type == SourceTypeCI
	Workflow     *WorkflowContext     // Set when Source.Type == SourceTypeWorkflow
	Review       *ReviewContext       // Set when Source.Type == SourceTypeReviewComment
	IssueComment *IssueCommentContext // Set when Source.Type == SourceTypeIssueComment
}

// GetSourceID returns the source ID used for resolution tracking.
// Returns empty string if not applicable.
func (f FeedbackItem) GetSourceID() string {
	return f.Source.ID
}

// GetInReplyToID returns the ID of the parent comment if this is a reply.
// Returns empty string if this is not a reply.
func (f FeedbackItem) GetInReplyToID() string {
	if f.Review != nil && f.Review.InReplyToID != 0 {
		return fmt.Sprintf("%d", f.Review.InReplyToID)
	}
	return ""
}

// GetSourceName returns a human-readable source name (e.g., "CI: test-suite").
func (f FeedbackItem) GetSourceName() string {
	switch f.Source.Type {
	case SourceTypeCI:
		return fmt.Sprintf("CI: %s", f.Source.Name)
	case SourceTypeWorkflow:
		return fmt.Sprintf("Workflow: %s", f.Source.Name)
	case SourceTypeReviewComment:
		return fmt.Sprintf("Review: %s", f.Source.Name)
	case SourceTypeIssueComment:
		return fmt.Sprintf("Comment: %s", f.Source.Name)
	default:
		return f.Source.Name
	}
}

// ToFeedbackContext creates a FeedbackContext from the item's typed context fields.
func (f FeedbackItem) ToFeedbackContext() *FeedbackContext {
	ctx := &FeedbackContext{
		CI:       f.CICheck,
		Workflow: f.Workflow,
		Review:   f.Review,
		Comment:  f.IssueComment,
	}
	// Return nil if no context is set
	if ctx.CI == nil && ctx.Workflow == nil && ctx.Review == nil && ctx.Comment == nil {
		return nil
	}
	return ctx
}
