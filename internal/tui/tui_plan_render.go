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
	case ViewCreateBead, ViewCreateBeadInline, ViewAddChildBead, ViewEditBead:
		m.beadFormPanel.SetSize(detailsWidth, planPanelHeight)
		detailsPanel = m.beadFormPanel.RenderWithPanel(planPanelHeight)
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
	case ViewCreateBead, ViewCreateBeadInline, ViewAddChildBead, ViewEditBead:
		rightPanel = m.beadFormPanel.RenderWithPanel(contentHeight)
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
// Returns the absolute index in m.beadItems, or -1 if not over an issue
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
	if m.viewMode != ViewCreateBead && m.viewMode != ViewCreateBeadInline &&
		m.viewMode != ViewAddChildBead && m.viewMode != ViewEditBead &&
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
	help := `
  Plan Mode - Help

  Each issue gets its own dedicated Claude session in a separate tab.
  Use 'p' to start or resume a planning session for an issue.

  Layout
  ────────────────────────────
  Two-column layout:
    - Left: Issues list (default 40% width)
    - Right: Issue details (default 60% width)
  [ / ]         Adjust column ratio (30/70, 40/60, 50/50)

  Navigation
  ────────────────────────────
  j/k, ↑/↓      Navigate list
  1-9           Select work by position
  p             Start/Resume planning session

  Issue Management
  ────────────────────────────
  n             Create new issue (any type)
  e             Edit issue inline (textarea)
  E             Edit issue in $EDITOR
  a             Add child issue (blocked by selected)
  x             Close selected issue
  d             Delete issue (permanent removal)
  Space         Toggle issue selection (for multi-select)
  w             Create work from issue(s)
  A             Add issue to existing work
  i             Import issue from Linear
  I             Import from GitHub PR

  Filtering & Sorting
  ────────────────────────────
  o             Show open issues
  c             Show closed issues
  r             Show ready issues
  /             Fuzzy search
  L             Filter by label
  s             Cycle sort mode
  v             Toggle expanded view

  Indicators
  ────────────────────────────
  ●             Issue is selected for multi-select
  P             Issue is processing (active Claude session)
  [w-xxx]       Issue is assigned to work w-xxx

  Press any key to close...
`
	return tuiHelpStyle().Width(m.width).Height(m.height).Render(help)
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
			if m.beadsCursor > 0 {
				m.beadsCursor--
			}
		} else {
			if m.beadsCursor < len(m.beadItems)-1 {
				m.beadsCursor++
			}
		}
		return m, nil
	}

	// Normal mode (no focused work) - check which panel mouse is over

	// Check if mouse is over the issues panel (left side)
	if msg.X <= leftPanelWidth+2 {
		// Issues panel - move cursor
		if scrollUp {
			if m.beadsCursor > 0 {
				m.beadsCursor--
			}
		} else {
			if m.beadsCursor < len(m.beadItems)-1 {
				m.beadsCursor++
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
