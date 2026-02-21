package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sargehq/sarge/internal/progress"
)

// WorkDetailAction represents an action result from the work details panel
type WorkDetailAction int

const (
	WorkDetailActionNone                WorkDetailAction = iota
	WorkDetailActionOpenTerminal                         // Open terminal/console (t)
	WorkDetailActionOpenAgent                            // Open agent session (c)
	WorkDetailActionOpenIDE                              // Open worktree in IDE (i)
	WorkDetailActionRun                                  // Run work (r)
	WorkDetailActionReview                               // Create review task (v)
	WorkDetailActionPR                                   // Create PR task (p)
	WorkDetailActionPlan                                 // Start planning session for bead (p when unassigned bead selected)
	WorkDetailActionNavigateUp                           // Navigate up (k/up)
	WorkDetailActionNavigateDown                         // Navigate down (j/down)
	WorkDetailActionRestartOrchestrator                  // Restart orchestrator (o)
	WorkDetailActionCheckFeedback                        // Check PR feedback (f)
	WorkDetailActionDestroy                              // Destroy work (d)
	WorkDetailActionAddChildIssue                        // Add child issue to root issue (a)
	WorkDetailActionResetTask                            // Reset failed task (x)
	WorkDetailActionAttachTerminal                       // Attach terminal to zmx session (g)
)

// WorkDetailsPanel is a coordinator that manages the work detail sub-panels.
// It handles layout, keyboard/mouse events, and coordinates which right panel to show.
type WorkDetailsPanel struct {
	// Dimensions
	width       int
	height      int
	columnRatio float64 // Ratio of left column width (0.0-1.0), synced with issues panel

	// Focus state
	leftPanelFocused  bool
	rightPanelFocused bool

	// Sub-panels
	overviewPanel *WorkOverviewPanel // Left panel: work info + tasks list
	summaryPanel  *WorkSummaryPanel  // Right panel: work overview (when root selected)
	taskPanel     *WorkTaskPanel     // Right panel: task/bead details

	// Data reference (shared with sub-panels)
	focusedWork *progress.WorkProgress
}

// NewWorkDetailsPanel creates a new WorkDetailsPanel coordinator
func NewWorkDetailsPanel() *WorkDetailsPanel {
	return &WorkDetailsPanel{
		width:         80,
		height:        20,
		columnRatio:   0.4, // Default 40/60 split to match issues panel
		overviewPanel: NewWorkOverviewPanel(),
		summaryPanel:  NewWorkSummaryPanel(),
		taskPanel:     NewWorkTaskPanel(),
	}
}

// SetSize updates the panel dimensions
func (p *WorkDetailsPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Calculate column widths using the same formula as render
	totalContentWidth := width - 4
	leftWidth := int(float64(totalContentWidth) * p.columnRatio)
	rightWidth := totalContentWidth - leftWidth

	// Calculate available lines for content (minus border and title)
	visibleLines := max(height-3, 1)

	// Update sub-panel sizes
	p.overviewPanel.SetSize(leftWidth, height)
	p.summaryPanel.SetSize(rightWidth, visibleLines)
	p.taskPanel.SetSize(rightWidth, visibleLines)
}

// SetColumnRatio sets the column width ratio to match the issues panel
func (p *WorkDetailsPanel) SetColumnRatio(ratio float64) {
	p.columnRatio = ratio
}

// SetFocus updates which side is focused
func (p *WorkDetailsPanel) SetFocus(leftFocused, rightFocused bool) {
	p.leftPanelFocused = leftFocused
	p.rightPanelFocused = rightFocused
	p.overviewPanel.SetFocus(leftFocused)
	p.summaryPanel.SetFocus(rightFocused)
	p.taskPanel.SetFocus(rightFocused)
}

// SetFocusedWork updates the focused work, preserving selection if valid
func (p *WorkDetailsPanel) SetFocusedWork(focusedWork *progress.WorkProgress) {
	p.focusedWork = focusedWork
	p.overviewPanel.SetFocusedWork(focusedWork)
	p.summaryPanel.SetFocusedWork(focusedWork)

	// Update task panel based on current selection
	p.syncTaskPanel()
}

