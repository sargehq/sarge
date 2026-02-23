package feedback

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/sargehq/sarge/internal/beads"
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

// BeadInfo represents information for creating a bead from feedback.
type BeadInfo struct {
	Title       string
	Description string
	Type        string // task, bug, feature
	Priority    int    // 0-4
	ParentID    string // Parent issue ID (root_issue_id)
	Labels      []string
	SourceURL   string // GitHub URL for external reference generation
}

// Integration handles the integration between GitHub PR feedback and beads.
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

// CreateBeadFromFeedback creates a bead using the beads package with proper input validation.
func (i *Integration) CreateBeadFromFeedback(ctx context.Context, beadDir string, beadInfo BeadInfo) (string, error) {
	// Validate and sanitize all inputs to prevent injection attacks
	title, err := validateAndSanitizeInput(beadInfo.Title, 200, "title")
	if err != nil {
		return "", fmt.Errorf("invalid title: %w", err)
	}

	// Validate bead type
	if err := validateBeadType(beadInfo.Type); err != nil {
		return "", err
	}

	// Validate priority
	if err := validatePriority(beadInfo.Priority); err != nil {
		return "", err
	}

	// Validate and sanitize parent ID
	parentID, err := validateAndSanitizeInput(beadInfo.ParentID, 100, "parent ID")
	if err != nil {
		return "", fmt.Errorf("invalid parent ID: %w", err)
	}

	// Validate and sanitize description (allow longer descriptions)
	description, err := validateAndSanitizeInput(beadInfo.Description, 5000, "description")
	if err != nil {
		return "", fmt.Errorf("invalid description: %w", err)
	}

	// Prepare external reference if we have a source URL
	var externalRef string
	if beadInfo.SourceURL != "" {
		// Validate the source URL
		sanitizedURL, err := validateAndSanitizeInput(beadInfo.SourceURL, 500, "source URL")
		if err == nil {
			// Create a short reference for the external-ref field
			// For example, "gh-comment-123456" or "gh-pr-123"
			ref := fmt.Sprintf("gh-%s", extractGitHubID(sanitizedURL))
			// Validate the external reference
			if sanitizedRef, err := validateAndSanitizeInput(ref, 100, "external reference"); err == nil {
				externalRef = sanitizedRef
			}
		}
		// If validation fails, we skip adding the external reference but continue
	}

	// Validate labels
	var validLabels []string
	for _, label := range beadInfo.Labels {
		// Validate each label
		sanitizedLabel, err := validateAndSanitizeInput(label, 50, "label")
		if err != nil {
			// Skip invalid labels but continue
			continue
		}
		validLabels = append(validLabels, sanitizedLabel)
	}

	// Create bead using the beads package
	createOpts := beads.CreateOptions{
		Title:       title,
		Type:        beadInfo.Type,     // Already validated
		Priority:    beadInfo.Priority, // Already validated as int
		Parent:      parentID,
		Description: description,
		Labels:      validLabels,
		ExternalRef: externalRef,
	}

	beanID, err := beads.NewCLI(beadDir).Create(ctx, createOpts)
	if err != nil {
		return "", fmt.Errorf("failed to create bead: %w", err)
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

// validateBeadType ensures the bead type is valid.
func validateBeadType(beadType string) error {
	validTypes := map[string]bool{
		"bug":     true,
		"feature": true,
		"task":    true,
		"epic":    true,
	}

	if !validTypes[strings.ToLower(beadType)] {
		return fmt.Errorf("invalid bead type: %s", beadType)
	}
	return nil
}

// validatePriority ensures the priority is within valid range.
func validatePriority(priority int) error {
	if priority < 0 || priority > 4 {
		return errors.New("priority must be between 0 and 4")
	}
	return nil
}

// GetBeadType converts a feedback type to a bead type string.
func GetBeadType(feedbackType github.FeedbackType) string {
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
