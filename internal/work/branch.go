package work

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/git"
)

// GenerateBranchNameFromIssue creates a git-friendly branch name from an issue's title.
// It converts the title to lowercase, replaces spaces with hyphens,
// removes special characters, and prefixes with "feat/".
func GenerateBranchNameFromIssue(issue *beans.Bean) string {
	title := issue.Title

	// Convert to lowercase
	title = strings.ToLower(title)

	// Replace spaces and underscores with hyphens
	title = strings.ReplaceAll(title, " ", "-")
	title = strings.ReplaceAll(title, "_", "-")

	// Remove characters that aren't alphanumeric or hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	title = reg.ReplaceAllString(title, "")

	// Collapse multiple hyphens into one
	reg = regexp.MustCompile(`-+`)
	title = reg.ReplaceAllString(title, "-")

	// Trim leading/trailing hyphens
	title = strings.Trim(title, "-")

	// Truncate if too long (git branch names can be long, but let's be reasonable)
	if len(title) > 50 {
		title = title[:50]
		// Don't end with a hyphen
		title = strings.TrimRight(title, "-")
	}

	// Add prefix based on common conventions
	return fmt.Sprintf("feat/%s", title)
}

// GenerateBranchNameFromIssues creates a git-friendly branch name from multiple issues' titles.
// For a single issue, it uses that issue's title.
// For multiple issues, it combines titles (truncated) or uses a generic name if too long.
func GenerateBranchNameFromIssues(issues []*beans.Bean) string {
	if len(issues) == 0 {
		return "feat/automated-work"
	}

	if len(issues) == 1 {
		return GenerateBranchNameFromIssue(issues[0])
	}

	// For multiple issues, combine their titles
	var titles []string
	for _, issue := range issues {
		titles = append(titles, issue.Title)
	}
	combined := strings.Join(titles, " and ")

	// Convert to lowercase
	combined = strings.ToLower(combined)

	// Replace spaces and underscores with hyphens
	combined = strings.ReplaceAll(combined, " ", "-")
	combined = strings.ReplaceAll(combined, "_", "-")

	// Remove characters that aren't alphanumeric or hyphens
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	combined = reg.ReplaceAllString(combined, "")

	// Collapse multiple hyphens into one
	reg = regexp.MustCompile(`-+`)
	combined = reg.ReplaceAllString(combined, "-")

	// Trim leading/trailing hyphens
	combined = strings.Trim(combined, "-")

	// Truncate if too long (git branch names can be long, but let's be reasonable)
	if len(combined) > 50 {
		combined = combined[:50]
		// Don't end with a hyphen
		combined = strings.TrimRight(combined, "-")
	}

	return fmt.Sprintf("feat/%s", combined)
}

// EnsureUniqueBranchName checks if a branch already exists and appends a suffix if needed.
// Returns a unique branch name that doesn't conflict with existing branches.
// The gitOps parameter allows dependency injection for testing.
func EnsureUniqueBranchName(ctx context.Context, gitOps git.Operations, repoPath, baseName string) (string, error) {
	// Check if the base name is available
	if !gitOps.BranchExists(ctx, repoPath, baseName) {
		return baseName, nil
	}

	// Try appending suffixes until we find an available name
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", baseName, i)
		if !gitOps.BranchExists(ctx, repoPath, candidate) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find unique branch name after 100 attempts (base: %s)", baseName)
}

// ParseBeanIDs parses a comma-delimited string of bead IDs into a slice.
// It trims whitespace from each ID and filters out empty strings.
func ParseBeanIDs(beanIDStr string) []string {
	if beanIDStr == "" {
		return nil
	}

	parts := strings.Split(beanIDStr, ",")
	var result []string
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id != "" {
			result = append(result, id)
		}
	}
	return result
}
