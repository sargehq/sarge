package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// renderFocusedWorkSplitView renders the split view when a work is focused
// This shows a horizontal split: Work details on top (40%), Issues/Details below (60%)
func (m *planModel) renderFocusedWorkSplitView() string {
	// Calculate heights for split view
	// Note: m.height has already been adjusted for tabs bar in View()
	totalHeight := m.height - 1 // -1 for status bar

	// Calculate work panel height
	// calculateWorkPanelHeight returns content height (10-23)
	// Add 2 for border to get total panel height
	calcBase := m.calculateWorkPanelHeight()
	workPanelHeight := calcBase + 2
	planPanelHeight := totalHeight - workPanelHeight

	// Update work details panel size and render (same pattern as IssuesPanel)
	m.workDetails.SetSize(m.width, workPanelHeight)
	workPanel := m.workDetails.RenderWithPanel(workPanelHeight)

	// === Render Plan Mode Panel (Bottom) ===
	// Update issues and details panel sizes for the reduced height
	totalContentWidth := m.width - 4
	issuesWidth := int(float64(totalContentWidth) * m.columnRatio)
	detailsWidth := totalContentWidth - issuesWidth

	// Temporarily update panel sizes for the reduced height
	m.issuesPanel.SetSize(issuesWidth, planPanelHeight)
	m.detailsPanel.SetSize(detailsWidth, planPanelHeight)

	// Render issues panel
	issuesPanel := m.issuesPanel.RenderWithPanel(planPanelHeight)

	// Select the right panel based on view mode
	var detailsPanel string
	switch m.viewMode {
	case ViewCreateBean, ViewCreateBeanInline, ViewAddChildBean, ViewEditBean:
		m.beanFormPanel.SetSize(detailsWidth, planPanelHeight)
		detailsPanel = m.beanFormPanel.RenderWithPanel(planPanelHeight)
	case ViewLinearImportInline:
		m.linearImportPanel.SetSize(detailsWidth, planPanelHeight)
		detailsPanel = m.linearImportPanel.RenderWithPanel(planPanelHeight)
	case ViewPRImportInline:
		m.prImportPanel.SetSize(detailsWidth, planPanelHeight)
		detailsPanel = m.prImportPanel.RenderWithPanel(planPanelHeight)
	case ViewCreateWork:
		m.createWorkPanel.SetSize(detailsWidth, planPanelHeight)
		detailsPanel = m.createWorkPanel.RenderWithPanel(planPanelHeight)
	default:
		detailsPanel = m.detailsPanel.RenderWithPanel(planPanelHeight)
	}

	// Combine plan mode columns (panels have their own borders)
	planSection := lipgloss.JoinHorizontal(lipgloss.Top, issuesPanel, detailsPanel)

	// Combine everything vertically (panel borders provide visual separation)
	return lipgloss.JoinVertical(lipgloss.Left, workPanel, planSection)
}

// renderTwoColumnLayout renders the issues and details panels side-by-side
func (m *planModel) renderTwoColumnLayout() string {
	// Check if a work is focused - if so, render split view
	if m.focusedWorkID != "" {
		return m.renderFocusedWorkSplitView()
	}

	// Calculate content height
	// Note: m.height has already been adjusted for tabs bar in View()
	contentHeight := m.height - 1 // -1 for status bar

	// Use panels for rendering (they're already synced with correct sizes and data)
	issuesPanel := m.issuesPanel.RenderWithPanel(contentHeight)

	// Select the right panel based on view mode
	var rightPanel string
	switch m.viewMode {
	case ViewCreateBean, ViewCreateBeanInline, ViewAddChildBean, ViewEditBean:
		rightPanel = m.beanFormPanel.RenderWithPanel(contentHeight)
	case ViewLinearImportInline:
		rightPanel = m.linearImportPanel.RenderWithPanel(contentHeight)
	case ViewPRImportInline:
		rightPanel = m.prImportPanel.RenderWithPanel(contentHeight)
	case ViewCreateWork:
		rightPanel = m.createWorkPanel.RenderWithPanel(contentHeight)
	default:
		rightPanel = m.detailsPanel.RenderWithPanel(contentHeight)
	}

	// Combine columns horizontally (panels have their own borders)
	return lipgloss.JoinHorizontal(lipgloss.Top, issuesPanel, rightPanel)
}

