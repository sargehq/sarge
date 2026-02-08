package tui

import (
	"fmt"
	"strings"
	"time"

	progressbar "github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
)

// renderTwoColumnLayout renders the issues and details panels side-by-side
func (m *planModel) renderTwoColumnLayout() string {
	// Calculate content height
	// Note: m.height has already been adjusted for tabs bar in View()
	contentHeight := m.height - 1 // -1 for status bar

	// Dimming: unfocused panels get gray borders + gray content
	issuesDimmed := m.activePanel != PanelLeft
	rightDimmed := m.activePanel != PanelRight

	// Use panels for rendering (they're already synced with correct sizes and data)
	issuesPanel := m.issuesPanel.RenderWithPanel(contentHeight, issuesDimmed)

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
		rightPanel = m.detailsPanel.RenderWithPanel(contentHeight, rightDimmed)
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

// viewTabsHeight returns the height of the view tabs bar.
// Returns 0 when on splash screen (no tabs shown), 3 otherwise
// (top border + label + bottom border from bubbletea tabs pattern).
func (m *planModel) viewTabsHeight() int {
	if m.shouldShowSplash() {
		return 0
	}
	return 3
}

// renderViewTabs renders the Prompt/Issues tab bar using the bubbletea tabs pattern
// with connected borders (active tab bottom opens into the content below).
func (m *planModel) renderViewTabs() string {
	t := CurrentTheme()
	borderColor := t.Dim

	// Helper to customize bottom border characters (from bubbletea tabs example)
	tabBorderWithBottom := func(left, middle, right string) lipgloss.Border {
		border := lipgloss.RoundedBorder()
		border.BottomLeft = left
		border.Bottom = middle
		border.BottomRight = right
		return border
	}

	inactiveTabBorder := tabBorderWithBottom("┴", "─", "┴")
	activeTabBorder := tabBorderWithBottom("┘", " ", "└")

	inactiveTabStyle := lipgloss.NewStyle().
		Border(inactiveTabBorder, true).
		BorderForeground(borderColor).
		Foreground(t.Dim).
		Padding(0, 1)

	activeTabStyle := lipgloss.NewStyle().
		Border(activeTabBorder, true).
		BorderForeground(borderColor).
		Foreground(t.Text).
		Bold(true).
		Padding(0, 1)

	type viewTab struct {
		label, id string
		active    bool
	}
	tabs := []viewTab{
		{"Prompt", "prompt", m.topView == "prompt"},
		{"Issues", "issues", m.topView == "issues"},
	}

	var renderedTabs []string
	for _, tb := range tabs {
		style := inactiveTabStyle
		if tb.active {
			style = activeTabStyle
		}
		rendered := style.Render(tb.label)
		renderedTabs = append(renderedTabs, zone.Mark("viewtab-"+tb.id, rendered))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)

	// Fill remaining width: extend the bottom border line across the full width.
	// Build a 3-line gap element matching tab height so the border line aligns
	// with the tab bottom borders.
	// Line 2 (middle) contains breadcrumb filters when works exist.
	rowWidth := lipgloss.Width(row)
	gapWidth := max(0, m.width-rowWidth)
	if gapWidth > 0 {
		lineStyle := lipgloss.NewStyle().Foreground(borderColor)
		blankLine := strings.Repeat(" ", gapWidth)
		borderLine := lineStyle.Render(strings.Repeat("─", gapWidth))

		// Build breadcrumb content for the middle line
		breadcrumbLine := m.renderBreadcrumbs(gapWidth)
		if breadcrumbLine == "" {
			breadcrumbLine = blankLine
		}

		gap := blankLine + "\n" + breadcrumbLine + "\n" + borderLine
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, gap)
	}

	return row
}

// detectViewTabClick returns "prompt" or "issues" if a view tab was clicked, "" otherwise.
func (m *planModel) detectViewTabClick(msg tea.MouseMsg) string {
	if zone.Get("viewtab-prompt").InBounds(msg) {
		return "prompt"
	}
	if zone.Get("viewtab-issues").InBounds(msg) {
		return "issues"
	}
	return ""
}

