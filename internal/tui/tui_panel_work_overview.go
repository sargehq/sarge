package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
)

// WorkOverviewPanel renders the left side of the work details view.
// It displays the work header, branch info, progress, orchestrator health,
// and a selectable list of tasks and unassigned beans.
type WorkOverviewPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Data
	focusedWork         *progress.WorkProgress
	selectedIndex       int  // 0 = root issue, 1+ = tasks, N+ = unassigned beans
	hoveredIndex        int  // -1 = none, 0 = root issue, 1+ = tasks/unassigned beans
	orchestratorHealthy bool // Whether the orchestrator process is running

	// Zone prefix for unique zone IDs
	zonePrefix string
}

// NewWorkOverviewPanel creates a new WorkOverviewPanel
func NewWorkOverviewPanel() *WorkOverviewPanel {
	return &WorkOverviewPanel{
		width:        40,
		height:       20,
		hoveredIndex: -1, // No item hovered initially
		zonePrefix:   zone.NewPrefix(),
	}
}

// SetSize updates the panel dimensions
func (p *WorkOverviewPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *WorkOverviewPanel) SetFocus(focused bool) {
	p.focused = focused
}

// SetFocusedWork updates the focused work, preserving selection if valid
func (p *WorkOverviewPanel) SetFocusedWork(focusedWork *progress.WorkProgress) {
	p.focusedWork = focusedWork
	// Validate current selection still exists
	if focusedWork != nil {
		// 0 = root, 1..n = tasks, n+1..m = unassigned beans
		maxIndex := len(focusedWork.Tasks) + len(focusedWork.UnassignedBeans)
		if p.selectedIndex > maxIndex {
			p.selectedIndex = 0 // Reset to root issue
		}
	} else {
		p.selectedIndex = 0
	}
}

// SetOrchestratorHealth updates the orchestrator health status
func (p *WorkOverviewPanel) SetOrchestratorHealth(healthy bool) {
	p.orchestratorHealthy = healthy
}

// IsOrchestratorHealthy returns whether the orchestrator is running
func (p *WorkOverviewPanel) IsOrchestratorHealthy() bool {
	return p.orchestratorHealthy
}

// GetSelectedIndex returns the currently selected index (0 = root issue, 1+ = tasks)
func (p *WorkOverviewPanel) GetSelectedIndex() int {
	return p.selectedIndex
}

// SetSelectedIndex sets the selected index
func (p *WorkOverviewPanel) SetSelectedIndex(idx int) {
	p.selectedIndex = idx
}

// SetHoveredItem updates which item is hovered
func (p *WorkOverviewPanel) SetHoveredItem(index int) {
	p.hoveredIndex = index
}

// GetHoveredItem returns the currently hovered item index
func (p *WorkOverviewPanel) GetHoveredItem() int {
	return p.hoveredIndex
}

// GetFocusedWork returns the currently focused work, or nil if none
func (p *WorkOverviewPanel) GetFocusedWork() *progress.WorkProgress {
	return p.focusedWork
}

// GetSelectedTaskID returns the currently selected task ID, or empty if root issue is selected
func (p *WorkOverviewPanel) GetSelectedTaskID() string {
	if p.selectedIndex == 0 || p.focusedWork == nil {
		return ""
	}
	taskIdx := p.selectedIndex - 1
	if taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks) {
		return p.focusedWork.Tasks[taskIdx].Task.ID
	}
	return ""
}

// GetSelectedBeanIDs returns the bean IDs that should be shown based on current selection.
// - If root issue is selected (index 0): returns all work beans (root + dependents)
// - If a task is selected: returns only the beans assigned to that task
// - If an unassigned bean is selected: returns just that bean's ID
// Returns nil if no work is focused.
func (p *WorkOverviewPanel) GetSelectedBeanIDs() []string {
	if p.focusedWork == nil {
		return nil
	}

	if p.selectedIndex == 0 {
		// Root issue selected - return all work beans
		var beanIDs []string
		for _, bp := range p.focusedWork.WorkBeans {
			beanIDs = append(beanIDs, bp.ID)
		}
		return beanIDs
	}

	tasksEndIdx := 1 + len(p.focusedWork.Tasks)

	// Task selected - return only task's beans
	taskIdx := p.selectedIndex - 1
	if taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks) {
		var beanIDs []string
		for _, bp := range p.focusedWork.Tasks[taskIdx].Beans {
			beanIDs = append(beanIDs, bp.ID)
		}
		return beanIDs
	}

	// Unassigned bean selected - return just that bean
	unassignedIdx := p.selectedIndex - tasksEndIdx
	if unassignedIdx >= 0 && unassignedIdx < len(p.focusedWork.UnassignedBeans) {
		return []string{p.focusedWork.UnassignedBeans[unassignedIdx].ID}
	}

	return nil
}

