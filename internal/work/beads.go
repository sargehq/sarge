package work

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/beads"
)

// CollectIssueIDsForAutomatedWorkflow collects all issue IDs to include in the workflow.
// It includes:
// - The issue itself
// - All children recursively (parent-child dependents)
// - All blocked issues recursively (blocks dependents)
// - For issues without children/blocked, all transitive dependencies
func CollectIssueIDsForAutomatedWorkflow(ctx context.Context, beadID string, beadsReader beads.Reader) ([]string, error) {
	if beadsReader == nil {
		return nil, fmt.Errorf("beads reader is nil")
	}

	// First, get the main issue
	mainIssue, err := beadsReader.GetBead(ctx, beadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bead %s: %w", beadID, err)
	}
	if mainIssue == nil {
		return nil, fmt.Errorf("bead %s not found", beadID)
	}

	// Check if this issue has children or blocked issues
	var hasChildrenOrBlocked bool
	for _, dep := range mainIssue.Dependents {
		if dep.Type == "parent-child" || dep.Type == "blocks" {
			hasChildrenOrBlocked = true
			break
		}
	}

	if hasChildrenOrBlocked {
		// Collect all children and blocked issues recursively
		allIssueIDs, err := collectChildrenAndBlocked(ctx, beadID, beadsReader)
		if err != nil {
			return nil, fmt.Errorf("failed to collect children and blocked for %s: %w", beadID, err)
		}

		// Filter out closed issues
		var result []string
		for _, id := range allIssueIDs {
			issue, err := beadsReader.GetBead(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get bead %s: %w", id, err)
			}
			if issue != nil && issue.Status != beads.StatusClosed {
				result = append(result, id)
			}
		}
		return result, nil
	}

	// For regular issues, collect transitive dependencies
	transitiveIssues, err := beadsReader.GetTransitiveDependencies(ctx, beadID)
	if err != nil {
		return nil, err
	}

	// Extract issue IDs, filtering out closed issues
	var issueIDs []string
	for _, issue := range transitiveIssues {
		if issue.Status != beads.StatusClosed {
			issueIDs = append(issueIDs, issue.ID)
		}
	}

	return issueIDs, nil
}

// collectChildrenAndBlocked recursively collects all children (parent-child) and
// blocked issues (blocks) for a given bead.
func collectChildrenAndBlocked(ctx context.Context, beadID string, beadsReader beads.Reader) ([]string, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(id string) error
	collect = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true

		// Add this bead first
		orderedIDs = append(orderedIDs, id)

		// Get this bead to find its dependents
		result, err := beadsReader.GetBeadsWithDeps(ctx, []string{id})
		if err != nil {
			return err
		}

		// Recursively collect all children and blocked issues
		for _, dep := range result.Dependents[id] {
			if (dep.Type == "parent-child" || dep.Type == "blocks") && !visited[dep.IssueID] {
				if err := collect(dep.IssueID); err != nil {
					return err
				}
			}
		}

		return nil
	}

	if err := collect(beadID); err != nil {
		return nil, err
	}

	return orderedIDs, nil
}
