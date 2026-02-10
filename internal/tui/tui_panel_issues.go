package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

// IssuesPanel renders the issues list with filtering, tree structure, and selection.
type IssuesPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Data (set by coordinator)
	beadItems      []beadItem
	cursor         int
	filters        beadFilters
	expanded       bool
	selectedBeads  map[string]bool
	activeSessions map[string]bool
	newBeads       map[string]time.Time
	hoveredIssue   int

	// Work context
	focusedWorkID string

	// Zone prefix for unique zone IDs
	zonePrefix string
}

// NewIssuesPanel creates a new IssuesPanel
func NewIssuesPanel() *IssuesPanel {
	return &IssuesPanel{
		width:          40,
		height:         20,
		selectedBeads:  make(map[string]bool),
		activeSessions: make(map[string]bool),
		newBeads:       make(map[string]time.Time),
		hoveredIssue:   -1,
		zonePrefix:     zone.NewPrefix(),
	}
}

// SetSize updates the panel dimensions
func (p *IssuesPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *IssuesPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *IssuesPanel) IsFocused() bool {
	return p.focused
}

// SetData updates the panel's data
func (p *IssuesPanel) SetData(
	beadItems []beadItem,
	cursor int,
	filters beadFilters,
	expanded bool,
	selectedBeads map[string]bool,
	activeSessions map[string]bool,
	newBeads map[string]time.Time,
) {
	p.beadItems = beadItems
	p.cursor = cursor
	p.filters = filters
	p.expanded = expanded
	p.selectedBeads = selectedBeads
	p.activeSessions = activeSessions
	p.newBeads = newBeads
}

// SetWorkContext updates work-related display state
func (p *IssuesPanel) SetWorkContext(focusedWorkID string) {
	p.focusedWorkID = focusedWorkID
}

// SetHoveredIssue updates which issue is hovered
func (p *IssuesPanel) SetHoveredIssue(index int) {
	p.hoveredIssue = index
}

// GetHoveredIssue returns the currently hovered issue index
func (p *IssuesPanel) GetHoveredIssue() int {
	return p.hoveredIssue
}

// Render returns the issues panel content (without border/panel styling)
func (p *IssuesPanel) Render(visibleLines int) string {
	var filterInfo string

	// When task or children filter is active, show simplified filter info
	// (status filter is not applied in these modes)
	if p.filters.task != "" {
		filterInfo = fmt.Sprintf("[task:%s]", p.filters.task)
		if p.filters.searchText != "" {
			filterInfo += fmt.Sprintf(" | Search: %s", p.filters.searchText)
		}
	} else if p.filters.children != "" {
		filterInfo = fmt.Sprintf("[children:%s]", p.filters.children)
		if p.filters.searchText != "" {
			filterInfo += fmt.Sprintf(" | Search: %s", p.filters.searchText)
		}
	} else {
		// Normal filter display
		filterInfo = fmt.Sprintf("Filter: %s | Sort: %s", p.filters.status, p.filters.sortBy)
		if p.filters.searchText != "" {
			filterInfo += fmt.Sprintf(" | Search: %s", p.filters.searchText)
		}
		if p.filters.label != "" {
			filterInfo += fmt.Sprintf(" | Label: %s", p.filters.label)
		}
	}

	var content strings.Builder
	content.WriteString(tuiDimStyle().Render(filterInfo))
	content.WriteString("\n")

	if len(p.beadItems) == 0 {
		content.WriteString(tuiDimStyle().Render("No issues found"))
	} else {
		visibleItems := max(visibleLines-1, 1) // -1 for filter line

		start := 0
		if p.cursor >= visibleItems {
			start = p.cursor - visibleItems + 1
		}
		end := min(start+visibleItems, len(p.beadItems))

		for i := start; i < end; i++ {
			// Mark each issue line with a zone for click/hover detection
			line := p.renderBeadLine(i, p.beadItems[i])
			content.WriteString(zone.Mark(p.zonePrefix+p.beadItems[i].ID, line))
			if i < end-1 {
				content.WriteString("\n")
			}
		}
	}

	return content.String()
}

