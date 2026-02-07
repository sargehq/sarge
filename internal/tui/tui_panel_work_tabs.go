package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
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
// Styled similar to zellij with seamless color transitions between tabs.
type WorkTabsBar struct {
	// Dimensions
	width int

	// Data
	workTiles          []*progress.WorkProgress
	focusedWorkID      string
	hoveredTabID       string
	orchestratorHealth map[string]bool // workID -> orchestrator alive

	// Panel state
	activePanel Panel // Which panel is currently focused

	// Spinner for running works
	spinner spinner.Model

	// Zone prefix for unique zone IDs
	zonePrefix string
}

// NewWorkTabsBar creates a new WorkTabsBar
func NewWorkTabsBar() *WorkTabsBar {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(CurrentTheme().Accent)

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

// SetWorkTiles updates the work tiles data
func (b *WorkTabsBar) SetWorkTiles(workTiles []*progress.WorkProgress) {
	b.workTiles = workTiles
}

// SetFocusedWorkID sets which work is currently focused
func (b *WorkTabsBar) SetFocusedWorkID(id string) {
	b.focusedWorkID = id
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

// Render renders the tab bar with zellij-like styling
func (b *WorkTabsBar) Render() string {
	// Colors
	barBg := CurrentTheme().SurfaceTabBar
	ribbonBg := CurrentTheme().Ribbon
	ribbonFg := CurrentTheme().RibbonText
	inactiveBg := CurrentTheme().TabInactiveBg
	inactiveFg := CurrentTheme().TabInactiveFg
	activeBg := CurrentTheme().TabActiveBg
	activeFg := CurrentTheme().TabActiveFg

	// Zellij-style: uses right-pointing triangle on both sides
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
		var tabBg, tabFg lipgloss.Color
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

		// Add pending work indicator (orange warning for feedback or unassigned beads)
		if work.FeedbackCount > 0 || work.UnassignedBeadCount > 0 {
			badgeStyle := lipgloss.NewStyle().
				Foreground(CurrentTheme().Warning). // Orange for pending work
				Background(tabBg)
			tabBuilder += badgeStyle.Render(" \uf071") // nf-fa-exclamation_triangle
		}

		// Add unseen PR changes indicator (colored dot)
		if work.HasUnseenPRChanges {
			badgeStyle := lipgloss.NewStyle().
				Foreground(CurrentTheme().Cyan). // Cyan dot for new changes
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

// HandleClick handles a mouse click and returns the clicked work ID (if any)
func (b *WorkTabsBar) HandleClick(msg tea.MouseMsg) string {
	return b.DetectHoveredTab(msg)
}