// IsTaskSelected returns true if a task is currently selected (vs root issue or unassigned bean)
func (p *WorkOverviewPanel) IsTaskSelected() bool {
	if p.selectedIndex == 0 || p.focusedWork == nil {
		return false
	}
	taskIdx := p.selectedIndex - 1
	return taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks)
}

// IsSelectedTaskFailed returns true if the selected task has failed status
func (p *WorkOverviewPanel) IsSelectedTaskFailed() bool {
	if !p.IsTaskSelected() {
		return false
	}
	taskIdx := p.selectedIndex - 1
	if taskIdx >= 0 && taskIdx < len(p.focusedWork.Tasks) {
		return p.focusedWork.Tasks[taskIdx].Task.Status == db.StatusFailed
	}
	return false
}

// IsUnassignedBeanSelected returns true if an unassigned bean is currently selected
func (p *WorkOverviewPanel) IsUnassignedBeanSelected() bool {
	if p.focusedWork == nil {
		return false
	}
	tasksEndIdx := 1 + len(p.focusedWork.Tasks)
	unassignedIdx := p.selectedIndex - tasksEndIdx
	return unassignedIdx >= 0 && unassignedIdx < len(p.focusedWork.UnassignedBeans)
}

// GetSelectedUnassignedBeanID returns the ID of the selected unassigned bean, or empty if none selected
func (p *WorkOverviewPanel) GetSelectedUnassignedBeanID() string {
	if !p.IsUnassignedBeanSelected() {
		return ""
	}
	tasksEndIdx := 1 + len(p.focusedWork.Tasks)
	unassignedIdx := p.selectedIndex - tasksEndIdx
	if unassignedIdx >= 0 && unassignedIdx < len(p.focusedWork.UnassignedBeans) {
		return p.focusedWork.UnassignedBeans[unassignedIdx].ID
	}
	return ""
}

// SetSelectedTaskID sets selection to the task with given ID
func (p *WorkOverviewPanel) SetSelectedTaskID(id string) {
	if p.focusedWork == nil {
		return
	}
	for i, task := range p.focusedWork.Tasks {
		if task.Task.ID == id {
			p.selectedIndex = i + 1 // +1 because 0 is root issue
			return
		}
	}
}

// NavigateUp moves selection to the previous item
func (p *WorkOverviewPanel) NavigateUp() {
	if p.focusedWork == nil {
		return
	}
	if p.selectedIndex > 0 {
		p.selectedIndex--
	}
}

// NavigateDown moves selection to the next item
func (p *WorkOverviewPanel) NavigateDown() {
	if p.focusedWork == nil {
		return
	}
	// 0 = root, 1..n = tasks, n+1..m = unassigned beans
	maxIndex := len(p.focusedWork.Tasks) + len(p.focusedWork.UnassignedBeans)
	if p.selectedIndex < maxIndex {
		p.selectedIndex++
	}
}