// detectCommandsBarButton determines which button is at the mouse position in the commands bar
func (m *planModel) detectCommandsBarButton(msg tea.MouseMsg) string {
	// Delegate to the status bar panel
	return m.statusBar.DetectButton(msg)
}

// detectHoveredIssue determines which issue is at the mouse position using bubblezone
// Returns the absolute index in m.beanItems, or -1 if not over an issue
func (m *planModel) detectHoveredIssue(msg tea.MouseMsg) int {
	return m.issuesPanel.DetectHoveredIssue(msg)
}

// calculateWorkPanelHeight returns the height of the work details panel content.
// This returns the content height, not including the panel border (+2).
// NOTE: This function assumes m.height has been adjusted for tabs bar (as done in View()).
// For event handling where m.height is the original value, use calculateWorkPanelHeightForEvents().
func (m *planModel) calculateWorkPanelHeight() int {
	// Calculate based on available height
	// Note: m.height has already been adjusted for tabs bar in View()
	availableHeight := m.height - 1 // -1 for status bar
	dropdownHeight := int(float64(availableHeight) * 0.4)
	if dropdownHeight < 10 {
		dropdownHeight = 10
	} else if dropdownHeight > 23 {
		dropdownHeight = 23
	}
	return dropdownHeight
}

// calculateWorkPanelHeightForEvents returns the work panel height for event handling.
// Unlike calculateWorkPanelHeight(), this function works with the original m.height
// (not temporarily reduced for tabs bar) which is the case during event handling.
func (m *planModel) calculateWorkPanelHeightForEvents() int {
	tabsBarHeight := m.workTabsBar.Height()
	// Subtract tabs bar and status bar from original height
	availableHeight := m.height - tabsBarHeight - 1
	dropdownHeight := int(float64(availableHeight) * 0.4)
	if dropdownHeight < 10 {
		dropdownHeight = 10
	} else if dropdownHeight > 23 {
		dropdownHeight = 23
	}
	return dropdownHeight
}

// detectHoveredIssueWithOffset detects issue hover when content is offset by work panel
// With bubblezone, this is the same as detectHoveredIssue since zones handle coordinates
func (m *planModel) detectHoveredIssueWithOffset(msg tea.MouseMsg) int {
	return m.issuesPanel.DetectHoveredIssue(msg)
}

// detectClickedPanel determines which panel was clicked in the focused work view
// Returns "work-left", "work-right", "issues-left", "issues-right", or "" if not in a panel
func (m *planModel) detectClickedPanel(msg tea.MouseMsg) string {
	if m.focusedWorkID == "" {
		return ""
	}

	x, y := msg.X, msg.Y

	// Calculate panel boundaries using calculateWorkPanelHeightForEvents (event handling context)
	tabsBarHeight := m.workTabsBar.Height()
	workPanelHeight := m.calculateWorkPanelHeightForEvents() + 2 // +2 for border
	halfWidth := (m.width - 4) / 2                               // Half width

	// Determine Y section (top = work, bottom = issues)
	// Account for tabs bar at the top
	workPanelEndY := tabsBarHeight + workPanelHeight
	isWorkSection := y >= tabsBarHeight && y < workPanelEndY
	isIssuesSection := y >= workPanelEndY

	// Determine X section (left or right)
	isLeftSide := x <= halfWidth
	isRightSide := x > halfWidth

	if isWorkSection {
		if isLeftSide {
			return "work-left"
		}
		if isRightSide {
			return "work-right"
		}
	}

	if isIssuesSection {
		if isLeftSide {
			return "issues-left"
		}
		if isRightSide {
			return "issues-right"
		}
	}

	return ""
}

