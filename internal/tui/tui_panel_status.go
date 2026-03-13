package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

// StatusBarContext indicates which panel the status bar should show commands for
type StatusBarContext int

const (
	StatusBarContextIssues StatusBarContext = iota
	StatusBarContextWorkFocused // Merged: work actions + issue actions
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

	// Data providers (set by coordinator)
	getBeanItems            func() []beanItem
	getBeansCursor          func() int
	getActiveSessions       func() map[string]bool
	getViewMode             func() ViewMode
	getTextInput            func() string
	isFailedTaskSelected    func() bool
	getActivePanel          func() Panel
}

// NewStatusBar creates a new StatusBar panel
func NewStatusBar() *StatusBar {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

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
	getBeanItems func() []beanItem,
	getBeansCursor func() int,
	getActiveSessions func() map[string]bool,
	getViewMode func() ViewMode,
	getTextInput func() string,
) {
	s.getBeanItems = getBeanItems
	s.getBeansCursor = getBeansCursor
	s.getActiveSessions = getActiveSessions
	s.getViewMode = getViewMode
	s.getTextInput = getTextInput
}

// SetFailedTaskSelectedProvider sets the provider for checking if a failed task is selected
func (s *StatusBar) SetFailedTaskSelectedProvider(isFailedTaskSelected func() bool) {
	s.isFailedTaskSelected = isFailedTaskSelected
}

// SetActivePanelProvider sets the provider for checking which panel is active
func (s *StatusBar) SetActivePanelProvider(getActivePanel func() Panel) {
	s.getActivePanel = getActivePanel
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
	if s.getViewMode != nil && s.getViewMode() == ViewBeanSearch {
		searchPrompt := "/"
		searchInput := ""
		if s.getTextInput != nil {
			searchInput = s.getTextInput()
		}
		hint := tuiDimStyle.Render("  [Enter]Search  [Esc]Cancel")
		return tuiStatusBarStyle.Width(s.width).Render(searchPrompt + searchInput + hint)
	}

	var commands string
	var commandsPlain string

	switch s.context {
	case StatusBarContextWorkFocused:
		// Merged commands: work actions + non-conflicting issue actions
		commands, commandsPlain = s.renderWorkFocusedCommands()
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
			status = tuiErrorStyle.Render(s.statusMessage)
		} else {
			status = tuiSuccessStyle.Render(s.statusMessage)
		}
	} else if s.loading {
		statusPlain = "Loading..."
		status = s.spinner.View() + " Loading..."
	} else {
		statusPlain = fmt.Sprintf("Updated: %s", s.lastUpdate.Format("15:04:05"))
		status = tuiDimStyle.Render(statusPlain)
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
				status = tuiErrorStyle.Render(truncatedPlain)
			} else if s.loading {
				status = s.spinner.View() + " Loading..."
			} else if s.statusMessage != "" {
				status = tuiSuccessStyle.Render(truncatedPlain)
			} else {
				status = tuiDimStyle.Render(truncatedPlain)
			}
		}
	}

	// Build bar with commands left, status right
	// Padding fills the remaining space
	padding := max(innerWidth-commandsWidth-statusWidth, minPadding)
	return tuiStatusBarStyle.Width(s.width).Render(commands + strings.Repeat(" ", padding) + status)
}

// renderIssuesCommands returns commands for the issues panel (no work focused)
func (s *StatusBar) renderIssuesCommands() (string, string) {
	// Commands on the left with hover effects - wrap each with zone.Mark
	// All action keys use ctrl+ prefix; pi sessions get complete control (only ESC exits)
	nButton := zone.Mark(s.zonePrefix+"n", styleButtonWithHover("^[n]ew", s.hoveredButton == "n"))
	eButton := zone.Mark(s.zonePrefix+"e", styleButtonWithHover("^[e]dit", s.hoveredButton == "e"))
	aButton := zone.Mark(s.zonePrefix+"a", styleButtonWithHover("^[a]child", s.hoveredButton == "a"))
	xButton := zone.Mark(s.zonePrefix+"x", styleButtonWithHover("^[x]close", s.hoveredButton == "x"))
	dButton := zone.Mark(s.zonePrefix+"d", styleButtonWithHover("^[d]el", s.hoveredButton == "d"))
	wButton := zone.Mark(s.zonePrefix+"w", styleButtonWithHover("^[w]ork", s.hoveredButton == "w"))
	helpButton := zone.Mark(s.zonePrefix+"?", styleButtonWithHover("[?]help", s.hoveredButton == "?"))

	commands := nButton + " " + eButton + " " + aButton + " " + xButton + " " + dButton + " " + wButton + " " + helpButton
	commandsPlain := "^[n]ew ^[e]dit ^[a]child ^[x]close ^[d]el ^[w]ork [?]help"

	return commands, commandsPlain
}

