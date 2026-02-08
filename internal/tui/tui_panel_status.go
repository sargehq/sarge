package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

// StatusBarContext indicates which panel the status bar should show commands for
type StatusBarContext int

const (
	StatusBarContextIssues StatusBarContext = iota
	StatusBarContextWorkDetail
)

// StatusBar is the status bar panel at the bottom of the TUI.
// It renders command buttons, status messages, and handles hover/click detection.
type StatusBar struct {
	// Dimensions
	width int

	// State
	statusMessage string
	statusIsError bool
	loading       bool
	lastUpdate    time.Time
	spinner       spinner.Model

	// Context determines which commands to show
	context StatusBarContext

	// Mouse state
	hoveredButton string

	// Zone prefix for unique zone IDs
	zonePrefix string

	// Version string
	version string

	// Data providers (set by coordinator)
	getBeadItems            func() []beadItem
	getBeadsCursor          func() int
	getActiveSessions       func() map[string]bool
	getViewMode             func() ViewMode
	getTextInput            func() string
	isFailedTaskSelected    func() bool
	getFocusedWorkID        func() string
}

// NewStatusBar creates a new StatusBar panel
func NewStatusBar() *StatusBar {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(CurrentTheme().Accent)

	return &StatusBar{
		width:      80,
		spinner:    s,
		zonePrefix: zone.NewPrefix(),
	}
}

// SetSize updates the panel dimensions
func (s *StatusBar) SetSize(width int) {
	s.width = width
}

// SetDataProviders sets the functions to get data from the coordinator
func (s *StatusBar) SetDataProviders(
	getBeadItems func() []beadItem,
	getBeadsCursor func() int,
	getActiveSessions func() map[string]bool,
	getViewMode func() ViewMode,
	getTextInput func() string,
) {
	s.getBeadItems = getBeadItems
	s.getBeadsCursor = getBeadsCursor
	s.getActiveSessions = getActiveSessions
	s.getViewMode = getViewMode
	s.getTextInput = getTextInput
}

// SetFailedTaskSelectedProvider sets the provider for checking if a failed task is selected
func (s *StatusBar) SetFailedTaskSelectedProvider(isFailedTaskSelected func() bool) {
	s.isFailedTaskSelected = isFailedTaskSelected
}

// SetFocusedWorkIDProvider sets the provider for the focused work ID
func (s *StatusBar) SetFocusedWorkIDProvider(getFocusedWorkID func() string) {
	s.getFocusedWorkID = getFocusedWorkID
}

// SetStatus updates the status message
func (s *StatusBar) SetStatus(message string, isError bool) {
	// Strip newlines - status bar is single line only
	message = strings.ReplaceAll(message, "\n", " ")
	message = strings.ReplaceAll(message, "\r", "")
	s.statusMessage = strings.TrimSpace(message)
	s.statusIsError = isError
}

// SetLoading updates the loading state
func (s *StatusBar) SetLoading(loading bool) {
	s.loading = loading
}

// SetLastUpdate records when data was last refreshed
func (s *StatusBar) SetLastUpdate(t time.Time) {
	s.lastUpdate = t
}

// SetHoveredButton updates which button is hovered
func (s *StatusBar) SetHoveredButton(button string) {
	s.hoveredButton = button
}

// SetVersion sets the version string to display
func (s *StatusBar) SetVersion(v string) {
	s.version = v
}

// SetContext updates the status bar context (which panel's commands to show)
func (s *StatusBar) SetContext(ctx StatusBarContext) {
	s.context = ctx
}

// GetHoveredButton returns which button is currently hovered
func (s *StatusBar) GetHoveredButton() string {
	return s.hoveredButton
}

// UpdateSpinner updates the spinner animation
func (s *StatusBar) UpdateSpinner(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return cmd
}

