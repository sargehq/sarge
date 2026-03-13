package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
)

// IssuesPanel renders the issues list with filtering, tree structure, and selection.
type IssuesPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Data (set by coordinator)
	beanItems      []beanItem
	cursor         int
	filters        beanFilters
	expanded       bool
	selectedBeans  map[string]bool
	activeSessions map[string]bool
	newBeans       map[string]time.Time
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
		selectedBeans:  make(map[string]bool),
		activeSessions: make(map[string]bool),
		newBeans:       make(map[string]time.Time),
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
	beanItems []beanItem,
	cursor int,
	filters beanFilters,
	expanded bool,
	selectedBeans map[string]bool,
	activeSessions map[string]bool,
	newBeans map[string]time.Time,
) {
	p.beanItems = beanItems
	p.cursor = cursor
	p.filters = filters
	p.expanded = expanded
	p.selectedBeans = selectedBeans
	p.activeSessions = activeSessions
	p.newBeans = newBeans
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
	content.WriteString(tuiDimStyle.Render(filterInfo))
	content.WriteString("\n")

	if len(p.beanItems) == 0 {
		content.WriteString(tuiDimStyle.Render("No issues found"))
	} else {
		visibleItems := max(visibleLines-1, 1) // -1 for filter line

		start := 0
		if p.cursor >= visibleItems {
			start = p.cursor - visibleItems + 1
		}
		end := min(start+visibleItems, len(p.beanItems))

		for i := start; i < end; i++ {
			// Mark each issue line with a zone for click/hover detection
			line := p.renderBeanLine(i, p.beanItems[i])
			content.WriteString(zone.Mark(p.zonePrefix+p.beanItems[i].ID, line))
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

	panelStyle := tuiPanelStyle.Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(lipgloss.Color("214"))
	}

	return panelStyle.Render(tuiTitleStyle.Render("Issues") + "\n" + issuesContent)
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

// renderBeanLine renders a single bean line
func (p *IssuesPanel) renderBeanLine(i int, bean beanItem) string {
	icon := statusIcon(bean.Status)

	// Selection indicator for multi-select
	var selectionIndicator string
	if p.selectedBeans[bean.ID] {
		selectionIndicator = tuiSelectedCheckStyle.Render("●") + " "
	}

	// Session indicator - compact "P" (processing) shown after status icon
	var sessionIndicator string
	if p.activeSessions[bean.ID] {
		sessionIndicator = tuiSuccessStyle.Render("P")
	}

	// Work assignment indicator
	var workIndicator string
	if bean.assignedWorkID != "" {
		workIndicator = tuiDimStyle.Render("["+bean.assignedWorkID+"]") + " "
	}

	// Tree indentation with connector lines (styled dim)
	var treePrefix string
	if bean.treeDepth > 0 && bean.treePrefixPattern != "" {
		treePrefix = issueTreeStyle.Render(bean.treePrefixPattern)
	}

	// Styled issue ID
	styledID := issueIDStyle.Render(bean.ID)

	// Short type indicator with color
	var styledType string
	switch bean.Type {
	case "task":
		styledType = typeTaskStyle.Render("T")
	case "bug":
		styledType = typeBugStyle.Render("B")
	case "feature":
		styledType = typeFeatureStyle.Render("F")
	case "epic":
		styledType = typeEpicStyle.Render("E")
	case "chore":
		styledType = typeChoreStyle.Render("C")
	case "merge-request":
		styledType = typeDefaultStyle.Render("M")
	case "molecule":
		styledType = typeDefaultStyle.Render("m")
	case "gate":
		styledType = typeDefaultStyle.Render("G")
	case "agent":
		styledType = typeDefaultStyle.Render("A")
	case "role":
		styledType = typeDefaultStyle.Render("R")
	case "rig":
		styledType = typeDefaultStyle.Render("r")
	case "convoy":
		styledType = typeDefaultStyle.Render("c")
	case "event":
		styledType = typeDefaultStyle.Render("v")
	default:
		styledType = typeDefaultStyle.Render("?")
	}

	// Calculate available width and truncate title if needed
	availableWidth := p.width - 4 // Account for panel padding/borders

	// Calculate prefix length for normal display
	var prefixLen int
	if p.expanded {
		prefixLen = 3 + len(bean.ID) + 1 + 3 + len(bean.Type) + 3 // icon + ID + space + [P# type] + spaces
	} else {
		prefixLen = 3 + len(bean.ID) + 3 // icon + ID + type letter + spaces
	}
	if bean.assignedWorkID != "" {
		prefixLen += len(bean.assignedWorkID) + 3 // [work-id] + space
	}
	if bean.treeDepth > 0 {
		prefixLen += len(bean.treePrefixPattern)
	}

	// Truncate title to fit on one line
	title := bean.Title
	maxTitleLen := availableWidth - prefixLen
	if maxTitleLen < 10 {
		maxTitleLen = 10
	}
	title = ansi.Truncate(title, maxTitleLen, "...")

	// Build styled line for normal display
	var line string
	if p.expanded {
		line = fmt.Sprintf("%s%s%s%s %s [P:%s %s] %s%s", selectionIndicator, treePrefix, workIndicator, icon, styledID, bean.Priority, bean.Type, sessionIndicator, title)
	} else {
		line = fmt.Sprintf("%s%s%s%s %s %s%s %s", selectionIndicator, treePrefix, workIndicator, icon, styledID, styledType, sessionIndicator, title)
	}

	// For selected/hovered lines, build plain text version to avoid ANSI code conflicts
	if i == p.cursor || i == p.hoveredIssue {
		// Get type letter for compact display
		var typeLetter string
		switch bean.Type {
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
		if p.selectedBeans[bean.ID] {
			plainSelectionIndicator = "● "
		}

		// Build session indicator (plain text)
		var plainSessionIndicator string
		if p.activeSessions[bean.ID] {
			plainSessionIndicator = "P"
		}

		// Build work indicator (plain text)
		var plainWorkIndicator string
		if bean.assignedWorkID != "" {
			plainWorkIndicator = "[" + bean.assignedWorkID + "] "
		}

		// Build tree prefix (plain text, no styling)
		var plainTreePrefix string
		if bean.treeDepth > 0 && bean.treePrefixPattern != "" {
			plainTreePrefix = bean.treePrefixPattern
		}

		// Build plain text line without any styling
		var plainLine string
		if p.expanded {
			plainLine = fmt.Sprintf("%s%s%s%s %s [P:%s %s] %s%s", plainSelectionIndicator, plainTreePrefix, plainWorkIndicator, icon, bean.ID, bean.Priority, bean.Type, plainSessionIndicator, title)
		} else {
			plainLine = fmt.Sprintf("%s%s%s%s %s %s%s %s", plainSelectionIndicator, plainTreePrefix, plainWorkIndicator, icon, bean.ID, typeLetter, plainSessionIndicator, title)
		}

		// Pad to fill width
		visWidth := lipgloss.Width(plainLine)
		if visWidth < availableWidth {
			plainLine += strings.Repeat(" ", availableWidth-visWidth)
		}

		if i == p.cursor {
			// Use yellow background for newly created beans
			if _, isNew := p.newBeans[bean.ID]; isNew {
				newSelectedStyle := lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("0")).
					Background(lipgloss.Color("226"))
				return newSelectedStyle.Render(plainLine)
			}
			return tuiSelectedStyle.Render(plainLine)
		}

		// Hover style
		if _, isNew := p.newBeans[bean.ID]; isNew {
			newHoverStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("228")).
				Bold(true)
			return newHoverStyle.Render(plainLine)
		}
		hoverStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("240")).
			Bold(true)
		return hoverStyle.Render(plainLine)
	}

	// Style closed parent beans with dim style
	if bean.isClosedParent {
		return tuiDimStyle.Render(line)
	}

	// Style new beans - apply yellow only to the title
	if _, isNew := p.newBeans[bean.ID]; isNew {
		yellowTitle := tuiNewBeanStyle.Render(title)

		var newLine string
		if p.expanded {
			newLine = fmt.Sprintf("%s%s%s%s %s [P:%s %s] %s%s", selectionIndicator, treePrefix, workIndicator, icon, styledID, bean.Priority, bean.Type, sessionIndicator, yellowTitle)
		} else {
			newLine = fmt.Sprintf("%s%s%s%s %s %s%s %s", selectionIndicator, treePrefix, workIndicator, icon, styledID, styledType, sessionIndicator, yellowTitle)
		}

		return newLine
	}

	return line
}

// DetectHoveredIssue determines which issue is at the mouse position using bubblezone
// Returns the absolute index in beanItems, or -1 if not over an issue
func (p *IssuesPanel) DetectHoveredIssue(msg tea.MouseMsg) int {
	for i, bean := range p.beanItems {
		if zone.Get(p.zonePrefix + bean.ID).InBounds(msg) {
			return i
		}
	}
	return -1
}
