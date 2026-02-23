package linear

import (
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
)

// MapStatus converts Linear state type to beans status
// Linear state types: "unstarted", "started", "completed", "canceled"
// Beans statuses: "todo", "in-progress", "completed", "scrapped"
func MapStatus(state State) string {
	switch strings.ToLower(state.Type) {
	case "unstarted":
		return beans.StatusTodo
	case "started":
		return beans.StatusInProgress
	case "completed":
		return beans.StatusCompleted
	case "canceled":
		return beans.StatusScrapped
	default:
		return beans.StatusTodo
	}
}

// MapPriority converts Linear priority (0-4) to Beads priority (P0-P4)
// Linear: 0=Urgent, 1=High, 2=Medium, 3=Low, 4=No priority
// Beads: P0=Critical, P1=High, P2=Medium, P3=Low, P4=Backlog
func MapPriority(priority int) string {
	switch priority {
	case 0:
		return "P0" // Urgent -> Critical
	case 1:
		return "P1" // High -> High
	case 2:
		return "P2" // Medium -> Medium
	case 3:
		return "P3" // Low -> Low
	case 4:
		return "P4" // No priority -> Backlog
	default:
		return "P2" // Default to medium
	}
}

// MapType infers a Beads issue type from Linear data
// Returns: "task", "bug", or "feature"
func MapType(issue *Issue) string {
	// Check labels for type hints
	for _, label := range issue.Labels {
		labelLower := strings.ToLower(label.Name)
		if strings.Contains(labelLower, "bug") || strings.Contains(labelLower, "fix") {
			return "bug"
		}
		if strings.Contains(labelLower, "feature") || strings.Contains(labelLower, "enhancement") {
			return "feature"
		}
	}

	// Check title for type hints
	titleLower := strings.ToLower(issue.Title)
	if strings.Contains(titleLower, "bug") || strings.Contains(titleLower, "fix") {
		return "bug"
	}
	if strings.Contains(titleLower, "feature") || strings.Contains(titleLower, "add") {
		return "feature"
	}

	// Default to task
	return "task"
}

// BeadCreateOptions represents options for creating a bead from Linear
type BeadCreateOptions struct {
	Title       string
	Description string
	Type        string   // task, bug, feature
	Priority    string   // P0-P4
	Status      string   // open, in_progress, closed
	Assignee    string   // username or email
	Labels      []string // label names
	Metadata    map[string]string
}

// MapIssueToBeadCreate converts a Linear issue to Beads creation options
func MapIssueToBeadCreate(issue *Issue) *BeadCreateOptions {
	opts := &BeadCreateOptions{
		Title:       issue.Title,
		Description: issue.Description,
		Type:        MapType(issue),
		Priority:    MapPriority(issue.Priority),
		Status:      MapStatus(issue.State),
		Metadata:    make(map[string]string),
	}

	// Set assignee if present
	if issue.Assignee != nil {
		opts.Assignee = issue.Assignee.Email
		if opts.Assignee == "" {
			opts.Assignee = issue.Assignee.Name
		}
	}

	// Extract label names
	if len(issue.Labels) > 0 {
		opts.Labels = make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			opts.Labels[i] = label.Name
		}
	}

	// Store Linear metadata
	opts.Metadata["linear_id"] = issue.Identifier
	opts.Metadata["linear_url"] = issue.URL
	if issue.Project != nil {
		opts.Metadata["linear_project"] = issue.Project.Name
	}
	if issue.Estimate != nil {
		opts.Metadata["linear_estimate"] = fmt.Sprintf("%.1f", *issue.Estimate)
	}

	return opts
}

// FormatBeadDescription formats a bead description with Linear metadata
// Appends Linear-specific information to the description
func FormatBeadDescription(issue *Issue) string {
	var builder strings.Builder

	// Add the original description
	if issue.Description != "" {
		builder.WriteString(issue.Description)
		builder.WriteString("\n\n")
	}

	// Add Linear metadata section
	builder.WriteString("---\n")
	builder.WriteString("**Imported from Linear**\n")
	builder.WriteString(fmt.Sprintf("- ID: %s\n", issue.Identifier))
	builder.WriteString(fmt.Sprintf("- URL: %s\n", issue.URL))
	builder.WriteString(fmt.Sprintf("- State: %s (%s)\n", issue.State.Name, issue.State.Type))

	if issue.Project != nil {
		builder.WriteString(fmt.Sprintf("- Project: %s\n", issue.Project.Name))
	}
	if issue.Estimate != nil {
		builder.WriteString(fmt.Sprintf("- Estimate: %.1f\n", *issue.Estimate))
	}
	if issue.Assignee != nil {
		builder.WriteString(fmt.Sprintf("- Assignee: %s\n", issue.Assignee.Name))
	}

	return builder.String()
}