// Render returns the status bar content
func (s *StatusBar) Render() string {
	// If in search mode, show vim-style inline search bar
	if s.getViewMode != nil && s.getViewMode() == ViewBeadSearch {
		searchPrompt := "/"
		searchInput := ""
		if s.getTextInput != nil {
			searchInput = s.getTextInput()
		}
		hint := tuiDimStyle().Render("  [Enter]Search  [Esc]Cancel")
		return tuiStatusBarStyle().Width(s.width).Render(searchPrompt + searchInput + hint)
	}

	var commands string
	var commandsPlain string

	switch s.context {
	case StatusBarContextWorkDetail:
		// Work detail commands
		commands, commandsPlain = s.renderWorkDetailCommands()
	default:
		// Issues commands (default)
		commands, commandsPlain = s.renderIssuesCommands()
	}

	// Status on the right
	var status string
	var statusPlain string
	if s.statusMessage != "" {
		statusPlain = s.statusMessage
		if s.statusIsError {
			status = tuiErrorStyle().Render(s.statusMessage)
		} else {
			status = tuiSuccessStyle().Render(s.statusMessage)
		}
	} else if s.loading {
		statusPlain = "Loading..."
		status = s.spinner.View() + " Loading..."
	} else {
		if s.version != "" {
			statusPlain = fmt.Sprintf("%s · Updated: %s", s.version, s.lastUpdate.Format("15:04:05"))
		} else {
			statusPlain = fmt.Sprintf("Updated: %s", s.lastUpdate.Format("15:04:05"))
		}
		status = tuiDimStyle().Render(statusPlain)
	}

	// Calculate available space for status message and truncate if needed
	// Inner content width = s.width - 2 (status bar has Padding(0,1) = 1 on each side)
	// Content = commands + minPadding + status
	minPadding := 2
	innerWidth := s.width - 2
	commandsWidth := ansi.StringWidth(commandsPlain)
	statusWidth := ansi.StringWidth(statusPlain)

	// Available width for status = inner width minus commands and minimum padding
	availableWidth := max(innerWidth-commandsWidth-minPadding, 0)

	// Truncate status if needed
	if statusWidth > availableWidth {
		if availableWidth <= 3 {
			// Not enough room for status, hide it
			status = ""
			statusWidth = 0
		} else {
			// Truncate with ellipsis
			truncatedPlain := ansi.Truncate(statusPlain, availableWidth, "...")
			statusPlain = truncatedPlain
			statusWidth = ansi.StringWidth(statusPlain)
			if s.statusIsError {
				status = tuiErrorStyle().Render(truncatedPlain)
			} else if s.loading {
				status = s.spinner.View() + " Loading..."
			} else if s.statusMessage != "" {
				status = tuiSuccessStyle().Render(truncatedPlain)
			} else {
				status = tuiDimStyle().Render(truncatedPlain)
			}
		}
	}

	// Build bar with commands left, status right
	// Padding fills the remaining space
	padding := max(innerWidth-commandsWidth-statusWidth, minPadding)
	return tuiStatusBarStyle().Width(s.width).Render(commands + strings.Repeat(" ", padding) + status)
}

// renderIssuesCommands returns commands for the issues panel.
// Commands that don't apply to the current state are dimmed.
func (s *StatusBar) renderIssuesCommands() (string, string) {
	dim := tuiDimStyle()

	// Gather state
	var beadItems []beadItem
	hasIssues := false
	hasCursorBead := false
	cursorBeadAssigned := false
	hasWorkSelected := false

	if s.getBeadItems != nil {
		beadItems = s.getBeadItems()
		hasIssues = len(beadItems) > 0
	}
	if s.getBeadsCursor != nil && hasIssues {
		cursor := s.getBeadsCursor()
		if cursor < len(beadItems) {
			hasCursorBead = true
			cursorBeadAssigned = beadItems[cursor].assignedWorkID != ""
		}
	}
	if s.getFocusedWorkID != nil {
		hasWorkSelected = s.getFocusedWorkID() != ""
	}

	// Determine p action text
	pAction := "[p]Plan"
	if hasCursorBead && s.getActiveSessions != nil {
		beadID := beadItems[s.getBeadsCursor()].ID
		if s.getActiveSessions()[beadID] {
			pAction = "[p]Resume"
		}
	}

	type btn struct {
		id      string
		label   string
		active  bool
	}

	buttons := []btn{
		{"n", "[n]New", true},                                          // always available
		{"e", "[e]Edit", hasCursorBead},                                // need a selected issue
		{"a", "[a]Child", hasCursorBead},                               // need a selected issue
		{"x", "[x]Close", hasCursorBead},                               // need a selected issue
		{"d", "[d]Del", hasCursorBead},                                 // need a selected issue
		{"w", "[w]Work", hasCursorBead && !cursorBeadAssigned},         // need unassigned issue
		{"A", "[A]dd", hasCursorBead && hasWorkSelected},               // need issue + work filter
		{"i", "[i]Import", true},                                       // always available
		{"p", pAction, hasCursorBead},                                  // need a selected issue
		{"?", "[?]Help", true},                                         // always available
	}

	var rendered []string
	var plain []string
	for _, b := range buttons {
		if b.active {
			rendered = append(rendered, zone.Mark(s.zonePrefix+b.id, styleButtonWithHover(b.label, s.hoveredButton == b.id)))
		} else {
			rendered = append(rendered, dim.Render(b.label))
		}
		plain = append(plain, b.label)
	}

	return strings.Join(rendered, " "), strings.Join(plain, " ")
}

