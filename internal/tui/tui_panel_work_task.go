package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
)

// WorkTaskPanel renders the right side of the work details view when a task or unassigned bead is selected.
// It displays task details including ID, type, status, beads, and errors, or unassigned bead details.
type WorkTaskPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Viewport for scrolling
	viewport viewport.Model

	// Data
	selectedTask   *progress.TaskProgress // The selected task, or nil if unassigned bead
	selectedBead   *progress.BeanProgress // The selected unassigned bead, or nil if task
	isUnassigned   bool          // True if showing an unassigned bead
}

// NewWorkTaskPanel creates a new WorkTaskPanel
func NewWorkTaskPanel() *WorkTaskPanel {
	vp := viewport.New(40, 20) // Initial size, will be updated
	// Mouse wheel events are handled at the top level (planModel.handleMouseWheel)
	// to ensure only the panel under the cursor scrolls
	vp.MouseWheelEnabled = false

	return &WorkTaskPanel{
		width:    40,
		height:   20,
		viewport: vp,
	}
}

// SetSize updates the panel dimensions
func (p *WorkTaskPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update viewport dimensions
	// Calculate available lines for content (minus border and title)
	visibleLines := max(height-3, 1)

	// Set viewport size accounting for padding (2 chars total)
	p.viewport.Width = width - 2
	p.viewport.Height = visibleLines
}

// SetFocus updates the focus state
func (p *WorkTaskPanel) SetFocus(focused bool) {
	p.focused = focused
}

// SetTask sets the task to display
func (p *WorkTaskPanel) SetTask(task *progress.TaskProgress) {
	p.selectedTask = task
	p.selectedBead = nil
	p.isUnassigned = false
	// Reset viewport scroll when switching content
	p.viewport.SetYOffset(0)
}

// SetUnassignedBead sets the unassigned bead to display
func (p *WorkTaskPanel) SetUnassignedBead(bead *progress.BeanProgress) {
	p.selectedTask = nil
	p.selectedBead = bead
	p.isUnassigned = true
	// Reset viewport scroll when switching content
	p.viewport.SetYOffset(0)
}

// Clear clears the panel content
func (p *WorkTaskPanel) Clear() {
	p.selectedTask = nil
	p.selectedBead = nil
	p.isUnassigned = false
}

// ScrollUp scrolls the content up (shows earlier content)
func (p *WorkTaskPanel) ScrollUp() {
	p.viewport.ScrollUp(1)
}

// ScrollDown scrolls the content down (shows later content)
func (p *WorkTaskPanel) ScrollDown() {
	p.viewport.ScrollDown(1)
}

// ScrollToTop scrolls to the beginning of the content
func (p *WorkTaskPanel) ScrollToTop() {
	p.viewport.GotoTop()
}

// ScrollToBottom scrolls to the end of the content
func (p *WorkTaskPanel) ScrollToBottom() {
	p.viewport.GotoBottom()
}

// GetViewport returns the viewport for external updates
func (p *WorkTaskPanel) GetViewport() *viewport.Model {
	return &p.viewport
}

// Render returns the task/bead details content using the viewport
func (p *WorkTaskPanel) Render(panelWidth int) string {
	// Get the full content first
	var fullContent string
	if p.isUnassigned && p.selectedBead != nil {
		fullContent = p.renderUnassignedBeadDetails(panelWidth)
	} else if p.selectedTask != nil {
		fullContent = p.renderTaskDetails(panelWidth)
	} else {
		fullContent = tuiDimStyle.Render("Select an item to view details")
	}

	// Set the content in the viewport
	p.viewport.SetContent(fullContent)

	// The viewport will handle scrolling internally
	return p.viewport.View()
}

// renderTaskDetails renders details for a task
func (p *WorkTaskPanel) renderTaskDetails(panelWidth int) string {
	if p.selectedTask == nil {
		return tuiDimStyle.Render("No task selected")
	}

	var content strings.Builder
	task := p.selectedTask

	// Account for padding (tuiPanelStyle has Padding(0, 1) = 2 chars total)
	contentWidth := panelWidth - 2

	fmt.Fprintf(&content, "ID: %s\n", task.Task.ID)
	fmt.Fprintf(&content, "Type: %s\n", task.Task.TaskType)
	fmt.Fprintf(&content, "Status: %s\n", task.Task.Status)

	if task.Task.ComplexityBudget > 0 {
		fmt.Fprintf(&content, "Budget: %d\n", task.Task.ComplexityBudget)
	}

	// Show task beads
	fmt.Fprintf(&content, "\nBeads (%d):\n", len(task.Beads))
	for i, bead := range task.Beads {
		if i >= 10 {
			fmt.Fprintf(&content, "  ... and %d more\n", len(task.Beads)-10)
			break
		}
		statusStr := "○"
		switch bead.Status {
		case db.StatusCompleted:
			statusStr = "✓"
		case db.StatusProcessing:
			statusStr = "●"
		}
		beadLine := fmt.Sprintf("  %s %s", statusStr, bead.ID)
		if bead.Title != "" {
			// "  ○ ID: " is about 8 chars prefix
			beadLine += ": " + ansi.Truncate(bead.Title, contentWidth-8-len(bead.ID), "...")
		}
		content.WriteString(beadLine + "\n")
	}

	// Show error if failed
	if task.Task.Status == db.StatusFailed && task.Task.ErrorMessage != "" {
		errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		content.WriteString("\n")
		content.WriteString(errorStyle.Render("Error:"))
		content.WriteString("\n")
		content.WriteString(ansi.Truncate(task.Task.ErrorMessage, contentWidth, "..."))
	}

	return content.String()
}

// renderUnassignedBeadDetails renders details for an unassigned bead
func (p *WorkTaskPanel) renderUnassignedBeadDetails(panelWidth int) string {
	if p.selectedBead == nil {
		return tuiDimStyle.Render("No bead selected")
	}

	var content strings.Builder
	bead := p.selectedBead

	// Account for padding (tuiPanelStyle has Padding(0, 1) = 2 chars total)
	contentWidth := panelWidth - 2

	// Header with warning style and action hint
	warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	content.WriteString(warningStyle.Render("Unassigned Issue"))
	content.WriteString(" ")
	content.WriteString(tuiDimStyle.Render("[p] plan [r] run"))
	content.WriteString("\n\n")

	fmt.Fprintf(&content, "ID: %s\n", bead.ID)
	if bead.Title != "" {
		fmt.Fprintf(&content, "Title: %s\n", ansi.Truncate(bead.Title, contentWidth-7, "..."))
	}
	if bead.IssueType != "" {
		fmt.Fprintf(&content, "Type: %s\n", bead.IssueType)
	}
	fmt.Fprintf(&content, "Priority: %s\n", bead.Priority)
	fmt.Fprintf(&content, "Status: %s\n", bead.BeanStatus)

	if bead.Description != "" {
		content.WriteString("\nDescription:\n")
		content.WriteString(ansi.Truncate(bead.Description, contentWidth, "..."))
	}

	return content.String()
}