// syncTaskPanel updates the task panel based on current selection
func (p *WorkDetailsPanel) syncTaskPanel() {
	if p.focusedWork == nil {
		p.taskPanel.Clear()
		return
	}

	selectedIndex := p.overviewPanel.GetSelectedIndex()
	if selectedIndex == 0 {
		// Root issue selected - show summary panel
		p.taskPanel.Clear()
		return
	}

	tasksEndIdx := 1 + len(p.focusedWork.Tasks)

	// Check if task is selected
	taskIdx := selectedIndex - 1
	if taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks) {
		p.taskPanel.SetTask(p.focusedWork.Tasks[taskIdx])
		return
	}

	// Check if unassigned bead is selected
	unassignedIdx := selectedIndex - tasksEndIdx
	if unassignedIdx >= 0 && unassignedIdx < len(p.focusedWork.UnassignedBeads) {
		bead := p.focusedWork.UnassignedBeads[unassignedIdx]
		p.taskPanel.SetUnassignedBead(&bead)
		return
	}

	p.taskPanel.Clear()
}

// ScrollUp scrolls the right panel content up (shows earlier content)
func (p *WorkDetailsPanel) ScrollUp() {
	if p.overviewPanel.GetSelectedIndex() == 0 {
		p.summaryPanel.ScrollUp()
	} else {
		p.taskPanel.ScrollUp()
	}
}

// ScrollDown scrolls the right panel content down (shows later content)
func (p *WorkDetailsPanel) ScrollDown() {
	if p.overviewPanel.GetSelectedIndex() == 0 {
		p.summaryPanel.ScrollDown()
	} else {
		p.taskPanel.ScrollDown()
	}
}

// ScrollToTop scrolls to the beginning of the right panel content
func (p *WorkDetailsPanel) ScrollToTop() {
	if p.overviewPanel.GetSelectedIndex() == 0 {
		p.summaryPanel.ScrollToTop()
	} else {
		p.taskPanel.ScrollToTop()
	}
}

// ScrollToBottom scrolls to the end of the right panel content
func (p *WorkDetailsPanel) ScrollToBottom() {
	if p.overviewPanel.GetSelectedIndex() == 0 {
		p.summaryPanel.ScrollToBottom()
	} else {
		p.taskPanel.ScrollToBottom()
	}
}

// SetOrchestratorHealth updates the orchestrator health status
func (p *WorkDetailsPanel) SetOrchestratorHealth(healthy bool) {
	p.overviewPanel.SetOrchestratorHealth(healthy)
}

// IsOrchestratorHealthy returns whether the orchestrator is running
func (p *WorkDetailsPanel) IsOrchestratorHealthy() bool {
	return p.overviewPanel.IsOrchestratorHealthy()
}

// GetSelectedIndex returns the currently selected index (0 = root issue, 1+ = tasks)
func (p *WorkDetailsPanel) GetSelectedIndex() int {
	return p.overviewPanel.GetSelectedIndex()
}

// GetFocusedWork returns the currently focused work, or nil if none
func (p *WorkDetailsPanel) GetFocusedWork() *progress.WorkProgress {
	return p.focusedWork
}

// SetSelectedIndex sets the selected index
func (p *WorkDetailsPanel) SetSelectedIndex(idx int) {
	p.overviewPanel.SetSelectedIndex(idx)
	p.resetRightViewportScroll()
	p.syncTaskPanel()
}

// resetRightViewportScroll resets the right panel viewport scroll position
func (p *WorkDetailsPanel) resetRightViewportScroll() {
	p.summaryPanel.GetViewport().SetYOffset(0)
	p.taskPanel.GetViewport().SetYOffset(0)
}

// SetHoveredItem updates which item is hovered
func (p *WorkDetailsPanel) SetHoveredItem(index int) {
	p.overviewPanel.SetHoveredItem(index)
}

// GetHoveredItem returns the currently hovered item index
func (p *WorkDetailsPanel) GetHoveredItem() int {
	return p.overviewPanel.GetHoveredItem()
}

// GetSelectedTaskID returns the currently selected task ID, or empty if root issue is selected
func (p *WorkDetailsPanel) GetSelectedTaskID() string {
	return p.overviewPanel.GetSelectedTaskID()
}

// GetSelectedBeadIDs returns the bead IDs that should be shown based on current selection.
func (p *WorkDetailsPanel) GetSelectedBeadIDs() []string {
	return p.overviewPanel.GetSelectedBeadIDs()
}

// IsTaskSelected returns true if a task is currently selected (vs root issue)
func (p *WorkDetailsPanel) IsTaskSelected() bool {
	return p.overviewPanel.IsTaskSelected()
}

// IsSelectedTaskFailed returns true if the selected task has failed status
func (p *WorkDetailsPanel) IsSelectedTaskFailed() bool {
	return p.overviewPanel.IsSelectedTaskFailed()
}

// IsUnassignedBeadSelected returns true if an unassigned bead is currently selected
func (p *WorkDetailsPanel) IsUnassignedBeadSelected() bool {
	return p.overviewPanel.IsUnassignedBeadSelected()
}

