package tui

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
)

// TUI-specific styles - theme-aware functions that read from CurrentTheme()

func tuiTitleStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(CurrentTheme().Title)
}

func tuiHotkeyStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(CurrentTheme().Accent)
}

func tuiPanelStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CurrentTheme().Border).
		Padding(0, 1)
}

func tuiSelectedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).
		Foreground(CurrentTheme().Text).
		Background(CurrentTheme().SurfaceSelected)
}

func tuiSelectedCheckStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Success)
}

func tuiLabelStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Label)
}

func tuiValueStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Text)
}

func tuiDimStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Dim)
}

func tuiErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Error)
}

func tuiSuccessStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Success)
}

func tuiStatusBarStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(CurrentTheme().SurfaceBar).Padding(0, 1)
}

func tuiDialogStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CurrentTheme().DialogBorder).
		Padding(1, 2).
		Background(CurrentTheme().SurfaceDialog)
}

func tuiHelpStyle() lipgloss.Style {
	return lipgloss.NewStyle().Padding(2, 4).Background(CurrentTheme().SurfaceDialog)
}

func statusPendingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().StatusPending)
}

func statusProcessingStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().StatusProcessing).Bold(true)
}

func statusCompletedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().StatusCompleted).Bold(true)
}

func statusFailedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().StatusFailed).Bold(true)
}

func issueIDStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Accent)
}

func issueTreeStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().Dim)
}

func typeTaskStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeTask)
}

func typeBugStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeBug)
}

func typeFeatureStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeFeature)
}

func typeEpicStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeEpic).Bold(true)
}

func typeChoreStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeDefault)
}

func typeDefaultStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().TypeDefault)
}

func tuiNewBeadStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(CurrentTheme().NewBead).Bold(true)
}

// ansiRegexp matches ANSI SGR escape sequences (colors, bold, etc.)
var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// dimContent strips all ANSI color/style sequences from rendered content
// and re-renders everything in the Dim color, creating a "washed out" effect
// for unfocused panels.
func dimContent(s string) string {
	plain := ansiRegexp.ReplaceAllString(s, "")
	return lipgloss.NewStyle().Foreground(CurrentTheme().Dim).Render(plain)
}

// Panel represents which panel is currently focused
type Panel int

const (
	PanelLeft        Panel = iota // Left panel (issues)
	PanelMiddle                   // Middle panel at current depth (used by tui.go)
	PanelRight                    // Right panel (details/forms)
)

// ViewMode represents the current view mode
type ViewMode int

const (
	ViewNormal ViewMode = iota
	ViewCreateWork
	ViewCreateBead
	ViewCreateBeadInline // Create issue inline in description area
	ViewCreateEpic
	ViewAddChildBead // Add child issue to selected issue
	ViewEditBead     // Edit selected issue
	ViewDestroyConfirm
	ViewCloseBeadConfirm
	ViewDeleteBeadConfirm // Permanently delete bead(s)
	ViewAssignBeads
	ViewBeadSearch
	ViewLabelFilter
	ViewLinearImportInline // Import from Linear (inline in details panel)
	ViewPRImportInline     // Import from GitHub PR (inline in details panel)
	ViewHelp
	ViewLinearNotConfigured // Show Linear configuration instructions
	ViewToolMissing         // Show tool missing info box
)

// beadItem represents a bead in the beads panel with TUI-specific display state.
// It embeds beads.BeadWithDeps to access domain data directly.
type beadItem struct {
	*beads.BeadWithDeps

	// TUI-specific display state
	isReady bool // computed ready state
	treeDepth         int      // depth in tree view (0 = root)
	assignedWorkID    string   // work ID if already assigned to a work (empty = not assigned)
	isClosedParent    bool     // true if this is a closed bead included for tree context (has visible children)
	isLastChild       bool     // true if this bead is the last child of its parent
	treePrefixPattern string   // precomputed tree prefix pattern (e.g., "│ └─")
	children          []string // IDs of issues blocked by this one (computed from tree)
}