// detectDialogButton determines which dialog button is at the mouse position using bubblezone.
// Returns "ok", "cancel", "execute", "auto", "import", or "" if not over a button.
//
// Dialog buttons use fixed zone IDs (e.g., "dialog-ok") without a prefix since only one dialog
// is visible at a time. If multiple simultaneous dialogs are needed, add zone.NewPrefix().
func (m *planModel) detectDialogButton(msg tea.MouseMsg) string {
	// Dialog buttons only visible in form modes, Linear import mode, PR import mode, and work creation mode
	if m.viewMode != ViewCreateBean && m.viewMode != ViewCreateBeanInline &&
		m.viewMode != ViewAddChildBean && m.viewMode != ViewEditBean &&
		m.viewMode != ViewLinearImportInline && m.viewMode != ViewPRImportInline && m.viewMode != ViewCreateWork {
		return ""
	}

	// Check zones for each possible button
	buttons := []string{"ok", "cancel", "execute", "auto", "import"}
	for _, btn := range buttons {
		if zone.Get("dialog-" + btn).InBounds(msg) {
			return btn
		}
	}
	return ""
}

func (m *planModel) renderWithDialog(dialog string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

func (m *planModel) renderHelp() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		MarginBottom(1)

	sectionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("252")).
		MarginTop(1)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245"))

	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("114"))

	sep := dimStyle.Render("─────────────────────────────")

	renderSection := func(title string, entries [][]string, note string) string {
		s := sectionStyle.Render(title) + "\n" + sep + "\n"
		for _, e := range entries {
			s += keyStyle.Render(e[0]) + dimStyle.Render(e[1]) + "\n"
		}
		if note != "" {
			s += "\n" + dimStyle.Render(note) + "\n"
		}
		return s
	}

	pad := func(s string, w int) string {
		for len(s) < w {
			s += " "
		}
		return s
	}

	kw := 16 // key column width
	entry := func(key, desc string) []string {
		return []string{pad(key, kw), desc}
	}

	leftCol := titleStyle.Render("Plan Mode ─ Help") + "\n\n" +
		renderSection("Layout", [][]string{
			entry("[ / ]", "Adjust column ratio"),
		}, "Two-column layout: Issues (left) + Details (right)\nWhen a work is selected (1-9), a work panel appears above.") + "\n" +

		renderSection("Navigation", [][]string{
			entry("j/k  ↑/↓", "Navigate list"),
			entry("Tab", "Cycle focus between panels"),
			entry("1-9", "Select work by position"),
		}, "") + "\n" +

		renderSection("Issue Management", [][]string{
			entry("n", "Create new issue"),
			entry("e", "Edit issue inline"),
			entry("E", "Edit issue in $EDITOR"),
			entry("a", "Add child issue"),
			entry("x", "Close selected issue"),
			entry("d", "Delete issue (permanent)"),
			entry("Space", "Toggle multi-select"),
			entry("w", "Create work from issue(s)"),
			entry("A", "Add issue to focused work"),
			entry("m", "Import from Linear"),
			entry("M", "Import from GitHub PR"),
			entry("p", "Start/Resume planning"),
		}, "")

	rightCol := "\n\n" +
		renderSection("Work Actions", [][]string{
			entry("t", "Open terminal/console"),
			entry("c", "Open agent chat"),
			entry("i", "Open IDE"),
			entry("r", "Run work"),
			entry("o", "Restart orchestrator"),
			entry("v", "Create review task"),
			entry("p", "Create PR / plan session"),
			entry("f", "Check PR feedback"),
			entry("d", "Destroy work / Delete issue"),
			entry("x", "Reset failed task"),
			entry("a", "Add child issue to work"),
			entry("g", "Pick zmx session to attach"),
		}, "Panel-aware: d changes behavior based on\nfocused panel. t/c/i/r/o/v/f/g are exclusively\nwork actions when a work is selected.") + "\n" +

		renderSection("Filters", [][]string{
			entry("O", "Show open issues"),
			entry("C", "Show closed issues"),
			entry("R", "Show ready issues"),
			entry("V", "Toggle expanded view"),
			entry("/", "Fuzzy search"),
			entry("L", "Filter by label"),
			entry("s", "Cycle sort mode"),
			entry("*", "Show all (clear filters)"),
		}, "") + "\n" +

		renderSection("Indicators", [][]string{
			entry("●", "Multi-selected"),
			entry("P", "Processing (active agent)"),
			entry("[w-xxx]", "Assigned to work w-xxx"),
		}, "")

	colWidth := (m.width - 10) / 2 // 10 for padding + gutter
	leftRendered := lipgloss.NewStyle().Width(colWidth).Render(leftCol)
	rightRendered := lipgloss.NewStyle().Width(colWidth).Render(rightCol)

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftRendered, "  ", rightRendered)

	footer := "\n" + dimStyle.Render("Press any key to close...")

	content := body + footer

	return tuiHelpStyle.Width(m.width).Height(m.height).Render(content)
}

