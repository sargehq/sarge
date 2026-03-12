package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
)

// WorkSummaryPanel renders the right side of the work details view when the root issue is selected.
// It displays work overview, alerts, root issue details, and statistics.
type WorkSummaryPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Viewport for scrolling
	viewport viewport.Model

	// Data
	focusedWork *progress.WorkProgress
}

// NewWorkSummaryPanel creates a new WorkSummaryPanel
func NewWorkSummaryPanel() *WorkSummaryPanel {
	vp := viewport.New(viewport.WithWidth(40), viewport.WithHeight(20)) // Initial size, will be updated
	// Mouse wheel events are handled at the top level (planModel.handleMouseWheel)
	// to ensure only the panel under the cursor scrolls
	vp.MouseWheelEnabled = false

	return &WorkSummaryPanel{
		width:    40,
		height:   20,
		viewport: vp,
	}
}

// SetSize updates the panel dimensions
func (p *WorkSummaryPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update viewport dimensions
	// Calculate available lines for content (minus border and title)
	visibleLines := max(height-3, 1)

	// Set viewport size accounting for padding (2 chars total)
	p.viewport.SetWidth(width - 2)
	p.viewport.SetHeight(visibleLines)
}

// SetFocus updates the focus state
func (p *WorkSummaryPanel) SetFocus(focused bool) {
	p.focused = focused
}

// SetFocusedWork updates the focused work
func (p *WorkSummaryPanel) SetFocusedWork(focusedWork *progress.WorkProgress) {
	p.focusedWork = focusedWork
	// Reset viewport scroll when switching focus
	p.viewport.SetYOffset(0)
}

// ScrollUp scrolls the content up (shows earlier content)
func (p *WorkSummaryPanel) ScrollUp() {
	p.viewport.ScrollUp(1)
}

// ScrollDown scrolls the content down (shows later content)
func (p *WorkSummaryPanel) ScrollDown() {
	p.viewport.ScrollDown(1)
}

// ScrollToTop scrolls to the beginning of the content
func (p *WorkSummaryPanel) ScrollToTop() {
	p.viewport.GotoTop()
}

// ScrollToBottom scrolls to the end of the content
func (p *WorkSummaryPanel) ScrollToBottom() {
	p.viewport.GotoBottom()
}

// GetViewport returns the viewport for external updates
func (p *WorkSummaryPanel) GetViewport() *viewport.Model {
	return &p.viewport
}

// Render returns the work summary content using the viewport
func (p *WorkSummaryPanel) Render(panelWidth int) string {
	// Get the full content first
	fullContent := p.renderFullContent(panelWidth)

	// Set the content in the viewport
	p.viewport.SetContent(fullContent)

	// The viewport will handle scrolling internally
	return p.viewport.View()
}

