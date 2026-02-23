package tui

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
)

// TUI-specific styles - shared across all TUI modes
var (
	tuiTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	tuiHotkeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("214")) // Orange for hotkeys

	tuiPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	tuiSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("255")).
				Background(lipgloss.Color("62"))

	tuiSelectedCheckStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	tuiLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("247"))

	tuiValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	tuiDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	tuiErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	tuiSuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	tuiStatusBarStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("236")).
				Padding(0, 1)

	tuiDialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2).
			Background(lipgloss.Color("235"))

	tuiHelpStyle = lipgloss.NewStyle().
			Padding(2, 4).
			Background(lipgloss.Color("235"))

	// Status indicator styles
	statusPending = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusProcessing = lipgloss.NewStyle().
				Foreground(lipgloss.Color("214")).
				Bold(true)

	statusCompleted = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	statusFailed = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	// Issue line styles
	issueIDStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")) // Orange

	issueTreeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")) // Dim gray for tree connectors

	// Type indicator styles
	typeTaskStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")) // Blue

	typeBugStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")) // Red

	typeFeatureStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")) // Green

	typeEpicStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("213")). // Pink/magenta
			Bold(true)

	typeChoreStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("247")) // Gray

	typeDefaultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("247")) // Gray for others

	// New bead animation style
	tuiNewBeadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFF00")). // Bright yellow for newly created beads
			Bold(true)
)

// Panel represents which panel is currently focused
type Panel int

const (
	PanelLeft        Panel = iota // Left panel (issues)
	PanelMiddle                   // Middle panel at current depth (used by tui.go)
	PanelRight                    // Right panel (details/forms)
	PanelWorkDetails              // Work details in split view
	PanelWorkTabs                 // Work tabs bar for work selection
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
	ViewZmxSessionPicker   // Zmx session picker (g key)
	ViewHelp
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
		return statusPending.Render("○")
	case db.StatusProcessing:
		return statusProcessing.Render("●")
	case db.StatusCompleted:
		return statusCompleted.Render("✓")
	case db.StatusFailed:
		return statusFailed.Render("✗")
	// Bead statuses from bd CLI
	case "open":
		return statusPending.Render("○")
	case "in_progress":
		return statusProcessing.Render("●")
	case "blocked":
		return statusFailed.Render("◐")
	case "deferred":
		return statusPending.Render("❄")
	case "closed":
		return statusCompleted.Render("✓")
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
				result.WriteString(tuiHotkeyStyle.Render(key))
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
	hoverStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("0")).   // Black text
		Background(lipgloss.Color("214")). // Orange background
		Bold(true)

	if hovered {
		return hoverStyle.Render(text)
	}
	return styleHotkeys(text)
}


// fetchBeadsWithFilters fetches and filters beads based on provided filters
func fetchBeadsWithFilters(ctx context.Context, beadsClient beads.Reader, _ string, filters beadFilters) ([]beadItem, error) {
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

func fetchReadyBeads(ctx context.Context, beadsClient beads.Reader, filters beadFilters) ([]beadItem, error) {
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

// compareIDsNatural compares two bead IDs using natural sort order.
// IDs like "ac-819" are split into prefix ("ac-") and numeric suffix (819),
// with the numeric part compared numerically instead of lexicographically.
// This ensures "ac-9" sorts before "ac-10".
func compareIDsNatural(a, b string) bool {
	aPre, aNum, aOk := splitIDNumeric(a)
	bPre, bNum, bOk := splitIDNumeric(b)

	if aOk && bOk && aPre == bPre {
		return aNum < bNum
	}
	return a < b
}

// splitIDNumeric splits an ID like "ac-819" into prefix "ac-" and numeric part 819.
// Returns the prefix, number, and whether parsing succeeded.
func splitIDNumeric(id string) (string, int, bool) {
	lastDash := strings.LastIndex(id, "-")
	if lastDash < 0 || lastDash == len(id)-1 {
		return "", 0, false
	}
	num, err := strconv.Atoi(id[lastDash+1:])
	if err != nil {
		return "", 0, false
	}
	return id[:lastDash+1], num, true
}