// GetSelectedUnassignedBeadID returns the ID of the selected unassigned bead
func (p *WorkDetailsPanel) GetSelectedUnassignedBeadID() string {
	return p.overviewPanel.GetSelectedUnassignedBeadID()
}

// SetSelectedTaskID sets selection to the task with given ID
func (p *WorkDetailsPanel) SetSelectedTaskID(id string) {
	p.overviewPanel.SetSelectedTaskID(id)
	p.syncTaskPanel()
}

// NavigateUp moves selection to the previous item
func (p *WorkDetailsPanel) NavigateUp() {
	p.overviewPanel.NavigateUp()
	p.resetRightViewportScroll()
	p.syncTaskPanel()
}

// NavigateDown moves selection to the next item
func (p *WorkDetailsPanel) NavigateDown() {
	p.overviewPanel.NavigateDown()
	p.resetRightViewportScroll()
	p.syncTaskPanel()
}

// NavigateTaskUp is an alias for NavigateUp (for compatibility)
func (p *WorkDetailsPanel) NavigateTaskUp() {
	p.NavigateUp()
}

// NavigateTaskDown is an alias for NavigateDown (for compatibility)
func (p *WorkDetailsPanel) NavigateTaskDown() {
	p.NavigateDown()
}

// Render returns the work details split view (uses p.height from SetSize)
func (p *WorkDetailsPanel) Render() string {
	return p.RenderWithPanel(p.height)
}

// RenderWithPanel returns the work details split view with the given total height
// This matches the IssuesPanel.RenderWithPanel pattern exactly
func (p *WorkDetailsPanel) RenderWithPanel(contentHeight int) string {
	// Ensure minimum content height to prevent layout issues
	if contentHeight < 6 {
		contentHeight = 6
	}

	// Calculate column widths using the same formula as issues panel
	totalContentWidth := p.width - 4
	leftWidth := int(float64(totalContentWidth) * p.columnRatio)
	rightWidth := totalContentWidth - leftWidth

	// Content lines available inside each sub-panel (excluding border and title)
	// Same formula as IssuesPanel: contentHeight - 3 for border (2) + title (1)
	availableContentLines := max(contentHeight-3, 1)
	availableContentLines--

	// === Left side: Work info and items list ===
	leftContent := p.overviewPanel.Render(availableContentLines, leftWidth)

	// === Right side: Selected item details ===
	rightContent := p.renderRightPanel(availableContentLines, rightWidth)

	// Create the two panels with fixed height (matching IssuesPanel pattern exactly)
	// IssuesPanel uses: Height(contentHeight - 2)
	leftPanelStyle := tuiPanelStyle.Width(leftWidth).Height(contentHeight - 2)
	if p.leftPanelFocused {
		leftPanelStyle = leftPanelStyle.BorderForeground(lipgloss.Color("214"))
	}

	leftPanel := leftPanelStyle.Render(tuiTitleStyle.Render("Work") + "\n" + leftContent)

	// Right panel uses its own height setting
	rightPanelStyle := tuiPanelStyle.Width(rightWidth).Height(contentHeight - 2)
	if p.rightPanelFocused {
		rightPanelStyle = rightPanelStyle.BorderForeground(lipgloss.Color("214"))
	}

	rightPanel := rightPanelStyle.Render(tuiTitleStyle.Render("Details") + "\n" + rightContent)

	// Combine panels horizontally
	result := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	return result
}

// renderRightPanel renders the right panel with selected item details using the appropriate sub-panel
func (p *WorkDetailsPanel) renderRightPanel(_, panelWidth int) string {
	if p.focusedWork == nil {
		return tuiDimStyle.Render("Loading...")
	}

	selectedIndex := p.overviewPanel.GetSelectedIndex()

	if selectedIndex == 0 {
		// Show root issue details using summary panel
		return p.summaryPanel.Render(panelWidth)
	}

	// Show task or unassigned bead details using task panel
	return p.taskPanel.Render(panelWidth)
}


// UpdateViewport handles mouse wheel events for the right panel viewport.
// The caller (handleMouseWheel) has already verified the mouse is over the right panel.
func (p *WorkDetailsPanel) UpdateViewport(msg tea.Msg) tea.Cmd {
	// Handle mouse wheel events by manually scrolling (since MouseWheelEnabled is false)
	if mouseMsg, ok := msg.(tea.MouseMsg); ok {
		if mouseMsg.Button == tea.MouseButtonWheelUp {
			p.ScrollUp()
		} else if mouseMsg.Button == tea.MouseButtonWheelDown {
			p.ScrollDown()
		}
	}
	return nil
}