// renderWorkFocusedCommands returns merged commands: work actions + non-conflicting issue actions
func (s *StatusBar) renderWorkFocusedCommands() (string, string) {
	// Work action keys - all use ctrl+ prefix
	tButton := zone.Mark(s.zonePrefix+"t", styleButtonWithHover("^[t]erm", s.hoveredButton == "t"))
	cButton := zone.Mark(s.zonePrefix+"c", styleButtonWithHover("^[c]hat", s.hoveredButton == "c"))
	iButton := zone.Mark(s.zonePrefix+"i", styleButtonWithHover("^[i]DE", s.hoveredButton == "i"))
	rButton := zone.Mark(s.zonePrefix+"r", styleButtonWithHover("^[r]un", s.hoveredButton == "r"))
	oButton := zone.Mark(s.zonePrefix+"o", styleButtonWithHover("^[o]rch", s.hoveredButton == "o"))
	vButton := zone.Mark(s.zonePrefix+"v", styleButtonWithHover("^[v]review", s.hoveredButton == "v"))
	pButton := zone.Mark(s.zonePrefix+"p", styleButtonWithHover("^[p]r", s.hoveredButton == "p"))
	fButton := zone.Mark(s.zonePrefix+"f", styleButtonWithHover("^[f]eedback", s.hoveredButton == "f"))

	// ctrl+d is panel-aware: shows ^[d]estroy when work panel is focused, ^[d]el when issues panel is focused
	dLabel := "^[d]estroy"
	dLabelPlain := "^[d]estroy"
	if s.getActivePanel != nil && s.getActivePanel() == PanelLeft {
		dLabel = "^[d]el"
		dLabelPlain = "^[d]el"
	}
	dButton := zone.Mark(s.zonePrefix+"d", styleButtonWithHover(dLabel, s.hoveredButton == "d"))

	// Separator
	sep := tuiDimStyle.Render("|")

	// Non-conflicting issue actions
	nButton := zone.Mark(s.zonePrefix+"n", styleButtonWithHover("^[n]ew", s.hoveredButton == "n"))
	eButton := zone.Mark(s.zonePrefix+"e", styleButtonWithHover("^[e]dit", s.hoveredButton == "e"))
	aButton := zone.Mark(s.zonePrefix+"a", styleButtonWithHover("^[a]child", s.hoveredButton == "a"))
	xButton := zone.Mark(s.zonePrefix+"x", styleButtonWithHover("^[x]close", s.hoveredButton == "x"))
	wButton := zone.Mark(s.zonePrefix+"w", styleButtonWithHover("^[w]ork", s.hoveredButton == "w"))
	helpButton := zone.Mark(s.zonePrefix+"?", styleButtonWithHover("[?]help", s.hoveredButton == "?"))

	commands := tButton + " " + cButton + " " + iButton + " " + rButton + " " + oButton + " " + vButton + " " + pButton + " " + fButton + " " + dButton + " " + sep + " " + nButton + " " + eButton + " " + aButton + " " + xButton + " " + wButton + " " + helpButton
	commandsPlain := "^[t]erm ^[c]hat ^[i]DE ^[r]un ^[o]rch ^[v]review ^[p]r ^[f]eedback " + dLabelPlain + " | ^[n]ew ^[e]dit ^[a]child ^[x]close ^[w]ork [?]help"

	return commands, commandsPlain
}

// DetectButton determines which button is at the mouse position using bubblezone
func (s *StatusBar) DetectButton(msg tea.MouseMsg) string {
	switch s.context {
	case StatusBarContextWorkFocused:
		return s.detectWorkFocusedButton(msg)
	default:
		return s.detectIssuesButton(msg)
	}
}

// detectIssuesButton detects button clicks for the issues panel using bubblezone
func (s *StatusBar) detectIssuesButton(msg tea.MouseMsg) string {
	buttons := []string{"n", "e", "a", "x", "d", "w", "O", "C", "R", "?"}
	for _, btn := range buttons {
		if zone.Get(s.zonePrefix + btn).InBounds(msg) {
			return btn
		}
	}
	return ""
}

// detectWorkFocusedButton detects button clicks for the merged work+issues bar
func (s *StatusBar) detectWorkFocusedButton(msg tea.MouseMsg) string {
	buttons := []string{"t", "c", "i", "r", "o", "v", "p", "f", "d", "n", "e", "a", "x", "w", "?"}
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