// Render returns the left panel content
func (p *WorkOverviewPanel) Render(panelHeight, panelWidth int) string {
	var content strings.Builder

	if p.focusedWork == nil {
		content.WriteString("Loading work details...")
		return content.String()
	}

	// Account for padding (tuiPanelStyle has Padding(0, 1) = 2 chars total)
	contentWidth := panelWidth - 2

	// Work header (1 line)
	workHeader := fmt.Sprintf("%s %s", statusIcon(p.focusedWork.Work.Status), p.focusedWork.Work.ID)
	if p.focusedWork.Work.Name != "" {
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
		// Calculate available space for name
		maxNameLen := contentWidth - 4 - len(p.focusedWork.Work.ID)

		// Add creation time (if it will fit)
		var timeStr string
		if p.focusedWork.Work.CreatedAt.Unix() > 0 {
			timeAgo := time.Since(p.focusedWork.Work.CreatedAt)
			if timeAgo.Hours() < 1 {
				timeStr = fmt.Sprintf(" (%dm ago)", int(timeAgo.Minutes()))
			} else if timeAgo.Hours() < 24 {
				timeStr = fmt.Sprintf(" (%dh ago)", int(timeAgo.Hours()))
			} else {
				days := int(timeAgo.Hours() / 24)
				timeStr = fmt.Sprintf(" (%dd ago)", days)
			}
			maxNameLen -= len(timeStr)
		}

		if maxNameLen > 0 {
			workHeader += " " + nameStyle.Render(ansi.Truncate(p.focusedWork.Work.Name, maxNameLen, "..."))
		}

		// Add time string at the end
		if timeStr != "" {
			timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			workHeader += timeStyle.Render(timeStr)
		}
	} else {
		// If no name, just show creation time
		if p.focusedWork.Work.CreatedAt.Unix() > 0 {
			timeAgo := time.Since(p.focusedWork.Work.CreatedAt)
			var timeStr string
			if timeAgo.Hours() < 1 {
				timeStr = fmt.Sprintf(" (%dm ago)", int(timeAgo.Minutes()))
			} else if timeAgo.Hours() < 24 {
				timeStr = fmt.Sprintf(" (%dh ago)", int(timeAgo.Hours()))
			} else {
				days := int(timeAgo.Hours() / 24)
				timeStr = fmt.Sprintf(" (%dd ago)", days)
			}
			timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			workHeader += timeStyle.Render(timeStr)
		}
	}
	content.WriteString(workHeader + "\n")
	// Branch info (1 line) - "Branch: " is 8 chars
	fmt.Fprintf(&content, "Branch: %s\n", ansi.Truncate(p.focusedWork.Work.BranchName, contentWidth-8, "..."))

	// Progress percentage and warnings (1 line)
	var progressLine strings.Builder

	// Calculate progress
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

	// Progress percentage
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
	progressLine.WriteString("Progress: ")
	progressLine.WriteString(progressStyle.Render(fmt.Sprintf("%d%%", percentage)))
	progressLine.WriteString(fmt.Sprintf(" (%d/%d tasks)", completedTasks, len(p.focusedWork.Tasks)))

	// Warning badges
	if p.focusedWork.UnassignedBeanCount > 0 {
		warningStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		progressLine.WriteString("  ")
		progressLine.WriteString(warningStyle.Render(fmt.Sprintf("⚠ %d unassigned", p.focusedWork.UnassignedBeanCount)))
	}
	if p.focusedWork.FeedbackCount > 0 {
		alertStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		progressLine.WriteString("  ")
		progressLine.WriteString(alertStyle.Render("feedback"))
	}

	content.WriteString(progressLine.String() + "\n")

	// Orchestrator health (1 line) - only show if work is processing or has active tasks
	hasActiveTask := false
	for _, task := range p.focusedWork.Tasks {
		if task.Task.Status == db.StatusProcessing {
			hasActiveTask = true
			break
		}
	}
	// Base header lines: work header (1), branch (1), progress (1), separator (1) = 4
	headerLines := 4
	if p.focusedWork.Work.Status == db.StatusProcessing || hasActiveTask {
		if p.orchestratorHealthy {
			healthStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
			content.WriteString(healthStyle.Render("✓ Orchestrator running"))
		} else {
			healthStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
			content.WriteString(healthStyle.Render("✗ Orchestrator dead [o] restart"))
		}
		content.WriteString("\n")
		headerLines = 5 // Add one for orchestrator health line
	}

	// Separator (1 line)
	content.WriteString(strings.Repeat("─", contentWidth))
	content.WriteString("\n")
	availableLines := max(panelHeight-headerLines-1, 1)

	// Total items: 1 root issue + n tasks + unassigned beans (if any)
	totalItems := 1 + len(p.focusedWork.Tasks) + len(p.focusedWork.UnassignedBeans)

	// Calculate scroll window
	startIdx := 0
	if p.selectedIndex >= availableLines && availableLines > 0 {
		startIdx = max(0, p.selectedIndex-availableLines/2)
	}
	endIdx := min(startIdx+availableLines, totalItems)

	// Render visible items (use contentWidth which accounts for padding)
	// Layout: index 0 = root issue, 1..n = tasks, n+1..m = unassigned beans
	tasksEndIdx := 1 + len(p.focusedWork.Tasks)
	for i := startIdx; i < endIdx; i++ {
		var itemLine string
		var zoneID string
		if i == 0 {
			// Root issue
			itemLine = p.renderRootIssueLine(contentWidth)
			zoneID = p.zonePrefix + "root"
		} else if i < tasksEndIdx {
			// Task (index i-1 in tasks array)
			taskIdx := i - 1
			if taskIdx < len(p.focusedWork.Tasks) {
				itemLine = p.renderTaskLine(taskIdx, contentWidth)
				zoneID = p.zonePrefix + "task-" + p.focusedWork.Tasks[taskIdx].Task.ID
			}
		} else {
			// Unassigned bean (index i - tasksEndIdx in unassignedBeans array)
			unassignedIdx := i - tasksEndIdx
			if unassignedIdx < len(p.focusedWork.UnassignedBeans) {
				itemLine = p.renderUnassignedBeanLine(unassignedIdx, contentWidth)
				zoneID = p.zonePrefix + "bean-" + p.focusedWork.UnassignedBeans[unassignedIdx].ID
			}
		}
		if zoneID != "" && itemLine != "" {
			content.WriteString(zone.Mark(zoneID, itemLine))
		}
	}

	// Scroll indicator
	if totalItems > availableLines && availableLines > 0 {
		scrollInfo := fmt.Sprintf("(%d-%d of %d)", startIdx+1, endIdx, totalItems)
		content.WriteString(tuiDimStyle.Render(scrollInfo))
	}

	return content.String()
}