// Update handles key events and returns an action.
func (p *WorkDetailsPanel) Update(msg tea.KeyMsg) (tea.Cmd, WorkDetailAction) {
	// When right panel is focused, let viewport handle scrolling keys
	if p.rightPanelFocused {
		var cmd tea.Cmd
		if p.overviewPanel.GetSelectedIndex() == 0 {
			vp := p.summaryPanel.GetViewport()
			*vp, cmd = vp.Update(msg)
		} else {
			vp := p.taskPanel.GetViewport()
			*vp, cmd = vp.Update(msg)
		}

		// Still handle action keys even when right panel is focused
		switch msg.String() {
		case "t":
			return cmd, WorkDetailActionOpenTerminal
		case "c":
			return cmd, WorkDetailActionOpenAgent
		case "i":
			return cmd, WorkDetailActionOpenIDE
		case "r":
			return cmd, WorkDetailActionRun
		case "v":
			return cmd, WorkDetailActionReview
		case "p":
			// 'p' = plan when unassigned bead selected, PR otherwise
			if p.IsUnassignedBeadSelected() {
				return cmd, WorkDetailActionPlan
			}
			return cmd, WorkDetailActionPR
		case "o":
			return cmd, WorkDetailActionRestartOrchestrator
		case "f":
			return cmd, WorkDetailActionCheckFeedback
		case "d":
			return cmd, WorkDetailActionDestroy
		case "g":
			return cmd, WorkDetailActionAttachTerminal
		case "a":
			// Add child issue - only when there's a focused work with root issue
			if p.focusedWork != nil && p.focusedWork.Work.RootIssueID != "" {
				return cmd, WorkDetailActionAddChildIssue
			}
			return cmd, WorkDetailActionNone
		case "x":
			// Reset failed task - only when a failed task is selected
			if p.IsTaskSelected() && p.IsSelectedTaskFailed() {
				return cmd, WorkDetailActionResetTask
			}
			return cmd, WorkDetailActionNone
		default:
			return cmd, WorkDetailActionNone
		}
	}

	// When left panel is focused, handle navigation and actions
	switch msg.String() {
	case "j", "down":
		// When left panel is focused, navigate selection
		p.NavigateDown()
		return nil, WorkDetailActionNavigateDown
	case "k", "up":
		// When left panel is focused, navigate selection
		p.NavigateUp()
		return nil, WorkDetailActionNavigateUp
	case "t":
		return nil, WorkDetailActionOpenTerminal
	case "c":
		return nil, WorkDetailActionOpenAgent
	case "i":
		return nil, WorkDetailActionOpenIDE
	case "r":
		return nil, WorkDetailActionRun
	case "v":
		return nil, WorkDetailActionReview
	case "p":
		// 'p' = plan when unassigned bead selected, PR otherwise
		if p.IsUnassignedBeadSelected() {
			return nil, WorkDetailActionPlan
		}
		return nil, WorkDetailActionPR
	case "o":
		return nil, WorkDetailActionRestartOrchestrator
	case "f":
		return nil, WorkDetailActionCheckFeedback
	case "d":
		return nil, WorkDetailActionDestroy
	case "g":
		return nil, WorkDetailActionAttachTerminal
	case "a":
		// Add child issue - only when there's a focused work with root issue
		if p.focusedWork != nil && p.focusedWork.Work.RootIssueID != "" {
			return nil, WorkDetailActionAddChildIssue
		}
	case "x":
		// Reset failed task - only when a failed task is selected
		if p.IsTaskSelected() && p.IsSelectedTaskFailed() {
			return nil, WorkDetailActionResetTask
		}
	}

	return nil, WorkDetailActionNone
}

// DetectClickedItem determines which item was clicked and returns its index
func (p *WorkDetailsPanel) DetectClickedItem(msg tea.MouseMsg) int {
	return p.overviewPanel.DetectClickedItem(msg)
}

// DetectClickedTask returns the task ID if a task was clicked, empty string otherwise
func (p *WorkDetailsPanel) DetectClickedTask(msg tea.MouseMsg) string {
	if p.focusedWork == nil {
		return ""
	}
	itemIndex := p.DetectClickedItem(msg)
	if itemIndex <= 0 {
		return "" // -1 = no click, 0 = root issue
	}
	taskIdx := itemIndex - 1
	if taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks) {
		return p.focusedWork.Tasks[taskIdx].Task.ID
	}
	return ""
}

// DetectHoveredItem determines which item is at the mouse position for hover detection.
// Returns the absolute index (0 = root, 1+ = tasks, N+ = unassigned beads), or -1 if not over an item.
func (p *WorkDetailsPanel) DetectHoveredItem(msg tea.MouseMsg) int {
	return p.overviewPanel.DetectHoveredItem(msg)
}
