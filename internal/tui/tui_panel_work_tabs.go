package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/ptysession"
)

// WorkState represents the current state of a work for display purposes
type WorkState int

const (
	WorkStateIdle      WorkState = iota // Orchestrator alive, no tasks running
	WorkStateRunning                    // Task is processing
	WorkStateCompleted                  // Work is completed
	WorkStateFailed                     // Work failed
	WorkStateDead                       // Orchestrator is dead
	WorkStateMerged                     // PR was merged
)

// WorkTabsBar renders a horizontal tab bar showing all works.
// Each tab can be clicked to focus that work. Running works show a spinner.
// Styled with seamless color transitions between tabs.
type WorkTabsBar struct {
	// Dimensions
	width int

	// Data — new tab model
	tabs          []*WorkTab
	ptyManager    *ptysession.Manager // For checking session liveness

	// Legacy data (kept for compatibility during transition)
	workTiles          []*progress.WorkProgress
	focusedWorkID      string
	hoveredTabID       string
	orchestratorHealth map[string]bool // workID -> orchestrator alive

	// Panel state
	activePanel Panel // Which panel is currently focused

	// The ID of the currently focused/active tab
	activeTabID string

	// Spinner for running works
	spinner spinner.Model

	// Zone prefix for unique zone IDs
	zonePrefix string
}

// NewWorkTabsBar creates a new WorkTabsBar
func NewWorkTabsBar() *WorkTabsBar {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	return &WorkTabsBar{
		width:              80,
		spinner:            s,
		orchestratorHealth: make(map[string]bool),
		zonePrefix:         zone.NewPrefix(),
	}
}

// SetSize updates the bar width
func (b *WorkTabsBar) SetSize(width int) {
	b.width = width
}

// SetTabs sets the tab list
func (b *WorkTabsBar) SetTabs(tabs []*WorkTab) {
	b.tabs = tabs
}

// SetPTYManager sets the PTY manager for session lookups
func (b *WorkTabsBar) SetPTYManager(mgr *ptysession.Manager) {
	b.ptyManager = mgr
}

// SetWorkTiles updates the work tiles data
func (b *WorkTabsBar) SetWorkTiles(workTiles []*progress.WorkProgress) {
	b.workTiles = workTiles
}

// SetFocusedWorkID sets which work is currently focused
func (b *WorkTabsBar) SetFocusedWorkID(id string) {
	b.focusedWorkID = id
}

// SetActiveTabID sets which tab is currently active/focused
func (b *WorkTabsBar) SetActiveTabID(id string) {
	b.activeTabID = id
}

// SetHoveredTabID sets which tab is being hovered
func (b *WorkTabsBar) SetHoveredTabID(id string) {
	b.hoveredTabID = id
}

// SetOrchestratorHealth sets the orchestrator health for a work
func (b *WorkTabsBar) SetOrchestratorHealth(healthMap map[string]bool) {
	b.orchestratorHealth = healthMap
}

// SetActivePanel sets which panel is currently active
func (b *WorkTabsBar) SetActivePanel(panel Panel) {
	b.activePanel = panel
}

// UpdateSpinner updates the spinner animation frame
func (b *WorkTabsBar) UpdateSpinner(s spinner.Model) {
	b.spinner = s
}

// GetSpinner returns the spinner model for update handling
func (b *WorkTabsBar) GetSpinner() spinner.Model {
	return b.spinner
}

// Height returns the height of the tab bar (always 1 line)
func (b *WorkTabsBar) Height() int {
	return 1
}

