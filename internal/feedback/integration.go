package feedback

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/project"
)

// Package-level compiled regexes for extractGitHubID
var (
	reviewCommentIDPattern = regexp.MustCompile(`#discussion_r(\d+)`)
	issueCommentIDPattern  = regexp.MustCompile(`#issuecomment-(\d+)`)
	prNumberPattern        = regexp.MustCompile(`/pull/(\d+)`)
	issueNumberPattern     = regexp.MustCompile(`/issues/(\d+)`)
)

// BeanInfo represents information for creating a bean from feedback.
type BeanInfo struct {
	Title       string
	Description string
	Type        string // task, bug, feature
	Priority    string // critical, high, normal, low, deferred
	ParentID    string // Parent issue ID (root_issue_id)
	Labels      []string
	SourceURL   string // GitHub URL for tag generation
}

// Integration handles the integration between GitHub PR feedback and beans.
type Integration struct {
	client    github.ClientInterface
	processor *FeedbackProcessor
}

// NewIntegrationWithProject creates a new feedback integration with project context.
// This enables Claude-based log analysis when configured.
func NewIntegrationWithProject(proj *project.Project, workID string) *Integration {
	client := github.NewClient()
	processor := NewFeedbackProcessorWithProject(client, proj, workID)

	return &Integration{
		client:    client,
		processor: processor,
	}
}

// FetchAndStoreFeedback fetches PR feedback and returns actionable items.
func (i *Integration) FetchAndStoreFeedback(ctx context.Context, prURL string) ([]github.FeedbackItem, error) {
	return i.processor.ProcessPRFeedback(ctx, prURL)
}

// ExtractPRStatus fetches a PR and extracts CI and approval status.
func (i *Integration) ExtractPRStatus(ctx context.Context, prURL string) (*PRStatusInfo, error) {
	status, err := i.client.GetPRStatus(ctx, prURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR status: %w", err)
	}

	return ExtractStatusFromPRStatus(status), nil
}

// CreateBeanFromFeedback creates a bean using the beans package with proper input validation.
func (i *Integration) CreateBeanFromFeedback(ctx context.Context, beanDir string, beanInfo BeanInfo) (string, error) {
	// Validate and sanitize all inputs to prevent injection attacks
	title, err := validateAndSanitizeInput(beanInfo.Title, 200, "title")
	if err != nil {
		return "", fmt.Errorf("invalid title: %w", err)
	}

	// Validate bean type
	if err := validateBeanType(beanInfo.Type); err != nil {
		return "", err
	}

	// Validate priority
	if err := validatePriority(beanInfo.Priority); err != nil {
		return "", err
	}

	// Validate and sanitize parent ID
	parentID, err := validateAndSanitizeInput(beanInfo.ParentID, 100, "parent ID")
	if err != nil {
		return "", fmt.Errorf("invalid parent ID: %w", err)
	}

	// Validate and sanitize description (allow longer descriptions)
	description, err := validateAndSanitizeInput(beanInfo.Description, 5000, "description")
	if err != nil {
		return "", fmt.Errorf("invalid description: %w", err)
	}

	// Build tags from labels and source URL
	var validTags []string
	for _, label := range beanInfo.Labels {
		sanitizedLabel, err := validateAndSanitizeInput(label, 50, "label")
		if err != nil {
			continue
		}
		validTags = append(validTags, sanitizedLabel)
	}
	if beanInfo.SourceURL != "" {
		if sanitizedURL, err := validateAndSanitizeInput(beanInfo.SourceURL, 500, "source URL"); err == nil {
			ref := fmt.Sprintf("gh-%s", extractGitHubID(sanitizedURL))
			if sanitizedRef, err := validateAndSanitizeInput(ref, 100, "source tag"); err == nil {
				validTags = append(validTags, sanitizedRef)
			}
		}
	}

	// Create bean using the beans package
	createOpts := beans.CreateOptions{
		Title:    title,
		Type:     beanInfo.Type,
		Priority: beanInfo.Priority,
		Parent:   parentID,
		Body:     description,
		Tags:     validTags,
	}

	beanID, err := beans.NewCLI(beanDir).Create(ctx, createOpts)
	if err != nil {
		return "", fmt.Errorf("failed to create bean: %w", err)
	}

	return beanID, nil
}