// renderFullContent returns the full content without scrolling applied
func (p *WorkSummaryPanel) renderFullContent(panelWidth int) string {
	var content strings.Builder

	if p.focusedWork == nil {
		content.WriteString(tuiDimStyle.Render("Loading..."))
		return content.String()
	}

	// Account for padding (tuiPanelStyle has Padding(0, 1) = 2 chars total)
	contentWidth := panelWidth - 2

	// == Work Overview Section ==
	overviewStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	content.WriteString(overviewStyle.Render("Work Overview"))
	content.WriteString("\n")
	content.WriteString(strings.Repeat("─", contentWidth))
	content.WriteString("\n")

	// Work metadata
	statusStyle := lipgloss.NewStyle()
	switch p.focusedWork.Work.Status {
	case db.StatusCompleted:
		statusStyle = statusStyle.Foreground(lipgloss.Color("82"))
	case db.StatusProcessing:
		statusStyle = statusStyle.Foreground(lipgloss.Color("214"))
	case db.StatusFailed:
		statusStyle = statusStyle.Foreground(lipgloss.Color("196"))
	default:
		statusStyle = statusStyle.Foreground(lipgloss.Color("247"))
	}
	fmt.Fprintf(&content, "Status: %s\n", statusStyle.Render(p.focusedWork.Work.Status))

	// PR URL (if available)
	if p.focusedWork.Work.PRURL != "" {
		prStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
		fmt.Fprintf(&content, "PR: %s\n", prStyle.Render(p.focusedWork.Work.PRURL))

		// PR Status section (only show if we have a PR)
		content.WriteString("\n")
		prStatusHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141"))
		content.WriteString(prStatusHeaderStyle.Render("PR Status"))
		content.WriteString("\n")

		// CI Status
		ciStatus := p.focusedWork.CIStatus
		if ciStatus == "" {
			ciStatus = db.CIStatusPending
		}
		ciIcon := "⏳"
		ciText := "Pending"
		ciColor := lipgloss.Color("226") // yellow
		switch ciStatus {
		case db.CIStatusSuccess:
			ciIcon = "✓"
			ciText = "Passing"
			ciColor = lipgloss.Color("82") // green
		case db.CIStatusFailure:
			ciIcon = "✗"
			ciText = "Failing"
			ciColor = lipgloss.Color("196") // red
		}
		ciStyle := lipgloss.NewStyle().Foreground(ciColor)
		fmt.Fprintf(&content, "  CI: %s\n", ciStyle.Render(ciIcon+" "+ciText))

		// Approval Status
		approvalStatus := p.focusedWork.ApprovalStatus
		if approvalStatus == "" {
			approvalStatus = db.ApprovalStatusPending
		}
		approvalIcon := "⏳"
		approvalText := "Awaiting review"
		approvalColor := lipgloss.Color("247") // dim
		switch approvalStatus {
		case db.ApprovalStatusApproved:
			approvalIcon = "✓"
			if len(p.focusedWork.Approvers) > 0 {
				approvalText = "Approved by " + strings.Join(p.focusedWork.Approvers, ", ")
			} else {
				approvalText = "Approved"
			}
			approvalColor = lipgloss.Color("82") // green
		case db.ApprovalStatusChangesRequested:
			approvalIcon = "⚠"
			approvalText = "Changes requested"
			approvalColor = lipgloss.Color("214") // orange
		}
		approvalStyle := lipgloss.NewStyle().Foreground(approvalColor)
		fmt.Fprintf(&content, "  Review: %s\n", approvalStyle.Render(approvalIcon+" "+approvalText))

		// Feedback (show bean IDs)
		if p.focusedWork.FeedbackCount > 0 {
			feedbackStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			beanIDsStr := strings.Join(p.focusedWork.FeedbackBeanIDs, ", ")
			fmt.Fprintf(&content, "  Feedback: %s\n", feedbackStyle.Render(beanIDsStr))
		}

		// Merge Status
		if p.focusedWork.MergeableState != "" {
			mergeIcon := "⏳"
			mergeText := "Unknown"
			mergeColor := lipgloss.Color("247") // dim
			switch p.focusedWork.MergeableState {
			case db.MergeableStateClean:
				mergeIcon = "✓"
				mergeText = "Ready to merge"
				mergeColor = lipgloss.Color("82") // green
			case db.MergeableStateDirty:
				mergeIcon = "⚠"
				mergeText = "Has conflicts"
				mergeColor = lipgloss.Color("196") // red
			case db.MergeableStateBlocked:
				mergeIcon = "⏸"
				mergeText = "Blocked by checks"
				mergeColor = lipgloss.Color("226") // yellow
			case db.MergeableStateBehind:
				mergeIcon = "↓"
				mergeText = "Behind main"
				mergeColor = lipgloss.Color("247") // dim
			case db.MergeableStateDraft:
				mergeIcon = "📝"
				mergeText = "Draft PR"
				mergeColor = lipgloss.Color("247") // dim
			case db.MergeableStateUnstable:
				mergeIcon = "⚠"
				mergeText = "CI unstable"
				mergeColor = lipgloss.Color("226") // yellow
			}
			mergeStyle := lipgloss.NewStyle().Foreground(mergeColor)
			fmt.Fprintf(&content, "  Merge: %s\n", mergeStyle.Render(mergeIcon+" "+mergeText))
		}
	}

	// Creation time
	if p.focusedWork.Work.CreatedAt.Unix() > 0 {
		timeAgo := time.Since(p.focusedWork.Work.CreatedAt)
		var timeStr string
		if timeAgo.Hours() < 1 {
			timeStr = fmt.Sprintf("%d minutes ago", int(timeAgo.Minutes()))
		} else if timeAgo.Hours() < 24 {
			timeStr = fmt.Sprintf("%d hours ago", int(timeAgo.Hours()))
		} else {
			days := int(timeAgo.Hours() / 24)
			timeStr = fmt.Sprintf("%d days ago", days)
		}
		fmt.Fprintf(&content, "Created: %s\n", timeStr)
	}

	// Progress
	completedTasks := 0
	for _, task := range p.focusedWork.Tasks {
		if task.Task.Status == db.StatusCompleted {
			completedTasks++
		}
	}
	percentage := 0
	if len(p.focusedWork.Tasks) > 0 {
		percentage = (completedTasks * 100) / len(p.focusedWork.Tasks)
	}

	progressStyle := lipgloss.NewStyle().Bold(true)
	if percentage == 100 {
		progressStyle = progressStyle.Foreground(lipgloss.Color("82"))
	} else if percentage >= 75 {
		progressStyle = progressStyle.Foreground(lipgloss.Color("226"))
	} else if percentage >= 50 {
		progressStyle = progressStyle.Foreground(lipgloss.Color("214"))
	} else {
		progressStyle = progressStyle.Foreground(lipgloss.Color("247"))
	}
	content.WriteString("Progress: ")
	content.WriteString(progressStyle.Render(fmt.Sprintf("%d%%", percentage)))
	fmt.Fprintf(&content, " (%d/%d tasks completed)\n", completedTasks, len(p.focusedWork.Tasks))

	// Alerts/Warnings
	if p.focusedWork.UnassignedBeanCount > 0 || p.focusedWork.FeedbackCount > 0 {
		content.WriteString("\n")
		alertHeaderStyle := lipgloss.NewStyle().Bold(true)
		content.WriteString(alertHeaderStyle.Render("Alerts:"))
		content.WriteString("\n")

		if p.focusedWork.UnassignedBeanCount > 0 {
			warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
			content.WriteString(warningStyle.Render(fmt.Sprintf("  ⚠ %d unassigned bean(s) need attention\n", p.focusedWork.UnassignedBeanCount)))
		}
		if p.focusedWork.FeedbackCount > 0 {
			alertStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
			beanIDsStr := strings.Join(p.focusedWork.FeedbackBeanIDs, ", ")
			content.WriteString(alertStyle.Render(fmt.Sprintf("  ● %d pending PR feedback: %s\n", p.focusedWork.FeedbackCount, beanIDsStr)))
		}
	}

	content.WriteString("\n")

	// == Root Issue Section ==
	rootID := p.focusedWork.Work.RootIssueID
	issueHeaderStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	content.WriteString(issueHeaderStyle.Render("Root Issue"))
	content.WriteString("\n")
	content.WriteString(strings.Repeat("─", contentWidth))
	content.WriteString("\n")

	// Find root bean in workBeans
	var rootBean *progress.BeanProgress
	for i := range p.focusedWork.WorkBeans {
		if p.focusedWork.WorkBeans[i].ID == rootID {
			rootBean = &p.focusedWork.WorkBeans[i]
			break
		}
	}

	// If not found in workBeans, try unassignedBeans
	if rootBean == nil {
		for i := range p.focusedWork.UnassignedBeans {
			if p.focusedWork.UnassignedBeans[i].ID == rootID {
				rootBean = &p.focusedWork.UnassignedBeans[i]
				break
			}
		}
	}

	// Display root issue details
	if rootBean != nil {
		// Title first (truncated to fit content width with some margin)
		if rootBean.Title != "" {
			titleStyle := lipgloss.NewStyle().Bold(true)
			title := ansi.Truncate(rootBean.Title, contentWidth-2, "...")
			content.WriteString(titleStyle.Render(title))
			content.WriteString("\n")
		}

		// Metadata line
		fmt.Fprintf(&content, "%s  Type: %s  P:%s  %s\n",
			rootID,
			rootBean.IssueType,
			rootBean.Priority,
			rootBean.BeanStatus)

		// Description (truncate to avoid layout issues)
		if rootBean.Description != "" {
			content.WriteString("\n")
			content.WriteString("Description:\n")
			// Keep multiline but truncate to reasonable length
			desc := rootBean.Description
			desc = ansi.Truncate(desc, 300, "...")
			content.WriteString(tuiDimStyle.Render(desc))
			content.WriteString("\n")
		}
	} else {
		// Fallback if bean not found
		fmt.Fprintf(&content, "Issue: %s\n", rootID)
		content.WriteString(tuiDimStyle.Render("(Issue details not loaded)"))
		content.WriteString("\n")
	}

	// Summary statistics
	content.WriteString("\n")
	summaryHeaderStyle := lipgloss.NewStyle().Bold(true)
	content.WriteString(summaryHeaderStyle.Render("Statistics:"))
	content.WriteString("\n")
	fmt.Fprintf(&content, "  Total Beans: %d\n", len(p.focusedWork.WorkBeans))
	fmt.Fprintf(&content, "  Total Tasks: %d\n", len(p.focusedWork.Tasks))

	// Count task types
	var estimateTasks, implementTasks, reviewTasks, prTasks int
	for _, task := range p.focusedWork.Tasks {
		switch task.Task.TaskType {
		case "estimate":
			estimateTasks++
		case "implement":
			implementTasks++
		case "review":
			reviewTasks++
		case "pr", "update-pr-description":
			prTasks++
		}
	}
	if estimateTasks > 0 || reviewTasks > 0 || prTasks > 0 {
		content.WriteString("  Task Breakdown: ")
		parts := []string{}
		if estimateTasks > 0 {
			parts = append(parts, fmt.Sprintf("%d estimate", estimateTasks))
		}
		if implementTasks > 0 {
			parts = append(parts, fmt.Sprintf("%d implement", implementTasks))
		}
		if reviewTasks > 0 {
			parts = append(parts, fmt.Sprintf("%d review", reviewTasks))
		}
		if prTasks > 0 {
			parts = append(parts, fmt.Sprintf("%d pr", prTasks))
		}
		content.WriteString(strings.Join(parts, ", "))
		content.WriteString("\n")
	}

	return content.String()
}