// RenderWithPanel returns the issues panel with border styling
func (p *IssuesPanel) RenderWithPanel(contentHeight int) string {
	issuesContentLines := contentHeight - 3 // -3 for border (2) + title (1)
	issuesContent := p.Render(issuesContentLines)

	// Ensure content is exactly the right number of lines to prevent layout overflow
	issuesContent = padOrTruncateLinesIssues(issuesContent, issuesContentLines)

	panelStyle := tuiPanelStyle().Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(CurrentTheme().Accent)
	}

	result := panelStyle.Render(tuiTitleStyle().Render("Issues") + "\n" + issuesContent)

	// If the result is taller than expected (due to lipgloss wrapping), fix it
	// by removing extra lines from the INNER content while preserving borders and title
	if lipgloss.Height(result) > contentHeight {
		lines := strings.Split(result, "\n")
		extraLines := len(lines) - contentHeight
		// Need at least 4 lines: top border, title, 1+ content, bottom border
		if extraLines > 0 && len(lines) > 3 {
			// Keep first line (top border), second line (title), and last line (bottom border)
			// Remove extra lines from content area only
			topBorder := lines[0]
			titleLine := lines[1]
			bottomBorder := lines[len(lines)-1]
			// Content is from lines[2] to lines[len-2]
			contentLines := lines[2 : len(lines)-1]
			// Calculate how many content lines we can keep
			keepContentLines := len(contentLines) - extraLines
			if keepContentLines < 1 {
				keepContentLines = 1 // Always show at least 1 content line
			}
			// Truncate content from the end
			if keepContentLines < len(contentLines) {
				contentLines = contentLines[:keepContentLines]
			}
			lines = []string{topBorder, titleLine}
			lines = append(lines, contentLines...)
			lines = append(lines, bottomBorder)
			result = strings.Join(lines, "\n")
		}
	}

	return result
}

// padOrTruncateLinesIssues ensures the content has exactly targetLines lines
func padOrTruncateLinesIssues(content string, targetLines int) string {
	if targetLines < 1 {
		targetLines = 1
	}

	lines := strings.Split(content, "\n")
	// Remove trailing empty line if present (from trailing \n)
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) > targetLines {
		// Truncate
		lines = lines[:targetLines]
	} else if len(lines) < targetLines {
		// Pad with empty lines
		for len(lines) < targetLines {
			lines = append(lines, "")
		}
	}

	return strings.Join(lines, "\n")
}