// getWorkState determines the current state of a work for display
func (b *WorkTabsBar) getWorkState(work *progress.WorkProgress) WorkState {
	if work == nil {
		return WorkStateDead
	}

	// Check if any task is running FIRST - this takes priority over work status
	// because new tasks can be added to idle/completed works
	for _, task := range work.Tasks {
		if task.Task.Status == db.StatusProcessing {
			return WorkStateRunning
		}
	}

	// Then check work status
	switch work.Work.Status {
	case db.StatusMerged:
		return WorkStateMerged
	case db.StatusCompleted:
		return WorkStateCompleted
	case db.StatusFailed:
		return WorkStateFailed
	case db.StatusIdle:
		return WorkStateIdle
	}

	// Check orchestrator health
	if alive, ok := b.orchestratorHealth[work.Work.ID]; ok && !alive {
		return WorkStateDead
	}

	// Default to idle
	return WorkStateIdle
}

// getTabIcon returns the status icon for a tab
func (b *WorkTabsBar) getTabIcon(tab *WorkTab) string {
	switch tab.Type {
	case WorkTabDefault:
		// Default tab: show session state
		if tab.HasActiveSession(b.ptyManager) {
			unstyled := b.spinner
			unstyled.Style = lipgloss.NewStyle()
			return unstyled.View()
		}
		return "⌂" // Home icon for main tab

	case WorkTabPlan:
		// Plan tab: show session state
		if tab.HasActiveSession(b.ptyManager) {
			unstyled := b.spinner
			unstyled.Style = lipgloss.NewStyle()
			return unstyled.View()
		}
		return "◇" // Diamond for plan

	case WorkTabWork:
		// Work tab: use work state
		if tab.WorkProgress != nil {
			state := b.getWorkState(tab.WorkProgress)
			switch state {
			case WorkStateMerged:
				return "✓"
			case WorkStateCompleted:
				return "✓"
			case WorkStateRunning:
				unstyled := b.spinner
				unstyled.Style = lipgloss.NewStyle()
				return unstyled.View()
			case WorkStateFailed:
				return "✗"
			case WorkStateDead:
				return "☠"
			default:
				return "○"
			}
		}
		return "○"
	}
	return "?"
}

// Render renders the tab bar with styled tabs
func (b *WorkTabsBar) Render() string {
	// If we have the new tab model, use it
	if len(b.tabs) > 0 {
		return b.renderTabs()
	}
	// Fall back to legacy rendering
	return b.renderLegacy()
}