// renderBreadcrumbs builds the breadcrumb filter string for the view tabs gap.
// Returns "" if no works exist. maxWidth constrains the rendered width.
func (m *planModel) renderBreadcrumbs(maxWidth int) string {
	workTiles := m.workTabsBar.GetWorkTiles()
	if len(workTiles) == 0 {
		return ""
	}

	dimStyle := lipgloss.NewStyle().Foreground(CurrentTheme().Dim)
	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("255")).
		Foreground(lipgloss.Color("241"))
	unselectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("255"))

	sep := dimStyle.Render(" › ")
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	label := "    " + labelStyle.Render("Filter by")

	var crumbs []string

	// "All" breadcrumb
	allID := m.breadcrumbZonePrefix + "workfilter-all"
	if m.focusedWorkID == "" {
		crumbs = append(crumbs, zone.Mark(allID, selectedStyle.Render(" All ")))
	} else {
		crumbs = append(crumbs, zone.Mark(allID, unselectedStyle.Render(" All ")))
	}

	// One breadcrumb per work
	for _, work := range workTiles {
		if work == nil {
			continue
		}
		name := work.Work.ID
		if work.Work.Name != "" {
			name = work.Work.Name
		}
		name = ansi.Truncate(name, 20, "…")

		// Status icon
		icon := m.workTabsBar.WorkStateIcon(work)

		text := fmt.Sprintf(" %s %s ", icon, name)
		crumbID := m.breadcrumbZonePrefix + "workfilter-" + work.Work.ID
		if m.focusedWorkID == work.Work.ID {
			crumbs = append(crumbs, zone.Mark(crumbID, selectedStyle.Render(text)))
		} else {
			crumbs = append(crumbs, zone.Mark(crumbID, unselectedStyle.Render(text)))
		}
	}

	result := label + sep + strings.Join(crumbs, sep)

	// Pad to fill maxWidth
	resultWidth := lipgloss.Width(result)
	if resultWidth < maxWidth {
		result += strings.Repeat(" ", maxWidth-resultWidth)
	}

	return result
}

// detectBreadcrumbClick checks if a breadcrumb was clicked.
// Returns "all" for the All filter, a work ID for a work filter, or "" if no breadcrumb clicked.
func (m *planModel) detectBreadcrumbClick(msg tea.MouseMsg) string {
	// Check "All" breadcrumb
	if zone.Get(m.breadcrumbZonePrefix + "workfilter-all").InBounds(msg) {
		return "all"
	}

	// Check each work breadcrumb
	workTiles := m.workTabsBar.GetWorkTiles()
	for _, work := range workTiles {
		if work == nil {
			continue
		}
		if zone.Get(m.breadcrumbZonePrefix + "workfilter-" + work.Work.ID).InBounds(msg) {
			return work.Work.ID
		}
	}

	return ""
}