// renderBeadLine renders a single bead line
func (p *IssuesPanel) renderBeadLine(i int, bead beadItem) string {
	icon := statusIcon(bead.Status)

	// Selection indicator for multi-select
	var selectionIndicator string
	if p.selectedBeads[bead.ID] {
		selectionIndicator = tuiSelectedCheckStyle().Render("●") + " "
	}

	// Session indicator - compact "P" (processing) shown after status icon
	var sessionIndicator string
	if p.activeSessions[bead.ID] {
		sessionIndicator = tuiSuccessStyle().Render("P")
	}

	// Work assignment indicator
	var workIndicator string
	if bead.assignedWorkID != "" {
		workIndicator = tuiDimStyle().Render("["+bead.assignedWorkID+"]") + " "
	}

	// Tree indentation with connector lines (styled dim)
	var treePrefix string
	if bead.treeDepth > 0 && bead.treePrefixPattern != "" {
		treePrefix = issueTreeStyle().Render(bead.treePrefixPattern)
	}

	// Styled issue ID
	styledID := issueIDStyle().Render(bead.ID)

	// Short type indicator with color
	var styledType string
	switch bead.Type {
	case "task":
		styledType = typeTaskStyle().Render("T")
	case "bug":
		styledType = typeBugStyle().Render("B")
	case "feature":
		styledType = typeFeatureStyle().Render("F")
	case "epic":
		styledType = typeEpicStyle().Render("E")
	case "chore":
		styledType = typeChoreStyle().Render("C")
	case "merge-request":
		styledType = typeDefaultStyle().Render("M")
	case "molecule":
		styledType = typeDefaultStyle().Render("m")
	case "gate":
		styledType = typeDefaultStyle().Render("G")
	case "agent":
		styledType = typeDefaultStyle().Render("A")
	case "role":
		styledType = typeDefaultStyle().Render("R")
	case "rig":
		styledType = typeDefaultStyle().Render("r")
	case "convoy":
		styledType = typeDefaultStyle().Render("c")
	case "event":
		styledType = typeDefaultStyle().Render("v")
	default:
		styledType = typeDefaultStyle().Render("?")
	}

	// Calculate available width and truncate title if needed
	availableWidth := p.width - 4 // Account for panel padding/borders

	// Calculate prefix length for normal display
	var prefixLen int
	if p.expanded {
		prefixLen = 3 + len(bead.ID) + 1 + 3 + len(bead.Type) + 3 // icon + ID + space + [P# type] + spaces
	} else {
		prefixLen = 3 + len(bead.ID) + 3 // icon + ID + type letter + spaces
	}
	if bead.assignedWorkID != "" {
		prefixLen += len(bead.assignedWorkID) + 3 // [work-id] + space
	}
	if bead.treeDepth > 0 {
		prefixLen += len(bead.treePrefixPattern)
	}

	// Truncate title to fit on one line
	title := bead.Title
	maxTitleLen := availableWidth - prefixLen
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title = ansi.Truncate(title, maxTitleLen, "...")

	// Build styled line for normal display
	var line string
	if p.expanded {
		line = fmt.Sprintf("%s%s%s%s %s [P%d %s] %s%s", selectionIndicator, treePrefix, workIndicator, icon, styledID, bead.Priority, bead.Type, sessionIndicator, title)
	} else {
		line = fmt.Sprintf("%s%s%s%s %s %s%s %s", selectionIndicator, treePrefix, workIndicator, icon, styledID, styledType, sessionIndicator, title)
	}

	// For selected/hovered lines, build plain text version to avoid ANSI code conflicts
	if i == p.cursor || i == p.hoveredIssue {
		// Get type letter for compact display
		var typeLetter string
		switch bead.Type {
		case "task":
			typeLetter = "T"
		case "bug":
			typeLetter = "B"
		case "feature":
			typeLetter = "F"
		case "epic":
			typeLetter = "E"
		case "chore":
			typeLetter = "C"
		default:
			typeLetter = "?"
		}

		// Build selection indicator (plain text)
		var plainSelectionIndicator string
		if p.selectedBeads[bead.ID] {
			plainSelectionIndicator = "● "
		}

		// Build session indicator (plain text)
		var plainSessionIndicator string
		if p.activeSessions[bead.ID] {
			plainSessionIndicator = "P"
		}

		// Build work indicator (plain text)
		var plainWorkIndicator string
		if bead.assignedWorkID != "" {
			plainWorkIndicator = "[" + bead.assignedWorkID + "] "
		}

		// Build tree prefix (plain text, no styling)
		var plainTreePrefix string
		if bead.treeDepth > 0 && bead.treePrefixPattern != "" {
			plainTreePrefix = bead.treePrefixPattern
		}

		// Build plain text line without any styling
		var plainLine string
		if p.expanded {
			plainLine = fmt.Sprintf("%s%s%s%s %s [P%d %s] %s%s", plainSelectionIndicator, plainTreePrefix, plainWorkIndicator, icon, bead.ID, bead.Priority, bead.Type, plainSessionIndicator, title)
		} else {
			plainLine = fmt.Sprintf("%s%s%s%s %s %s%s %s", plainSelectionIndicator, plainTreePrefix, plainWorkIndicator, icon, bead.ID, typeLetter, plainSessionIndicator, title)
		}

		// Pad to fill width
		visWidth := lipgloss.Width(plainLine)
		if visWidth < availableWidth {
			plainLine += strings.Repeat(" ", availableWidth-visWidth)
		}

		if i == p.cursor {
			// Use yellow background for newly created beads
			if _, isNew := p.newBeads[bead.ID]; isNew {
				newSelectedStyle := lipgloss.NewStyle().
					Bold(true).
					Foreground(CurrentTheme().Black).
					Background(CurrentTheme().NewBeadSelectedBg)
				return newSelectedStyle.Render(plainLine)
			}
			return tuiSelectedStyle().Render(plainLine)
		}

		// Hover style
		if _, isNew := p.newBeads[bead.ID]; isNew {
			newHoverStyle := lipgloss.NewStyle().
				Foreground(CurrentTheme().Black).
				Background(CurrentTheme().NewBeadHoverBg).
				Bold(true)
			return newHoverStyle.Render(plainLine)
		}
		hoverStyle := lipgloss.NewStyle().
			Foreground(CurrentTheme().Text).
			Background(CurrentTheme().HoverBg).
			Bold(true)
		return hoverStyle.Render(plainLine)
	}

	// Style closed parent beads with dim style
	if bead.isClosedParent {
		return tuiDimStyle().Render(line)
	}

	// Style new beads - apply yellow only to the title
	if _, isNew := p.newBeads[bead.ID]; isNew {
		yellowTitle := tuiNewBeadStyle().Render(title)

		var newLine string
		if p.expanded {
			newLine = fmt.Sprintf("%s%s%s%s %s [P%d %s] %s%s", selectionIndicator, treePrefix, workIndicator, icon, styledID, bead.Priority, bead.Type, sessionIndicator, yellowTitle)
		} else {
			newLine = fmt.Sprintf("%s%s%s%s %s %s%s %s", selectionIndicator, treePrefix, workIndicator, icon, styledID, styledType, sessionIndicator, yellowTitle)
		}

		return newLine
	}

	return line
}

// DetectHoveredIssue determines which issue is at the mouse position using bubblezone
// Returns the absolute index in beadItems, or -1 if not over an issue
func (p *IssuesPanel) DetectHoveredIssue(msg tea.MouseMsg) int {
	for i, bead := range p.beadItems {
		if zone.Get(p.zonePrefix + bead.ID).InBounds(msg) {
			return i
		}
	}
	return -1
}