// renderRootIssueLine renders the root issue line and returns it
func (p *WorkOverviewPanel) renderRootIssueLine(panelWidth int) string {
	var content strings.Builder
	isSelected := p.selectedIndex == 0
	isHovered := p.hoveredIndex == 0

	prefix := "  "
	if isSelected {
		prefix = "► "
	}

	// Find root issue info from workBeans
	rootID := p.focusedWork.Work.RootIssueID
	rootTitle := ""
	for _, bean := range p.focusedWork.WorkBeans {
		if bean.ID == rootID {
			rootTitle = bean.Title
			break
		}
	}

	// Build the icon - style depends on hover/selected state
	var issueIcon string
	if isSelected || isHovered {
		// Use plain icon when applying full line style
		issueIcon = "◆"
	} else {
		// Styled icon for normal display
		issueIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Render("◆")
	}

	// Build text portion (ID and title)
	textPortion := rootID
	if rootTitle != "" {
		// Calculate max title length: panelWidth - prefix(2) - icon(1) - spaces(2) - ID - buffer
		maxTitleLen := panelWidth - 2 - 1 - 2 - len(rootID) - 4
		if maxTitleLen > 0 {
			textPortion += " " + ansi.Truncate(rootTitle, maxTitleLen, "...")
		}
	}

	content.WriteString(prefix)
	if isSelected {
		// Full selected style on icon + text
		content.WriteString(tuiSelectedStyle.Render(issueIcon + " " + textPortion))
	} else if isHovered {
		// Orange text for hover on icon + text
		hoverStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		content.WriteString(hoverStyle.Render(issueIcon + " " + textPortion))
	} else {
		// Normal: styled icon + dim text
		content.WriteString(issueIcon + " ")
		content.WriteString(tuiDimStyle.Render(textPortion))
	}
	content.WriteString("\n")
	return content.String()
}

