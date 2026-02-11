package tui

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// WorkTabsBar renders a simple 1-line header bar ("Sarge v0.x.x").
// Work tiles data is retained for breadcrumb rendering in the view tabs gap.
type WorkTabsBar struct {
	// Dimensions
	width int

	// Data
	workTiles          []*progress.WorkProgress
	focusedWorkID      string
	orchestratorHealth map[string]bool // workID -> orchestrator alive

	// Version string
	version string

	// Spinner for running works (used by breadcrumbs)
	spinner spinner.Model
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

// SetOrchestratorHealth sets the orchestrator health for a work
func (b *WorkTabsBar) SetOrchestratorHealth(healthMap map[string]bool) {
	b.orchestratorHealth = healthMap
}

// SetVersion sets the version string to display in the header
func (b *WorkTabsBar) SetVersion(v string) {
	b.version = v
}

// UpdateSpinner updates the spinner animation frame
func (b *WorkTabsBar) UpdateSpinner(s spinner.Model) {
	b.spinner = s
}

// GetSpinner returns the spinner model for update handling
func (b *WorkTabsBar) GetSpinner() spinner.Model {
	return b.spinner
}

// GetWorkTiles returns the work tiles for breadcrumb rendering
func (b *WorkTabsBar) GetWorkTiles() []*progress.WorkProgress {
	return b.workTiles
}

// Height returns the height of the header bar (always 1 line)
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

// WorkStateIcon returns the status icon for a work state
func (b *WorkTabsBar) WorkStateIcon(work *progress.WorkProgress) string {
	state := b.getWorkState(work)
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

// Render renders a plain 1-line header: "Sarge v0.x.x" in dim text, no background.
func (b *WorkTabsBar) Render() string {
	text := "Sarge"
	if b.version != "" {
		text += " " + b.version
	}

	return lipgloss.NewStyle().
		Foreground(CurrentTheme().Dim).
		Render(text)
}

// DetectHoveredTab is no longer used (tabs removed). Returns "".
func (b *WorkTabsBar) DetectHoveredTab(msg tea.MouseMsg) string {
	return ""
}

// HandleClick is no longer used (tabs removed). Returns "".
func (b *WorkTabsBar) HandleClick(msg tea.MouseMsg) string {
	return ""
}