func (m *planModel) renderWithDialog(dialog string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// renderWorkContextPanel renders a compact info panel when a work filter is active.
// Shows status, name, branch, progress bar, PR info, and applicable command hints
// inside a rounded border. Returns "" when no work is selected.
func (m *planModel) renderWorkContextPanel() string {
	if m.focusedWorkID == "" {
		return ""
	}

	work := m.findWorkByID(m.focusedWorkID)
	if work == nil {
		return ""
	}

	t := CurrentTheme()

	// Panel inner width: total - border(2) - padding(2)
	innerWidth := m.width - 4

	// Work name
	name := work.Work.Name
	if name == "" {
		name = work.Work.ID
	}

	// Status with icon
	icon := m.workTabsBar.WorkStateIcon(work)
	statusStyle := lipgloss.NewStyle()
	switch work.Work.Status {
	case db.StatusCompleted:
		statusStyle = statusStyle.Foreground(t.StatusCompleted)
	case db.StatusProcessing:
		statusStyle = statusStyle.Foreground(t.StatusProcessing)
	case db.StatusFailed:
		statusStyle = statusStyle.Foreground(t.StatusFailed)
	case db.StatusMerged:
		statusStyle = statusStyle.Foreground(t.Merged)
	default:
		statusStyle = statusStyle.Foreground(t.StatusIdle)
	}

	// Progress calculation
	completedTasks := 0
	for _, task := range work.Tasks {
		if task.Task.Status == db.StatusCompleted {
			completedTasks++
		}
	}
	totalTasks := len(work.Tasks)
	pct := 0.0
	if totalTasks > 0 {
		pct = float64(completedTasks) / float64(totalTasks)
	}

	// Line 1: icon + status + name + branch
	var line1Parts []string
	line1Parts = append(line1Parts, icon+" "+statusStyle.Render(work.Work.Status))
	line1Parts = append(line1Parts, lipgloss.NewStyle().Bold(true).Foreground(t.Text).Render(name))
	if work.Work.BranchName != "" {
		branchStr := ansi.Truncate(work.Work.BranchName, 30, "…")
		line1Parts = append(line1Parts, tuiDimStyle().Render(branchStr))
	}

	// Progress bar (compact, inline on line 1)
	if totalTasks > 0 {
		barWidth := 16
		if innerWidth < 80 {
			barWidth = 10
		}
		prog := progressbar.New(
			progressbar.WithDefaultGradient(),
			progressbar.WithoutPercentage(),
			progressbar.WithWidth(barWidth),
		)
		progressLabel := fmt.Sprintf(" %d/%d", completedTasks, totalTasks)
		line1Parts = append(line1Parts, prog.ViewAs(pct)+tuiDimStyle().Render(progressLabel))
	}

	var lines []string
	lines = append(lines, strings.Join(line1Parts, "  "))

	// Line 2: PR info (only when a PR exists)
	if work.Work.PRURL != "" {
		var prParts []string
		prStyle := lipgloss.NewStyle().Foreground(t.Cyan)
		maxURL := 60
		if innerWidth < 80 {
			maxURL = 40
		}
		prURL := ansi.Truncate(work.Work.PRURL, maxURL, "…")
		prParts = append(prParts, "PR: "+prStyle.Render(prURL))

		// CI Status
		if work.CIStatus != "" {
			ciIcon := "⏳"
			ciText := "Pending"
			ciColor := t.CIPending
			switch work.CIStatus {
			case db.CIStatusSuccess:
				ciIcon = "✓"
				ciText = "Passing"
				ciColor = t.CISuccess
			case db.CIStatusFailure:
				ciIcon = "✗"
				ciText = "Failing"
				ciColor = t.Error
			}
			ciStyle := lipgloss.NewStyle().Foreground(ciColor)
			prParts = append(prParts, "CI: "+ciStyle.Render(ciIcon+" "+ciText))
		}

		// Approval status (only non-pending)
		if work.ApprovalStatus != "" && work.ApprovalStatus != db.ApprovalStatusPending {
			switch work.ApprovalStatus {
			case db.ApprovalStatusApproved:
				prParts = append(prParts, lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Render("✓ Approved"))
			case db.ApprovalStatusChangesRequested:
				prParts = append(prParts, lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("⚠ Changes"))
			}
		}

		lines = append(lines, strings.Join(prParts, "  "))
	}

	// Last line: conditional command hints — only show applicable commands
	lines = append(lines, m.renderWorkCommands(work))

	content := strings.Join(lines, "\n")

	// Bordered panel with accent-colored border
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent).
		Padding(0, 1).
		Width(m.width)

	return panelStyle.Render(content)
}

// renderWorkCommands builds a command hint line for the work context panel.
// Only shows commands that apply to the current work state; inapplicable ones are dimmed.
func (m *planModel) renderWorkCommands(work *progress.WorkProgress) string {
	dim := tuiDimStyle()
	hasPR := work.Work.PRURL != ""
	isIdle := work.Work.Status == db.StatusIdle
	isCompleted := work.Work.Status == db.StatusCompleted
	isFailed := work.Work.Status == db.StatusFailed
	hasTasks := len(work.Tasks) > 0

	// Run: applicable when work has unprocessed beads (idle/failed/pending)
	// Review: applicable when work has tasks
	// PR: applicable when work is completed and no PR yet
	// Feedback: applicable when work has a PR
	// Term/Claude: always applicable (infrastructure)
	// Destroy: always applicable

	var parts []string

	parts = append(parts, styleHotkeys("[T]erm"))
	parts = append(parts, styleHotkeys("[C]laude"))

	if isIdle || isFailed || !hasTasks {
		parts = append(parts, styleHotkeys("[R]un"))
	} else {
		parts = append(parts, dim.Render("[R]un"))
	}

	if hasTasks {
		parts = append(parts, styleHotkeys("re[V]iew"))
	} else {
		parts = append(parts, dim.Render("re[V]iew"))
	}

	if isCompleted && !hasPR {
		parts = append(parts, styleHotkeys("[P]R"))
	} else {
		parts = append(parts, dim.Render("[P]R"))
	}

	if hasPR {
		parts = append(parts, styleHotkeys("[F]eedback"))
	}

	parts = append(parts, styleHotkeys("[D]estroy"))

	return strings.Join(parts, " ")
}

func (m *planModel) renderHelp() string {
	help := `
  Plan Mode - Help

  Each issue gets its own dedicated agent session in a separate tab.
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
  P             Issue is processing (active agent session)
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

	// Determine scroll direction
	scrollUp := msg.Button == tea.MouseButtonWheelUp

	// Calculate panel widths
	totalContentWidth := m.width - 4
	leftPanelWidth := int(float64(totalContentWidth) * m.columnRatio)

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