// beadFilters holds the current filter state for beads
type beadFilters struct {
	status     string // "open", "closed", "ready"
	label      string // filter by label (empty = no filter)
	searchText string // fuzzy search text
	sortBy     string // "default", "priority", "created", "title"

	// Entity-based filters (override status filter when set)
	task     string // task ID - show beads assigned to this task
	children string // bead ID - show children (dependents) of this bead

	// Context filters (always applied when work panel is present)
	rootIssue string // root issue ID - always include in results when set
}

// beadTypes is the list of valid bead types
var beadTypes = []string{
	"task",
	"bug",
	"feature",
	"epic",
	"chore",
	"merge-request",
	"molecule",
	"gate",
	"agent",
	"role",
	"rig",
	"convoy",
	"event",
}

// statusIcon returns the icon for a given status
func statusIcon(status string) string {
	switch status {
	// Internal db statuses
	case db.StatusPending:
		return statusPendingStyle().Render("○")
	case db.StatusProcessing:
		return statusProcessingStyle().Render("●")
	case db.StatusCompleted:
		return statusCompletedStyle().Render("✓")
	case db.StatusFailed:
		return statusFailedStyle().Render("✗")
	// Bead statuses from bd CLI
	case "open":
		return statusPendingStyle().Render("○")
	case "in_progress":
		return statusProcessingStyle().Render("●")
	case "blocked":
		return statusFailedStyle().Render("◐")
	case "deferred":
		return statusPendingStyle().Render("❄")
	case "closed":
		return statusCompletedStyle().Render("✓")
	default:
		return "?"
	}
}


// styleHotkeys styles text with hotkeys like "[c]reate [d]elete" by coloring the keys
// The keys inside brackets are rendered with tuiHotkeyStyle
func styleHotkeys(text string) string {
	var result strings.Builder
	i := 0
	for i < len(text) {
		if text[i] == '[' {
			// Find the closing bracket
			end := i + 1
			for end < len(text) && text[end] != ']' {
				end++
			}
			if end < len(text) {
				// Found a complete [key] sequence
				key := text[i+1 : end]
				result.WriteString("[")
				result.WriteString(tuiHotkeyStyle().Render(key))
				result.WriteString("]")
				i = end + 1
				continue
			}
		}
		result.WriteByte(text[i])
		i++
	}
	return result.String()
}

// styleButtonWithHover styles a button with hover effect if hovered is true
// This is used for clickable buttons and mode tabs in the TUI
func styleButtonWithHover(text string, hovered bool) string {
	t := CurrentTheme()
	hoverStyle := lipgloss.NewStyle().
		Foreground(t.Black).
		Background(t.Accent).
		Bold(true)

	if hovered {
		return hoverStyle.Render(text)
	}
	return styleHotkeys(text)
}