// renderTaskLine renders a task line and returns it
func (p *WorkOverviewPanel) renderTaskLine(taskIdx int, _ int) string {
	var content strings.Builder
	task := p.focusedWork.Tasks[taskIdx]
	itemIndex := taskIdx + 1 // +1 because 0 is root issue

	isSelected := p.selectedIndex == itemIndex
	isHovered := p.hoveredIndex == itemIndex

	prefix := "  "
	if isSelected {
		prefix = "► "
	}

	// Status icon (plain or styled depending on hover/selected state)
	statusStr := ""
	switch task.Task.Status {
	case db.StatusCompleted:
		statusStr = "✓"
	case db.StatusProcessing:
		statusStr = "●"
	case db.StatusFailed:
		statusStr = "✗"
	default:
		statusStr = "○"
	}

	// Task type
	taskType := "impl"
	switch task.Task.TaskType {
	case "estimate":
		taskType = "est"
	case "review":
		taskType = "rev"
	case "pr":
		taskType = "pr"
	case "update-pr-description":
		taskType = "pr-upd"
	case "log_analysis":
		taskType = "log"
	}

	content.WriteString(prefix)
	if isSelected {
		// Full selected style on entire line
		textContent := fmt.Sprintf("%s %s [%s]", statusStr, task.Task.ID, taskType)
		content.WriteString(tuiSelectedStyle.Render(textContent))
	} else if isHovered {
		// Orange text for hover on entire line
		hoverStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		textContent := fmt.Sprintf("%s %s [%s]", statusStr, task.Task.ID, taskType)
		content.WriteString(hoverStyle.Render(textContent))
	} else {
		// Normal: styled status icon + dim text
		var statusStyle lipgloss.Style
		switch task.Task.Status {
		case db.StatusCompleted:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
		case db.StatusProcessing:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		case db.StatusFailed:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		default:
			statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
		}
		content.WriteString(statusStyle.Render(statusStr))
		content.WriteString(" ")
		content.WriteString(tuiDimStyle.Render(fmt.Sprintf("%s [%s]", task.Task.ID, taskType)))
	}
	content.WriteString("\n")
	return content.String()
}

// renderUnassignedBeanLine renders an unassigned bean line and returns it
func (p *WorkOverviewPanel) renderUnassignedBeanLine(beanIdx, panelWidth int) string {
	if beanIdx >= len(p.focusedWork.UnassignedBeans) {
		return ""
	}

	var content strings.Builder
	bean := p.focusedWork.UnassignedBeans[beanIdx]
	tasksEndIdx := 1 + len(p.focusedWork.Tasks)
	itemIdx := tasksEndIdx + beanIdx

	isSelected := p.selectedIndex == itemIdx
	isHovered := p.hoveredIndex == itemIdx

	prefix := "  "
	if isSelected {
		prefix = "► "
	}

	// Build text portion (ID and title)
	textPortion := bean.ID
	if bean.Title != "" {
		// Calculate max title length: panelWidth - prefix(2) - icon(1) - spaces(2) - ID - buffer
		maxTitleLen := panelWidth - 2 - 1 - 2 - len(bean.ID) - 4
		if maxTitleLen > 0 {
			textPortion += " " + ansi.Truncate(bean.Title, maxTitleLen, "...")
		}
	}

	content.WriteString(prefix)
	if isSelected {
		// Full selected style on icon + text
		content.WriteString(tuiSelectedStyle.Render("○ " + textPortion))
	} else if isHovered {
		// Orange text for hover on icon + text
		hoverStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
		content.WriteString(hoverStyle.Render("○ " + textPortion))
	} else {
		// Normal: orange icon for unassigned + dim text
		beanIcon := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("○")
		content.WriteString(beanIcon + " ")
		content.WriteString(tuiDimStyle.Render(textPortion))
	}
	content.WriteString("\n")
	return content.String()
}

// DetectClickedItem determines which item was clicked using bubblezone and returns its index
func (p *WorkOverviewPanel) DetectClickedItem(msg tea.MouseMsg) int {
	if p.focusedWork == nil {
		return -1
	}

	// Check root issue zone
	if zone.Get(p.zonePrefix + "root").InBounds(msg) {
		return 0
	}

	// Check task zones
	for i, task := range p.focusedWork.Tasks {
		if zone.Get(p.zonePrefix + "task-" + task.Task.ID).InBounds(msg) {
			return i + 1 // +1 because 0 is root issue
		}
	}

	// Check unassigned bean zones
	tasksEndIdx := 1 + len(p.focusedWork.Tasks)
	for i, bean := range p.focusedWork.UnassignedBeans {
		if zone.Get(p.zonePrefix + "bean-" + bean.ID).InBounds(msg) {
			return tasksEndIdx + i
		}
	}

	return -1
}

// DetectHoveredItem determines which item is at the mouse position for hover detection.
// Returns the absolute index (0 = root, 1+ = tasks, N+ = unassigned beans), or -1 if not over an item.
func (p *WorkOverviewPanel) DetectHoveredItem(msg tea.MouseMsg) int {
	// Reuse click detection logic since hover uses the same boundaries
	return p.DetectClickedItem(msg)
}