// renderWorkDetailCommands returns commands for the work detail panel
func (s *StatusBar) renderWorkDetailCommands() (string, string) {
	// Work detail specific commands - wrap each with zone.Mark
	tButton := zone.Mark(s.zonePrefix+"t", styleButtonWithHover("[t]erminal", s.hoveredButton == "t"))
	cButton := zone.Mark(s.zonePrefix+"c", styleButtonWithHover("[c]hat", s.hoveredButton == "c"))
	iButton := zone.Mark(s.zonePrefix+"i", styleButtonWithHover("[i]DE", s.hoveredButton == "i"))
	rButton := zone.Mark(s.zonePrefix+"r", styleButtonWithHover("[r]un", s.hoveredButton == "r"))
	oButton := zone.Mark(s.zonePrefix+"o", styleButtonWithHover("[o]rch", s.hoveredButton == "o"))
	vButton := zone.Mark(s.zonePrefix+"v", styleButtonWithHover("[v]review", s.hoveredButton == "v"))
	pButton := zone.Mark(s.zonePrefix+"p", styleButtonWithHover("[p]r", s.hoveredButton == "p"))
	fButton := zone.Mark(s.zonePrefix+"f", styleButtonWithHover("[f]eedback", s.hoveredButton == "f"))
	dButton := zone.Mark(s.zonePrefix+"d", styleButtonWithHover("[d]estroy", s.hoveredButton == "d"))
	escButton := zone.Mark(s.zonePrefix+"esc", styleButtonWithHover("[Esc]Deselect", s.hoveredButton == "esc"))
	helpButton := zone.Mark(s.zonePrefix+"?", styleButtonWithHover("[?]Help", s.hoveredButton == "?"))

	// Check if a failed task is selected to conditionally show reset button
	showReset := s.isFailedTaskSelected != nil && s.isFailedTaskSelected()

	var commands, commandsPlain string
	if showReset {
		xButton := zone.Mark(s.zonePrefix+"x", styleButtonWithHover("[x]Reset", s.hoveredButton == "x"))
		commands = tButton + " " + cButton + " " + iButton + " " + rButton + " " + oButton + " " + vButton + " " + pButton + " " + fButton + " " + xButton + " " + dButton + " " + escButton + " " + helpButton
		commandsPlain = "[t]erminal [c]hat [i]DE [r]un [o]rch [v]review [p]r [f]eedback [x]Reset [d]estroy [Esc]Deselect [?]Help"
	} else {
		commands = tButton + " " + cButton + " " + iButton + " " + rButton + " " + oButton + " " + vButton + " " + pButton + " " + fButton + " " + dButton + " " + escButton + " " + helpButton
		commandsPlain = "[t]erminal [c]hat [i]DE [r]un [o]rch [v]review [p]r [f]eedback [d]estroy [Esc]Deselect [?]Help"
	}

	return commands, commandsPlain
}

// DetectButton determines which button is at the mouse position using bubblezone
func (s *StatusBar) DetectButton(msg tea.MouseMsg) string {
	switch s.context {
	case StatusBarContextWorkDetail:
		return s.detectWorkDetailButton(msg)
	default:
		return s.detectIssuesButton(msg)
	}
}

// detectIssuesButton detects button clicks for the issues panel using bubblezone
func (s *StatusBar) detectIssuesButton(msg tea.MouseMsg) string {
	buttons := []string{"n", "e", "a", "x", "d", "w", "A", "i", "p", "?"}
	for _, btn := range buttons {
		if zone.Get(s.zonePrefix + btn).InBounds(msg) {
			return btn
		}
	}
	return ""
}

// detectWorkDetailButton detects button clicks for the work detail panel using bubblezone
func (s *StatusBar) detectWorkDetailButton(msg tea.MouseMsg) string {
	buttons := []string{"t", "c", "i", "r", "o", "v", "p", "f", "x", "d", "esc", "?"}
	for _, btn := range buttons {
		if zone.Get(s.zonePrefix + btn).InBounds(msg) {
			return btn
		}
	}
	return ""
}

// ClearStatus clears the status message
func (s *StatusBar) ClearStatus() {
	s.statusMessage = ""
	s.statusIsError = false
}