// renderTabs renders using the new WorkTab model
func (b *WorkTabsBar) renderTabs() string {
	// Colors
	barBg := lipgloss.Color("235")      // Dark background
	ribbonBg := lipgloss.Color("29")    // Teal for ribbon
	ribbonFg := lipgloss.Color("15")    // White text
	inactiveBg := lipgloss.Color("240") // Gray for inactive
	inactiveFg := lipgloss.Color("255") // Light text
	activeBg := lipgloss.Color("214")   // Orange for active
	activeFg := lipgloss.Color("232")   // Dark text
	planBg := lipgloss.Color("99")      // Purple for plan tabs
	planFg := lipgloss.Color("255")     // White text
	defaultBg := lipgloss.Color("29")   // Teal for default tab
	defaultFg := lipgloss.Color("255")  // White text

	triangle := "\ue0b0" // U+E0B0 - right-pointing solid triangle

	var content string

	// Ribbon
	ribbonText := " Sarge "
	if b.activePanel == PanelWorkTabs {
		ribbonText = "► Sarge ◄"
	}
	ribbonStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ribbonFg).
		Background(ribbonBg)
	content += ribbonStyle.Render(ribbonText)

	spaceStyle := lipgloss.NewStyle().Background(barBg)
	content += spaceStyle.Render(" ")

	for i, tab := range b.tabs {
		isActive := tab.ID == b.activeTabID
		isHovered := tab.ID == b.hoveredTabID

		// Determine tab colors based on type and state
		var tabBg, tabFg color.Color
		if isActive || isHovered {
			tabBg = activeBg
			tabFg = activeFg
		} else {
			switch tab.Type {
			case WorkTabDefault:
				tabBg = defaultBg
				tabFg = defaultFg
			case WorkTabPlan:
				tabBg = planBg
				tabFg = planFg
			default:
				tabBg = inactiveBg
				tabFg = inactiveFg
			}
		}

		// Build tab content
		var tabBuilder string

		// Left triangle
		tabLeftStyle := lipgloss.NewStyle().
			Foreground(barBg).
			Background(tabBg)
		tabBuilder += tabLeftStyle.Render(triangle)

		// Icon
		icon := b.getTabIcon(tab)

		// Label
		name := tab.Label
		name = ansi.Truncate(name, 20, "…")

		// Sub-session indicator for work tabs
		subIndicator := ""
		if tab.Type == WorkTabWork && tab.ActiveSessionID != "" {
			// Show short label if not the default
			if strings.Contains(tab.ActiveSessionID, "task-") {
				suffix := tab.ActiveSessionID[strings.LastIndex(tab.ActiveSessionID, "."):]
				subIndicator = " [t" + suffix + "]"
			} else if strings.Contains(tab.ActiveSessionID, "console-") {
				subIndicator = " [sh]"
			} else if strings.Contains(tab.ActiveSessionID, "plan-") {
				subIndicator = " [pl]"
			}
		}

		// Zoom indicator
		zoomIndicator := ""
		if tab.SessionMaximized {
			zoomIndicator = " ◻"
		}

		tabContent := fmt.Sprintf(" %s %s%s%s", icon, name, subIndicator, zoomIndicator)
		tabStyle := lipgloss.NewStyle().
			Foreground(tabFg).
			Background(tabBg)
		tabBuilder += tabStyle.Render(tabContent)

		// Warning badges for work tabs
		if tab.Type == WorkTabWork && tab.WorkProgress != nil {
			wp := tab.WorkProgress
			if wp.FeedbackCount > 0 || wp.UnassignedBeanCount > 0 {
				badgeStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("214")).
					Background(tabBg)
				tabBuilder += badgeStyle.Render(" \uf071")
			}
			if wp.HasUnseenPRChanges {
				badgeStyle := lipgloss.NewStyle().
					Foreground(lipgloss.Color("81")).
					Background(tabBg)
				tabBuilder += badgeStyle.Render(" ●")
			}
		}

		// Trailing space
		tabBuilder += tabStyle.Render(" ")

		// Right chevron
		tabRightStyle := lipgloss.NewStyle().
			Foreground(tabBg).
			Background(barBg)
		tabBuilder += tabRightStyle.Render(triangle)

		content += zone.Mark(b.zonePrefix+tab.ID, tabBuilder)

		if i < len(b.tabs)-1 {
			content += spaceStyle.Render(" ")
		}
	}

	barStyle := lipgloss.NewStyle().
		Background(barBg).
		Width(b.width)

	return barStyle.Render(content)
}

