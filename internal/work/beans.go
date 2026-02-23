package work

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/beans"
)

// CollectIssueIDsForAutomatedWorkflow collects all issue IDs to include in the workflow.
// It includes:
// - The issue itself
// - All children recursively (parent-child dependents)
// - All blocked issues recursively (blocks dependents)
// - For issues without children/blocked, all transitive dependencies
func CollectIssueIDsForAutomatedWorkflow(ctx context.Context, beanID string, beansReader beans.Reader) ([]string, error) {
	if beansReader == nil {
		return nil, fmt.Errorf("beans reader is nil")
	}

	// First, get the main issue
	mainIssue, err := beansReader.GetBean(ctx, beanID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bean %s: %w", beanID, err)
	}
	if mainIssue == nil {
		return nil, fmt.Errorf("bean %s not found", beanID)
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
		allIssueIDs, err := collectChildrenAndBlocked(ctx, beanID, beansReader)
		if err != nil {
			return nil, fmt.Errorf("failed to collect children and blocked for %s: %w", beanID, err)
		}

		// Filter out closed issues
		var result []string
		for _, id := range allIssueIDs {
			issue, err := beansReader.GetBean(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("failed to get bean %s: %w", id, err)
			}
			if issue != nil && issue.Status != beans.StatusCompleted {
				result = append(result, id)
			}
		}
		return result, nil
	}

	// For regular issues, collect transitive dependencies
	transitiveIssues, err := beansReader.GetTransitiveDependencies(ctx, beanID)
	if err != nil {
		return nil, err
	}

	// Extract issue IDs, filtering out closed issues
	var issueIDs []string
	for _, issue := range transitiveIssues {
		if issue.Status != beans.StatusCompleted {
			issueIDs = append(issueIDs, issue.ID)
		}
	}

	return issueIDs, nil
}

// collectChildrenAndBlocked recursively collects all children (parent-child) and
// blocked issues (blocks) for a given bean.
func collectChildrenAndBlocked(ctx context.Context, beanID string, beansReader beans.Reader) ([]string, error) {
	visited := make(map[string]bool)
	var orderedIDs []string

	var collect func(id string) error
	collect = func(id string) error {
		if visited[id] {
			return nil
		}
		visited[id] = true

		// Add this bean first
		orderedIDs = append(orderedIDs, id)

		// Get this bean to find its dependents
		result, err := beansReader.GetBeansWithDeps(ctx, []string{id})
		if err != nil {
			return err
		}

		// Recursively collect all children and blocking issues
		for _, dep := range result.Dependents[id] {
			if (dep.Type == "parent-child" || dep.Type == "blocking") && !visited[dep.BeanID] {
				if err := collect(dep.BeanID); err != nil {
					return err
				}
			}
		}

		return nil
	}

	if err := collect(beanID); err != nil {
		return nil, err
	}

	return orderedIDs, nil
}