// handleMouseWheel handles mouse wheel events by routing them to the appropriate panel
// based on the mouse position. Only the panel under the mouse cursor will scroll.
func (m *planModel) handleMouseWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Debounce rapid scroll events (terminals often send 3+ events per wheel click)
	// 50ms debounce allows continuous scrolling while filtering burst events
	now := time.Now()
	if now.Sub(m.lastWheelScroll) < 50*time.Millisecond {
		return m, nil
	}
	m.lastWheelScroll = now

	// Calculate panel boundaries
	tabsBarHeight := m.workTabsBar.Height()

	// Determine scroll direction
	scrollUp := msg.Button == tea.MouseButtonWheelUp

	// Calculate panel widths
	totalContentWidth := m.width - 4
	leftPanelWidth := int(float64(totalContentWidth) * m.columnRatio)
	rightPanelStartX := leftPanelWidth + 2 // +2 for left panel border

	// If focused work mode, determine if mouse is over work details or issues panel
	if m.focusedWorkID != "" {
		workPanelHeight := m.calculateWorkPanelHeightForEvents() + 2 // +2 for border
		workPanelEndY := tabsBarHeight + workPanelHeight

		// Check if mouse is in work details area (top panel)
		if msg.Y >= tabsBarHeight && msg.Y < workPanelEndY {
			// Check if over the right panel (details)
			if msg.X >= rightPanelStartX {
				// Scroll the work details right panel (summary or task)
				return m, m.workDetails.UpdateViewport(msg)
			}
			// Over left panel (work overview) - navigate task selection
			if scrollUp {
				m.workDetails.NavigateUp()
			} else {
				m.workDetails.NavigateDown()
			}
			return m, nil
		}

		// Mouse is in issues area below work panel
		if scrollUp {
			if m.beansCursor > 0 {
				m.beansCursor--
			}
		} else {
			if m.beansCursor < len(m.beanItems)-1 {
				m.beansCursor++
			}
		}
		return m, nil
	}

	// Normal mode (no focused work) - check which panel mouse is over

	// Check if mouse is over the issues panel (left side)
	if msg.X <= leftPanelWidth+2 {
		// Issues panel - move cursor
		if scrollUp {
			if m.beansCursor > 0 {
				m.beansCursor--
			}
		} else {
			if m.beansCursor < len(m.beanItems)-1 {
				m.beansCursor++
			}
		}
		return m, nil
	}

	// Mouse is over the details panel (right side)
	// Details panel uses viewport for scrolling
	if scrollUp {
		m.detailsPanel.ScrollUp()
	} else {
		m.detailsPanel.ScrollDown()
	}
	return m, nil
}