// fetchBeadsWithFilters fetches and filters beads based on provided filters
func fetchBeadsWithFilters(ctx context.Context, beadsClient *beads.Client, _ string, filters beadFilters) ([]beadItem, error) {
	// For "ready" status, use bd ready command
	if filters.status == "ready" {
		return fetchReadyBeads(ctx, beadsClient, filters)
	}

	// List issues with optional status filter
	// "open" means all non-closed statuses (open, in_progress, blocked, deferred)
	// "all" means no filter
	// Other values are passed directly as status filter
	statusFilter := ""
	filterOutClosed := false
	if filters.status == beads.StatusOpen {
		// Fetch all and filter out closed
		statusFilter = ""
		filterOutClosed = true
	} else if filters.status != "" && filters.status != "all" {
		statusFilter = filters.status
	}
	issuesList, err := beadsClient.ListBeads(ctx, statusFilter)
	if err != nil {
		return nil, err
	}

	// Filter out closed issues if "open" filter was requested
	if filterOutClosed {
		filtered := make([]beads.Bead, 0, len(issuesList))
		for _, issue := range issuesList {
			if issue.Status != beads.StatusClosed {
				filtered = append(filtered, issue)
			}
		}
		issuesList = filtered
	}

	// TODO: Apply label filter if needed (requires additional query support)

	// Get ready issues to mark which ones are ready
	readyIssues, _ := beadsClient.GetReadyBeads(ctx)
	readySet := make(map[string]bool)
	for _, issue := range readyIssues {
		readySet[issue.ID] = true
	}

	// Fetch dependency/dependent counts for all issues
	issueIDs := make([]string, 0, len(issuesList))
	for _, issue := range issuesList {
		issueIDs = append(issueIDs, issue.ID)
	}
	depsResult, err := beadsClient.GetBeadsWithDeps(ctx, issueIDs)
	if err != nil {
		return nil, err
	}

	var items []beadItem
	for _, issue := range issuesList {
		// Apply search filter
		if filters.searchText != "" {
			searchLower := strings.ToLower(filters.searchText)
			if !strings.Contains(strings.ToLower(issue.ID), searchLower) &&
				!strings.Contains(strings.ToLower(issue.Title), searchLower) &&
				!strings.Contains(strings.ToLower(issue.Description), searchLower) {
				continue
			}
		}

		beadWithDeps := depsResult.GetBead(issue.ID)
		if beadWithDeps == nil {
			// Fallback: create BeadWithDeps from the issue
			bead := issue
			beadWithDeps = &beads.BeadWithDeps{Bead: &bead}
		}
		items = append(items, beadItem{
			BeadWithDeps: beadWithDeps,
			isReady:      readySet[issue.ID],
		})
	}

	// Apply sorting
	items = sortBeadItems(items, filters.sortBy)

	return items, nil
}

func fetchReadyBeads(ctx context.Context, beadsClient *beads.Client, filters beadFilters) ([]beadItem, error) {
	// Get ready issues
	readyIssues, err := beadsClient.GetReadyBeads(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch dependency/dependent counts for ready issues
	issueIDs := make([]string, 0, len(readyIssues))
	for _, issue := range readyIssues {
		issueIDs = append(issueIDs, issue.ID)
	}
	depsResult, err := beadsClient.GetBeadsWithDeps(ctx, issueIDs)
	if err != nil {
		return nil, err
	}

	var items []beadItem
	for _, issue := range readyIssues {
		// Apply search filter
		if filters.searchText != "" {
			searchLower := strings.ToLower(filters.searchText)
			if !strings.Contains(strings.ToLower(issue.ID), searchLower) &&
				!strings.Contains(strings.ToLower(issue.Title), searchLower) &&
				!strings.Contains(strings.ToLower(issue.Description), searchLower) {
				continue
			}
		}

		beadWithDeps := depsResult.GetBead(issue.ID)
		if beadWithDeps == nil {
			// Fallback: create BeadWithDeps from the issue
			bead := issue
			beadWithDeps = &beads.BeadWithDeps{Bead: &bead}
		}
		items = append(items, beadItem{
			BeadWithDeps: beadWithDeps,
			isReady:      true,
		})
	}

	// Apply sorting
	items = sortBeadItems(items, filters.sortBy)

	return items, nil
}

func sortBeadItems(items []beadItem, sortBy string) []beadItem {
	switch sortBy {
	case "priority":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Priority < items[j].Priority
		})
	case "title":
		sort.Slice(items, func(i, j int) bool {
			return items[i].Title < items[j].Title
		})
	case "triage":
		// Triage sort: priority first, then by type (bug > task > feature)
		sort.Slice(items, func(i, j int) bool {
			if items[i].Priority != items[j].Priority {
				return items[i].Priority < items[j].Priority
			}
			typeOrder := map[string]int{"bug": 0, "task": 1, "feature": 2}
			return typeOrder[items[i].Type] < typeOrder[items[j].Type]
		})
	}
	return items
}