// extractGitHubID extracts a GitHub identifier from a URL
// For example: from "https://github.com/owner/repo/pull/123#issuecomment-456789"
// returns "comment-456789"
// For review comments: "https://github.com/owner/repo/pull/123#discussion_r789"
// returns "review-comment-789"
func extractGitHubID(url string) string {
	// Try to extract review comment ID (e.g., #discussion_r123456)
	if matches := reviewCommentIDPattern.FindStringSubmatch(url); len(matches) > 1 {
		return "review-comment-" + matches[1]
	}

	// Try to extract regular comment ID (e.g., #issuecomment-456789)
	if matches := issueCommentIDPattern.FindStringSubmatch(url); len(matches) > 1 {
		return "comment-" + matches[1]
	}

	// Try to extract PR number
	if matches := prNumberPattern.FindStringSubmatch(url); len(matches) > 1 {
		return "pr-" + matches[1]
	}

	// Try to extract issue number
	if matches := issueNumberPattern.FindStringSubmatch(url); len(matches) > 1 {
		return "issue-" + matches[1]
	}

	// Default to using the last part of the URL path
	parts := strings.Split(strings.TrimSuffix(url, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "github"
}

// validateAndSanitizeInput validates and sanitizes user input to prevent injection attacks.
// It ensures the input is safe to pass to shell commands.
func validateAndSanitizeInput(input string, maxLength int, fieldName string) (string, error) {
	// Check for null bytes which could cause issues in command execution
	if strings.Contains(input, "\x00") {
		return "", fmt.Errorf("%s contains null bytes", fieldName)
	}

	// Trim whitespace
	input = strings.TrimSpace(input)

	// Check length
	if len(input) == 0 {
		return "", fmt.Errorf("%s cannot be empty", fieldName)
	}
	if maxLength > 0 && len(input) > maxLength {
		return "", fmt.Errorf("%s exceeds maximum length of %d characters", fieldName, maxLength)
	}

	// Remove any control characters except newlines and tabs
	var sanitized strings.Builder
	for _, r := range input {
		if r == '\n' || r == '\t' || (r >= 32 && r < 127) || r > 127 {
			// Allow printable ASCII, newlines, tabs, and UTF-8 characters
			sanitized.WriteRune(r)
		} else if unicode.IsSpace(r) {
			// Replace other whitespace with regular space
			sanitized.WriteRune(' ')
		}
		// Skip other control characters
	}

	result := sanitized.String()
	if len(result) == 0 {
		return "", fmt.Errorf("%s contains only invalid characters", fieldName)
	}

	return result, nil
}

// validateBeanType ensures the bean type is valid.
func validateBeanType(beanType string) error {
	validTypes := map[string]bool{
		"bug":     true,
		"feature": true,
		"task":    true,
		"epic":    true,
	}

	if !validTypes[strings.ToLower(beanType)] {
		return fmt.Errorf("invalid bean type: %s", beanType)
	}
	return nil
}

// validatePriority ensures the priority is a valid beans priority string.
func validatePriority(priority string) error {
	valid := map[string]bool{
		beans.PriorityCritical: true,
		beans.PriorityHigh:     true,
		beans.PriorityNormal:   true,
		beans.PriorityLow:      true,
		beans.PriorityDeferred: true,
	}
	if !valid[priority] {
		return fmt.Errorf("invalid priority: %s", priority)
	}
	return nil
}

// GetBeanType converts a feedback type to a bean type string.
func GetBeanType(feedbackType github.FeedbackType) string {
	switch feedbackType {
	case github.FeedbackTypeTest, github.FeedbackTypeBuild, github.FeedbackTypeCI, github.FeedbackTypeConflict:
		return "bug"
	case github.FeedbackTypeLint, github.FeedbackTypeSecurity:
		return "task"
	case github.FeedbackTypeReview:
		return "task"
	default:
		return "task"
	}
}