// renderLegacy renders using the old WorkProgress-based model (backward compat)
func (b *WorkTabsBar) renderLegacy() string {
	// Colors
	barBg := lipgloss.Color("235")      // Dark background
	ribbonBg := lipgloss.Color("29")    // Teal for ribbon
	ribbonFg := lipgloss.Color("15")    // White text
	inactiveBg := lipgloss.Color("240") // Gray for inactive
	inactiveFg := lipgloss.Color("255") // Light text
	activeBg := lipgloss.Color("214")   // Orange for active
	activeFg := lipgloss.Color("232")   // Dark text

	// Uses right-pointing triangle on both sides
	triangle := "\ue0b0" // U+E0B0 - right-pointing solid triangle

	var content string

	// Ribbon as simple box (no triangles)
	// Show focus indicator when work tabs panel is active
	ribbonText := " Sarge "
	if b.activePanel == PanelWorkTabs {
		ribbonText = "► Sarge ◄"
	}
	ribbonStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ribbonFg).
		Background(ribbonBg)

	content += ribbonStyle.Render(ribbonText)

	// Space before tabs
	spaceStyle := lipgloss.NewStyle().Background(barBg)
	content += spaceStyle.Render(" ")

	for i, work := range b.workTiles {
		if work == nil {
			continue
		}

		isActive := work.Work.ID == b.focusedWorkID
		isHovered := work.Work.ID == b.hoveredTabID
		workState := b.getWorkState(work)

		// Determine tab colors
		var tabBg, tabFg color.Color
		if isActive || isHovered {
			tabBg = activeBg
			tabFg = activeFg
		} else {
			tabBg = inactiveBg
			tabFg = inactiveFg
		}

		// Build the entire tab content
		var tabBuilder string

		// Left triangle for tab: dark arrow on tab background
		tabLeftStyle := lipgloss.NewStyle().
			Foreground(barBg).
			Background(tabBg)
		tabBuilder += tabLeftStyle.Render(triangle)

		// Status icon
		var icon string
		switch workState {
		case WorkStateMerged:
			icon = "✓" // Checkmark for merged PRs
		case WorkStateCompleted:
			icon = "✓"
		case WorkStateRunning:
			// Get raw spinner frame by removing style - View() with styling adds
			// ANSI reset codes that break the background color of the containing tab
			unstyled := b.spinner
			unstyled.Style = lipgloss.NewStyle()
			icon = unstyled.View()
		case WorkStateFailed:
			icon = "✗"
		case WorkStateDead:
			icon = "☠"
		default:
			icon = "○"
		}

		// Work name
		name := work.Work.ID
		if work.Work.Name != "" {
			name = work.Work.Name
		}
		name = ansi.Truncate(name, 20, "…")

		// Tab content with optional unseen badge
		tabContent := fmt.Sprintf(" %s %s", icon, name)
		tabStyle := lipgloss.NewStyle().
			Foreground(tabFg).
			Background(tabBg)
		tabBuilder += tabStyle.Render(tabContent)

		// Add pending work indicator (orange warning for feedback or unassigned beans)
		if work.FeedbackCount > 0 || work.UnassignedBeanCount > 0 {
			badgeStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")). // Orange for pending work
				Background(tabBg)
			tabBuilder += badgeStyle.Render(" \uf071") // nf-fa-exclamation_triangle
		}

		// Add unseen PR changes indicator (colored dot)
		if work.HasUnseenPRChanges {
			badgeStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")). // Cyan dot for new changes
				Background(tabBg)
			tabBuilder += badgeStyle.Render(" ●")
		}

		// Trailing space
		tabBuilder += tabStyle.Render(" ")

		// Right chevron for tab
		tabRightStyle := lipgloss.NewStyle().
			Foreground(tabBg).
			Background(barBg)
		tabBuilder += tabRightStyle.Render(triangle)

		// Mark the entire tab with a zone for click/hover detection
		content += zone.Mark(b.zonePrefix+work.Work.ID, tabBuilder)

		// Space between tabs (except last)
		if i < len(b.workTiles)-1 {
			content += spaceStyle.Render(" ")
		}
	}

	// Wrap in bar background
	barStyle := lipgloss.NewStyle().
		Background(barBg).
		Width(b.width)

	return barStyle.Render(content)
}

// DetectHoveredTab returns the work ID of the tab under the mouse using bubblezone
func (b *WorkTabsBar) DetectHoveredTab(msg tea.MouseMsg) string {
	// Check new tabs first
	for _, tab := range b.tabs {
		if zone.Get(b.zonePrefix + tab.ID).InBounds(msg) {
			return tab.ID
		}
	}
	// Fall back to legacy work tiles
	for _, work := range b.workTiles {
		if work == nil {
			continue
		}
		if zone.Get(b.zonePrefix + work.Work.ID).InBounds(msg) {
			return work.Work.ID
		}
	}
	return ""
}

// HandleClick handles a mouse click and returns the clicked tab ID (if any)
func (b *WorkTabsBar) HandleClick(msg tea.MouseMsg) string {
	return b.DetectHoveredTab(msg)
}
