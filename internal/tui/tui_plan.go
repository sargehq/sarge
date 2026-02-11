package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/messages"
	"github.com/sargehq/sarge/internal/tui/components/linearconfig"
	"github.com/sargehq/sarge/internal/tui/components/splash"
	"github.com/sargehq/sarge/internal/tui/ui"
	beadswatcher "github.com/sargehq/sarge/internal/beads/watcher"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/project"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
	"github.com/sargehq/sarge/internal/work"
	"github.com/sargehq/sarge/internal/zellij"
)

// watcherEventMsg wraps beads watcher events for tea.Msg
type watcherEventMsg beadswatcher.WatcherEvent

// trackingWatcherEventMsg wraps tracking watcher events for tea.Msg
type trackingWatcherEventMsg trackingwatcher.WatcherEvent

// planModel is the Plan Mode model focused on issue/bead management
type planModel struct {
	ctx         context.Context
	proj        *project.Project
	workService *work.WorkService // Shared WorkService for all work operations
	width       int
	height      int

	// Panels (self-contained rendering components)
	statusBar         *StatusBar
	issuesPanel       *IssuesPanel
	detailsPanel      *IssueDetailsPanel
	workTabsBar       *WorkTabsBar
	linearImportPanel *LinearImportPanel
	prImportPanel     *PRImportPanel
	beadFormPanel     *BeadFormPanel
	createWorkPanel   *CreateWorkPanel

	// Panel state
	activePanel Panel
	beadsCursor int

	// Data
	beadItems     []beadItem
	filters       beadFilters
	beadsExpanded bool

	// UI state
	viewMode      ViewMode
	spinner       spinner.Model
	textInput     textinput.Model // Used for search and label filter dialogs
	statusMessage string
	statusIsError bool
	lastUpdate    time.Time

	// Work state
	focusedWorkID          string          // ID of focused work (splits screen)
	workSelectionCleared   bool            // User manually cleared work selection filter (don't auto-restore)
	pendingWorkSelectIndex int             // Index of work to select after tiles load (-1 = none)
	workTiles              []*progress.WorkProgress // Cached work tiles for the tabs bar
	addChildToWorkID       string          // Work ID to add newly created child bead to (for add-child-and-run flow)

	// Multi-select state
	selectedBeads map[string]bool // beadID -> is selected

	// Loading state
	loading bool

	// Search sequence tracking to handle async refresh race conditions
	searchSeq uint64 // Incremented on each search change

	// Per-bead session tracking
	activeBeadSessions map[string]bool // beadID -> has active session
	zj                 zellij.SessionManager

	// Two-column layout settings
	columnRatio float64 // Ratio of issues column width (0.0-1.0), default 0.4 for 40/60 split

	// Mouse state
	mouseX              int
	mouseY              int
	hoveredButton       string    // which button is hovered ("n", "e", "w", "p", etc.)
	hoveredIssue        int       // index of hovered issue, -1 if none
	lastWheelScroll     time.Time // For debouncing rapid wheel events
	hoveredDialogButton string    // which dialog button is hovered ("ok", "cancel")

	// Database watcher for cache invalidation
	beadsWatcher    *beadswatcher.Watcher
	trackingWatcher *trackingwatcher.Watcher

	// New bead animation tracking
	newBeads map[string]time.Time // beadID -> creation timestamp for animation

	// Splash screen config (tool availability, platform, colors — computed once at startup)
	splashConfig       splash.Config
	linearConfigPanel  *linearconfig.Panel
	uiColors           ui.Colors

	// Top-level view tabs
	topView              string // "prompt" or "issues" — which top-level tab is active
	breadcrumbZonePrefix string // zone prefix for breadcrumb click detection
	viewTabsCursor       int    // keyboard-focused element in view tabs row (-1 = no focus)

	// Prompt (splash screen chat)
	promptInput        textinput.Model    // Text input for the prompt
	promptTimeline     *ui.ChatTimeline   // Scrollable chat message timeline
	promptAgentRunning bool               // True while headless prompt agent is executing

	// Info box state (for ViewToolMissing / ViewLinearNotConfigured overlays)
	infoBoxTitle string
	infoBoxBody  string
}

// newPlanModel creates a new Plan Mode model
func newPlanModel(ctx context.Context, proj *project.Project, version string) *planModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(CurrentTheme().Accent)

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40

	// Initialize beads database watcher
	beadsDBPath := filepath.Join(proj.BeadsPath(), "beads.db")
	beadsWatcher, err := beadswatcher.New(beadswatcher.DefaultConfig(beadsDBPath))
	if err != nil {
		// Log error but continue without watcher
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize beads watcher: %v\n", err)
		beadsWatcher = nil
	} else {
		if err := beadsWatcher.Start(); err != nil {
			// Log error and disable watcher
			fmt.Fprintf(os.Stderr, "Warning: Failed to start beads watcher: %v\n", err)
			beadsWatcher = nil
		}
	}

	// Initialize tracking database watcher
	trackingDBPath := filepath.Join(proj.Root, ".co", "tracking.db")
	trackingWatcher, err := trackingwatcher.New(trackingwatcher.DefaultConfig(trackingDBPath))
	if err != nil {
		// Log error but continue without watcher
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize tracking watcher: %v\n", err)
		trackingWatcher = nil
	} else {
		if err := trackingWatcher.Start(); err != nil {
			// Log error and disable watcher
			fmt.Fprintf(os.Stderr, "Warning: Failed to start tracking watcher: %v\n", err)
			trackingWatcher = nil
		}
	}

	m := &planModel{
		ctx:                    ctx,
		proj:                   proj,
		workService:            work.NewWorkService(proj),
		width:                  80,
		height:                 24,
		activePanel:            PanelLeft,
		spinner:                s,
		textInput:              ti,
		activeBeadSessions:     make(map[string]bool),
		selectedBeads:          make(map[string]bool),
		newBeads:               make(map[string]time.Time),
		zj:                     zellij.New(),
		columnRatio:            0.4,  // Default 40/60 split (issues/details)
		hoveredIssue:           -1,   // No issue hovered initially
		pendingWorkSelectIndex: -1,   // No pending work selection
		topView:                "prompt", // Start on prompt tab
		breadcrumbZonePrefix:   zone.NewPrefix(),
		viewTabsCursor:         -1, // No tab bar focus initially
		splashConfig:           buildSplashConfig(), // Tool checks run once at startup
		promptTimeline:         ui.NewChatTimeline(buildChatBubbleColors()),
		beadsWatcher:           beadsWatcher,
		trackingWatcher:        trackingWatcher,
		filters: beadFilters{
			status: "open",
			sortBy: "default",
		},
	}

	// Build shared UI colors from theme
	t := CurrentTheme()
	m.uiColors = ui.Colors{
		Accent: t.Accent,
		Text:   t.Text,
		Dim:    t.Dim,
		Border: t.DialogBorder,
		Error:  t.Error,
		Black:  t.Black,
	}
	m.linearConfigPanel = linearconfig.New(m.uiColors)

	// Initialize prompt input for splash screen
	promptTI := textinput.New()
	promptTI.Placeholder = "What do you want Sarge to work on next?"
	promptTI.CharLimit = 500
	promptTI.Width = 60
	promptTI.Focus()
	m.promptInput = promptTI

	// Initialize panels
	m.statusBar = NewStatusBar()
	m.issuesPanel = NewIssuesPanel()
	m.detailsPanel = NewIssueDetailsPanel()
	m.workTabsBar = NewWorkTabsBar()
	m.linearImportPanel = NewLinearImportPanel()
	m.prImportPanel = NewPRImportPanel()
	m.beadFormPanel = NewBeadFormPanel()
	m.createWorkPanel = NewCreateWorkPanel()

	m.statusBar.SetVersion(version)
	m.workTabsBar.SetVersion(version)

	// Set up status bar data providers
	m.statusBar.SetDataProviders(
		func() []beadItem { return m.beadItems },
		func() int { return m.beadsCursor },
		func() map[string]bool { return m.activeBeadSessions },
		func() ViewMode { return m.viewMode },
		func() string { return m.textInput.View() },
	)

	// No failed task selection (split view removed)
	m.statusBar.SetFailedTaskSelectedProvider(func() bool {
		return false
	})
	m.statusBar.SetFocusedWorkIDProvider(func() string {
		return m.focusedWorkID
	})

	return m
}

// SetSize implements SubModel
func (m *planModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Update prompt input and timeline widths
	contentWidth := width - 4
	if contentWidth > 100 {
		contentWidth = 100
	}
	if contentWidth < 40 {
		contentWidth = 40
	}
	m.promptInput.Width = contentWidth - 6 // account for border + padding
	m.promptTimeline.SetSize(contentWidth, 0) // height set at render time by splash
}

// FocusChanged implements SubModel
func (m *planModel) FocusChanged(focused bool) tea.Cmd {
	if focused {
		// Refresh data when gaining focus
		m.loading = true
		cmds := []tea.Cmd{m.refreshData()}
		// Load work tiles if a work is focused
		if m.focusedWorkID != "" {
			cmds = append(cmds, m.loadWorkTiles())
		}
		return tea.Batch(cmds...)
	}
	return nil
}

// InModal returns true if in a modal/dialog state
func (m *planModel) InModal() bool {
	return m.viewMode != ViewNormal
}

// IsCapturingInput returns true when the model is capturing text input
// (splash prompt, search, etc.) and single-key shortcuts should be suppressed.
func (m *planModel) IsCapturingInput() bool {
	if m.InModal() {
		return true
	}
	// Splash prompt captures input when user is typing or navigating messages
	if m.shouldShowSplash() && (m.promptInput.Value() != "" || m.promptTimeline.SelectedIdx() >= 0) {
		return true
	}
	return false
}

// Init implements tea.Model
func (m *planModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.workTabsBar.GetSpinner().Tick, // Tick the tabs bar spinner
		m.promptInput.Cursor.BlinkCmd(),  // Start cursor blinking for prompt
		m.refreshData(),
		m.loadWorkTiles(), // Load work tiles for the tabs bar
		m.loadMessages(),  // Load messages for chat timeline
	}

	// Subscribe to watcher events if watcher is available
	if m.beadsWatcher != nil {
		cmds = append(cmds, m.waitForWatcherEvent())
	}

	// Subscribe to tracking watcher events if watcher is available
	if m.trackingWatcher != nil {
		cmds = append(cmds, m.waitForTrackingWatcherEvent())
	}

	return tea.Batch(cmds...)
}

// waitForWatcherEvent waits for a watcher event and returns it as a tea.Msg
func (m *planModel) waitForWatcherEvent() tea.Cmd {
	if m.beadsWatcher == nil {
		return nil
	}

	return func() tea.Msg {
		sub := m.beadsWatcher.Broker().Subscribe(m.ctx)

		evt, ok := <-sub
		if !ok {
			return nil
		}

		return watcherEventMsg(evt.Payload)
	}
}

// waitForTrackingWatcherEvent waits for a tracking database watcher event and returns it as a tea.Msg
func (m *planModel) waitForTrackingWatcherEvent() tea.Cmd {
	if m.trackingWatcher == nil {
		return nil
	}

	return func() tea.Msg {
		sub := m.trackingWatcher.Broker().Subscribe(m.ctx)

		evt, ok := <-sub
		if !ok {
			return nil
		}

		return trackingWatcherEventMsg(evt.Payload)
	}
}

// Update implements tea.Model
func (m *planModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case watcherEventMsg:
		// Handle watcher events
		if msg.Type == beadswatcher.DBChanged {
			// Flush cache and trigger data reload
			if m.proj.Beads != nil {
				_ = m.proj.Beads.FlushCache(m.ctx)
			}
			// Trigger data reload and wait for next watcher event
			return m, tea.Batch(m.refreshData(), m.waitForWatcherEvent())
		} else if msg.Type == beadswatcher.WatcherError {
			// Log error and continue waiting for events
			return m, m.waitForWatcherEvent()
		}
		// Continue waiting for next event
		return m, m.waitForWatcherEvent()

	case trackingWatcherEventMsg:
		// Handle tracking database watcher events
		if msg.Type == trackingwatcher.DBChanged {
			// Tracking database changed - reload work tiles, work details, and messages
			cmds := []tea.Cmd{m.loadWorkTiles(), m.waitForTrackingWatcherEvent()}
			if m.shouldShowSplash() || m.topView == "prompt" || m.promptTimeline.MessageCount() > 0 {
				cmds = append(cmds, m.loadMessages())
			}
			return m, tea.Batch(cmds...)
		} else if msg.Type == trackingwatcher.WatcherError {
			// Log error and continue waiting for events
			return m, m.waitForTrackingWatcherEvent()
		}
		// Continue waiting for next event
		return m, m.waitForTrackingWatcherEvent()

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseMsg:
		m.mouseX = msg.X
		m.mouseY = msg.Y

		// Calculate status bar Y position (at bottom of view)
		statusBarY := m.height - 1

		// Handle hover detection for motion events
		if msg.Action == tea.MouseActionMotion {
			if msg.Y == statusBarY {
				m.hoveredButton = m.detectCommandsBarButton(msg)
				m.hoveredIssue = -1
				m.hoveredDialogButton = ""
			} else {
				m.hoveredButton = ""
				// Detect hover over dialog buttons if in form mode
				m.hoveredDialogButton = m.detectDialogButton(msg)
				if m.hoveredDialogButton != "" {
					m.hoveredIssue = -1
				} else {
					m.hoveredIssue = m.detectHoveredIssue(msg)
				}
			}
			return m, nil
		}

		// Handle mouse wheel events - route to appropriate panel based on mouse position
		if msg.Action == tea.MouseActionPress &&
			(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			return m.handleMouseWheel(msg)
		}

		// Handle clicks on status bar buttons
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// Check for clicks on view tabs bar (Prompt | Issues)
			if clickedView := m.detectViewTabClick(msg); clickedView != "" {
				m.topView = clickedView
				if clickedView == "prompt" {
					if !m.promptAgentRunning {
						m.promptInput.Focus()
					}
					return m, m.loadMessages()
				}
				m.promptInput.Blur()
				return m, nil
			}

			// Check for breadcrumb clicks
			if clicked := m.detectBreadcrumbClick(msg); clicked != "" {
				if clicked == "all" {
					// Clear work filter
					m.focusedWorkID = ""
					m.filters.task = ""
					m.filters.children = ""
					m.filters.rootIssue = ""
					m.workSelectionCleared = false
					m.activePanel = PanelLeft
					m.statusMessage = "Showing all issues"
					m.statusIsError = false
					return m, m.refreshData()
				}
				// Toggle: click same work again to deselect
				if m.focusedWorkID == clicked {
					m.focusedWorkID = ""
					m.filters.task = ""
					m.filters.children = ""
					m.filters.rootIssue = ""
					m.workSelectionCleared = false
					m.activePanel = PanelLeft
					m.statusMessage = "Showing all issues"
					m.statusIsError = false
					return m, m.refreshData()
				}
				// Select a work filter
				m.focusedWorkID = clicked
				m.workSelectionCleared = false
				m.viewMode = ViewNormal
				m.activePanel = PanelLeft
				m.statusMessage = fmt.Sprintf("Filtered to work %s", clicked)
				m.statusIsError = false

				// Clear unseen PR changes flag
				_ = m.proj.DB.MarkWorkPRSeen(m.ctx, clicked)

				return m, m.updateWorkSelectionFilter()
			}

			if msg.Y == statusBarY {
				clickedButton := m.detectCommandsBarButton(msg)
				// Trigger the corresponding action by simulating a key press
				switch clickedButton {
				case "n":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
				case "e":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
				case "a":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
				case "x":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
				case "d":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
				case "w":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
				case "p":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
				case "?":
					return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
				}
			} else {
				// Check if clicking on dialog buttons
				clickedDialogButton := m.detectDialogButton(msg)
				if clickedDialogButton == "ok" {
					// Handle different dialog types
					if m.viewMode == ViewLinearImportInline {
						// Submit Linear import
						result := m.linearImportPanel.GetResult()
						if result.IssueIDs != "" {
							m.viewMode = ViewNormal
							m.linearImportPanel.SetImporting(true)
							return m, m.importLinearIssue(result.IssueIDs)
						}
						return m, nil
					} else {
						// Submit bead form - inline the logic
						result := m.beadFormPanel.GetResult()
						if result.Title == "" {
							return m, nil
						}
						m.viewMode = ViewNormal
						m.beadFormPanel.Blur()

						// Determine mode and call appropriate action
						if result.EditBeadID != "" {
							// Edit mode
							return m, m.saveBeadEdit(result.EditBeadID, result.Title, result.Description, result.BeadType, result.Status)
						}

						// Create or add-child mode
						isEpic := result.BeadType == "epic"
						return m, m.createBead(result.Title, result.BeadType, result.Priority, isEpic, result.Description, result.ParentID)
					}
				} else if clickedDialogButton == "cancel" {
					// Cancel the form
					if m.viewMode == ViewLinearImportInline {
						m.linearImportPanel.Blur()
					} else if m.viewMode == ViewCreateWork {
						m.createWorkPanel.Blur()
					} else {
						m.beadFormPanel.Blur()
					}
					m.viewMode = ViewNormal
					return m, nil
				} else if clickedDialogButton == "execute" {
					// Handle execute button for work creation
					if m.viewMode == ViewCreateWork {
						result := m.createWorkPanel.GetResult()
						if result.BranchName == "" {
							m.statusMessage = "Branch name cannot be empty"
							m.statusIsError = true
							return m, nil
						}
						m.viewMode = ViewNormal
						m.selectedBeads = make(map[string]bool)
						return m, m.executeCreateWork(result.BeadID, result.BranchName, false, result.UseExistingBranch)
					}
				} else if clickedDialogButton == "auto" {
					// Handle auto button for work creation
					if m.viewMode == ViewCreateWork {
						result := m.createWorkPanel.GetResult()
						if result.BranchName == "" {
							m.statusMessage = "Branch name cannot be empty"
							m.statusIsError = true
							return m, nil
						}
						m.viewMode = ViewNormal
						m.selectedBeads = make(map[string]bool)
						return m, m.executeCreateWork(result.BeadID, result.BranchName, true, result.UseExistingBranch)
					}
				}

				// Check for issue clicks in the two-column layout
				clickedIssue := m.detectHoveredIssue(msg)
				if clickedIssue >= 0 && clickedIssue < len(m.beadItems) {
					m.beadsCursor = clickedIssue
					m.activePanel = PanelLeft
				} else if msg.X > m.width/2 {
					// Clicked on right side - switch to details panel
					m.activePanel = PanelRight
				}
			}
		}
		return m, nil

	case planDataMsg:
		// Ignore stale search results from older requests
		if msg.searchSeq < m.searchSeq {
			return m, nil
		}

		var expireCmds []tea.Cmd
		now := time.Now()

		// Detect new beads by comparing with existing list
		if len(m.beadItems) > 0 {
			existingIDs := make(map[string]bool)
			for _, bead := range m.beadItems {
				existingIDs[bead.ID] = true
			}
			for _, bead := range msg.beads {
				// Mark as new if not in existing list and not already animated
				if !existingIDs[bead.ID] && m.newBeads[bead.ID].IsZero() {
					m.newBeads[bead.ID] = now
					expireCmds = append(expireCmds, scheduleNewBeadExpire(bead.ID))
				}
			}
		}

		// Auto-switch to issues tab when beads appear (e.g., prompt agent created a bead)
		hadNoBeads := len(m.beadItems) == 0
		m.beadItems = msg.beads
		if hadNoBeads && len(m.beadItems) > 0 && m.topView == "prompt" {
			m.topView = "issues"
			m.promptInput.Blur()
		}
		if msg.activeSessions != nil {
			m.activeBeadSessions = msg.activeSessions
		}
		m.loading = false
		m.lastUpdate = time.Now()
		if msg.err != nil {
			m.statusMessage = msg.err.Error()
			m.statusIsError = true
		}

		// Ensure cursor stays within bounds after filter changes
		if m.beadsCursor >= len(m.beadItems) {
			if len(m.beadItems) > 0 {
				m.beadsCursor = len(m.beadItems) - 1
			} else {
				m.beadsCursor = 0
			}
		}

		// Check if we need to add a newly created bead to a work (add-child-and-run flow)
		if m.addChildToWorkID != "" && msg.createdBeadID != "" {
			workID := m.addChildToWorkID
			beadID := msg.createdBeadID
			// Don't clear addChildToWorkID yet - wait for beadAddedToWorkMsg
			cmds := append(expireCmds, m.addBeadsToWork([]string{beadID}, workID))
			return m, tea.Batch(cmds...)
		}

		// Don't clear status message on success - let it persist until next action
		if len(expireCmds) > 0 {
			return m, tea.Batch(expireCmds...)
		}
		return m, nil

	case planStatusMsg:
		m.statusMessage = msg.message
		m.statusIsError = msg.isError
		return m, nil

	case planSessionSpawnedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed: %v", msg.err)
			m.statusIsError = true
		} else if msg.resumed {
			m.statusMessage = fmt.Sprintf("Resumed session for %s", msg.beadID)
			m.statusIsError = false
		} else if msg.sessionCreated {
			m.statusMessage = fmt.Sprintf("Started session for %s | Zellij: zellij attach %s", msg.beadID, msg.sessionName)
			m.statusIsError = false
		} else {
			m.statusMessage = fmt.Sprintf("Started session for %s", msg.beadID)
			m.statusIsError = false
		}
		// Refresh to update session indicators
		return m, m.refreshData()

	case planWorkCreatedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to create work: %v", msg.err)
			m.statusIsError = true
		} else {
			if msg.sessionCreated {
				m.statusMessage = fmt.Sprintf("Created work %s | Zellij: zellij attach %s", msg.workID, msg.sessionName)
			} else {
				m.statusMessage = fmt.Sprintf("Created work %s from %s", msg.workID, msg.beadID)
			}
			m.statusIsError = false
		}
		// Refresh work tiles to show the new work in the tabs bar
		return m, tea.Batch(m.refreshData(), m.loadWorkTiles())

	case beadAddedToWorkMsg:
		m.viewMode = ViewNormal
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to add issue: %v", msg.err)
			m.statusIsError = true
			m.addChildToWorkID = "" // Clear on error
		} else {
			m.statusMessage = fmt.Sprintf("Added %s to work %s", msg.beadID, msg.workID)
			m.statusIsError = false

			// Check if we should run the work (add-child-and-run flow)
			if m.addChildToWorkID != "" && m.addChildToWorkID == msg.workID {
				m.addChildToWorkID = "" // Clear before running
				// Run the work in single-bead mode
				return m, tea.Batch(m.refreshData(), m.loadWorkTiles(), m.runFocusedWork(false))
			}
		}
		// Refresh work tiles to update the tabs bar
		return m, tea.Batch(m.refreshData(), m.loadWorkTiles())

	case workCommandMsg:
		// Reset to normal mode
		m.viewMode = ViewNormal
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("%s failed: %v", msg.action, msg.err)
			m.statusIsError = true
		} else {
			m.statusMessage = fmt.Sprintf("%s completed for %s", msg.action, msg.workID)
			m.statusIsError = false
			// If work was destroyed, clear the focused work
			if msg.action == "Destroy work" {
				m.focusedWorkID = ""
				m.filters.task = ""
				m.filters.children = ""
			}
		}
		// Refresh data and work tiles
		return m, tea.Batch(m.refreshData(), m.loadWorkTiles())

	case workTilesLoadedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to load works: %v", msg.err)
			m.statusIsError = true
			m.viewMode = ViewNormal
			m.loading = false
			m.pendingWorkSelectIndex = -1 // Clear pending selection on error
			return m, nil
		}
		m.workTiles = msg.works
		m.workTabsBar.SetWorkTiles(msg.works)
		m.workTabsBar.SetOrchestratorHealth(msg.orchestratorHealth)
		m.loading = false

		// Check for pending work selection (from [0-9] hotkey)
		if m.pendingWorkSelectIndex >= 0 {
			pendingIndex := m.pendingWorkSelectIndex
			m.pendingWorkSelectIndex = -1 // Clear pending selection
			return m.doSelectWorkAtIndex(pendingIndex)
		}

		// Rebuild the filter if a work filter is active
		if m.focusedWorkID != "" && !m.workSelectionCleared {
			return m, m.updateWorkSelectionFilter()
		}
		return m, nil

	case editorFinishedMsg:
		// Refresh data after external editor closes
		m.statusMessage = "Editor closed, refreshing..."
		m.statusIsError = false
		return m, m.refreshData()

	case linearImportCompleteMsg:
		m.linearImportPanel.SetImporting(false)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Import failed: %v", msg.err)
			m.statusIsError = true
		} else if msg.successCount > 0 || msg.skipCount > 0 || msg.errorCount > 0 {
			// Batch import results
			var summary []string

			if msg.successCount > 0 {
				summary = append(summary, fmt.Sprintf("%d imported", msg.successCount))
			}
			if msg.skipCount > 0 {
				summary = append(summary, fmt.Sprintf("%d skipped", msg.skipCount))
			}
			if msg.errorCount > 0 {
				summary = append(summary, fmt.Sprintf("%d failed", msg.errorCount))
			}

			m.statusMessage = fmt.Sprintf("Batch import: %s", strings.Join(summary, ", "))

			// Mark as error if there were failures, otherwise success
			m.statusIsError = msg.errorCount > 0

			// Log detailed errors and skip reasons if verbose output needed
			// These could be shown in a more detailed view or logged
			if msg.errorCount > 0 && len(msg.errors) > 0 {
				// Could expand to show first error in status
				m.statusMessage += fmt.Sprintf(" (first error: %s)", msg.errors[0])
			}
		} else if msg.skipReason != "" {
			// Single import skipped
			if len(msg.beadIDs) == 1 {
				m.statusMessage = fmt.Sprintf("%s: %s", msg.skipReason, msg.beadIDs[0])
			} else {
				m.statusMessage = msg.skipReason
			}
			m.statusIsError = false
		} else {
			// Single import success or legacy format
			if len(msg.beadIDs) == 1 {
				m.statusMessage = fmt.Sprintf("Successfully imported %s", msg.beadIDs[0])
			} else if len(msg.beadIDs) > 1 {
				m.statusMessage = fmt.Sprintf("Successfully imported %d issues", len(msg.beadIDs))
			} else {
				m.statusMessage = "Import completed (no new issues)"
			}
			m.statusIsError = false
		}
		return m, tea.Batch(m.refreshData(), clearStatusAfter(7*time.Second))

	case linearImportProgressMsg:
		if msg.total > 0 {
			m.statusMessage = fmt.Sprintf("Importing... [%d/%d] %s", msg.current, msg.total, msg.message)
		} else {
			m.statusMessage = msg.message
		}
		m.statusIsError = false
		return m, nil

	case prImportPreviewMsg:
		m.prImportPanel.SetPreviewing(false)
		m.prImportPanel.SetPreviewResult(msg.metadata, msg.err)
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Preview failed: %v", msg.err)
			m.statusIsError = true
		} else {
			m.statusMessage = fmt.Sprintf("PR #%d: %s", msg.metadata.Number, msg.metadata.Title)
			m.statusIsError = false
		}
		return m, nil

	case prImportCompleteMsg:
		m.prImportPanel.SetImporting(false)
		m.viewMode = ViewNormal
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("PR import failed: %v", msg.err)
			m.statusIsError = true
		} else {
			m.statusMessage = fmt.Sprintf("Imported PR into work %s", msg.workID)
			m.statusIsError = false
		}
		return m, tea.Batch(m.refreshData(), m.loadWorkTiles(), clearStatusAfter(7*time.Second))

	case promptAgentDoneMsg:
		// Agent finished — clear thinking state, re-enable input, reload messages
		m.promptAgentRunning = false
		m.promptInput.Focus()
		return m, m.loadMessages()

	case messagesLoadedMsg:
		if msg.err == nil {
			msgs := msg.messages
			// If the prompt agent is still running, append a thinking indicator
			if m.promptAgentRunning {
				msgs = append(msgs, ui.ChatMessage{
					Source: ui.MessageFromSystem,
					Text:   "Thinking...",
					Time:   "now",
				})
			}
			if len(msgs) > 0 {
				m.promptTimeline.SetMessages(msgs)
			}
		}
		return m, nil

	case statusClearMsg:
		m.statusMessage = ""
		m.statusIsError = false
		return m, nil

	case newBeadExpireMsg:
		// Remove the bead from the newBeads map to stop animation
		delete(m.newBeads, msg.beadID)
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case spinner.TickMsg:
		// Update both spinners
		var cmd1, cmd2 tea.Cmd
		m.spinner, cmd1 = m.spinner.Update(msg)
		tabsSpinner := m.workTabsBar.GetSpinner()
		tabsSpinner, cmd2 = tabsSpinner.Update(msg)
		m.workTabsBar.UpdateSpinner(tabsSpinner)
		return m, tea.Batch(cmd1, cmd2)

	default:
		// Route cursor blink messages to prompt input when splash is visible
		if m.shouldShowSplash() && m.promptTimeline.SelectedIdx() < 0 {
			var cmd tea.Cmd
			m.promptInput, cmd = m.promptInput.Update(msg)
			if cmd != nil {
				return m, cmd
			}
		}

		// Handle Kitty keyboard protocol escape sequences
		// Kitty/Ghostty send keys as CSI <keycode> ; <modifiers> u
		typeName := fmt.Sprintf("%T", msg)
		if typeName == "tea.unknownCSISequenceMsg" {
			msgStr := fmt.Sprintf("%s", msg)
			// Check for Kitty protocol escape key: "?CSI[50 55 117]?" = "27u"
			if strings.Contains(msgStr, "50 55 117") {
				return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyEsc})
			}
			// Check for Ctrl+G: 103;5u = bytes "49 48 51 59 53 117"
			if strings.Contains(msgStr, "49 48 51 59 53 117") {
				return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlG})
			}
			// Check for Ctrl+S: 115;5u = bytes "49 49 53 59 53 117"
			if strings.Contains(msgStr, "49 49 53 59 53 117") {
				return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlS})
			}
			// Check for Ctrl+O: 111;5u = bytes "49 49 49 59 53 117"
			if strings.Contains(msgStr, "49 49 49 59 53 117") {
				return m.handleKeyPress(tea.KeyMsg{Type: tea.KeyCtrlO})
			}
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
}

// planDataMsg is sent when data is refreshed
type planDataMsg struct {
	beads          []beadItem
	activeSessions map[string]bool
	err            error
	searchSeq      uint64 // Sequence number to detect stale results
	createdBeadID  string // ID of newly created bead (for add-child-and-run flow)
}

// planStatusMsg is sent to update status text
type planStatusMsg struct {
	message string
	isError bool
}

// planSessionSpawnedMsg indicates a planning session was spawned or resumed
type planSessionSpawnedMsg struct {
	beadID         string
	resumed        bool
	err            error
	sessionCreated bool   // true if a new zellij session was created
	sessionName    string // e.g., 'co-myproject'
}

// planWorkCreatedMsg indicates work was created from a bead
type planWorkCreatedMsg struct {
	beadID         string
	workID         string
	err            error
	sessionCreated bool   // true if a new zellij session was created
	sessionName    string // e.g., 'co-myproject'
}

// beadAddedToWorkMsg indicates a bead was added to a work
type beadAddedToWorkMsg struct {
	beadID string
	workID string
	err    error
}

// editorFinishedMsg is sent when the external editor closes
type editorFinishedMsg struct{}

// linearImportCompleteMsg is sent when a Linear import completes
type linearImportCompleteMsg struct {
	beadIDs      []string // IDs of imported beads
	err          error
	skipReason   string   // For single import: reason for skipping
	successCount int      // For batch import: number of successful imports
	skipCount    int      // For batch import: number of skipped issues
	errorCount   int      // For batch import: number of failed imports
	skipReasons  []string // For batch import: detailed skip reasons
	errors       []string // For batch import: detailed error messages
}

// linearImportProgressMsg is sent to update Linear import progress
type linearImportProgressMsg struct {
	current int
	total   int
	message string
}

// statusClearMsg is sent to clear the status message after a delay
type statusClearMsg struct{}

// clearStatusAfter returns a command that clears the status after the given duration
func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return statusClearMsg{}
	})
}

// newBeadExpireMsg is sent when the animation for a new bead should expire
type newBeadExpireMsg struct {
	beadID string
}

// promptAgentDoneMsg indicates the prompt agent finished
type promptAgentDoneMsg struct {
	err error
}

// messagesLoadedMsg indicates messages were loaded from the DB
type messagesLoadedMsg struct {
	messages []ui.ChatMessage
	err      error
}

// workCommandMsg indicates a work command completed
type workCommandMsg struct {
	action string
	workID string
	err    error
}

// newBeadAnimationDuration is how long newly created beads are highlighted
const newBeadAnimationDuration = 5 * time.Second

// scheduleNewBeadExpire returns a command that expires a new bead animation after the duration
func scheduleNewBeadExpire(beadID string) tea.Cmd {
	return tea.Tick(newBeadAnimationDuration, func(t time.Time) tea.Msg {
		return newBeadExpireMsg{beadID: beadID}
	})
}

func (m *planModel) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Route to splash prompt handler when splash is visible OR prompt tab is active
	// But NOT when focus is on the view tabs or work header — those use normal navigation
	if (m.shouldShowSplash() || m.topView == "prompt") && m.activePanel != PanelViewTabs && m.activePanel != PanelWorkHeader {
		// When input is empty, let known shortcuts fall through to normal handling
		if m.promptInput.Value() == "" && msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			switch string(msg.Runes) {
			case "n", "i", "I", "?":
				// Fall through to normal key handling below
			case "q":
				if m.shouldShowSplash() {
					// On splash screen: quit
					m.cleanup()
					return m, tea.Quit
				}
				// On prompt tab: switch to issues
				m.topView = "issues"
				m.promptInput.Blur()
				return m, nil
			default:
				return m.handleSplashPromptKey(msg)
			}
		} else {
			return m.handleSplashPromptKey(msg)
		}
	}

	// Handle escape key globally
	if msg.Type == tea.KeyEsc && m.viewMode == ViewNormal {
		// If in view tabs or work header, return to issues panel
		if m.activePanel == PanelViewTabs || m.activePanel == PanelWorkHeader {
			m.activePanel = PanelLeft
			m.viewTabsCursor = -1
			return m, nil
		}
		// If work filter is active, clear it
		if m.focusedWorkID != "" {
			m.focusedWorkID = ""
			m.filters.task = ""
			m.filters.children = ""
			m.filters.rootIssue = ""
			m.workSelectionCleared = false
			m.activePanel = PanelLeft
			m.statusMessage = "Showing all issues"
			m.statusIsError = false
			return m, m.refreshData()
		}
	}

	// Handle dialog-specific input
	switch m.viewMode {
	case ViewCreateBead, ViewCreateBeadInline, ViewAddChildBead, ViewEditBead:
		// Delegate to bead form panel and handle returned action
		cmd, action := m.beadFormPanel.Update(msg)

		switch action {
		case BeadFormActionCancel:
			m.viewMode = ViewNormal
			return m, cmd

		case BeadFormActionSubmit:
			result := m.beadFormPanel.GetResult()
			if result.Title == "" {
				return m, cmd
			}

			m.viewMode = ViewNormal
			m.beadFormPanel.Blur()

			// Determine mode and call appropriate action
			if result.EditBeadID != "" {
				// Edit mode
				return m, m.saveBeadEdit(result.EditBeadID, result.Title, result.Description, result.BeadType, result.Status)
			}

			// Create or add-child mode
			isEpic := result.BeadType == "epic"
			return m, m.createBead(result.Title, result.BeadType, result.Priority, isEpic, result.Description, result.ParentID)
		}

		return m, cmd
	case ViewCreateWork:
		// Delegate to create work panel and handle returned action
		cmd, action := m.createWorkPanel.Update(msg)

		switch action {
		case CreateWorkActionCancel:
			m.viewMode = ViewNormal
			return m, cmd

		case CreateWorkActionExecute:
			result := m.createWorkPanel.GetResult()
			if result.BranchName == "" {
				m.statusMessage = "Branch name cannot be empty"
				m.statusIsError = true
				return m, nil
			}
			m.viewMode = ViewNormal
			// Clear selections after work creation
			m.selectedBeads = make(map[string]bool)
			return m, m.executeCreateWork(result.BeadID, result.BranchName, false, result.UseExistingBranch)

		case CreateWorkActionAuto:
			result := m.createWorkPanel.GetResult()
			if result.BranchName == "" {
				m.statusMessage = "Branch name cannot be empty"
				m.statusIsError = true
				return m, nil
			}
			m.viewMode = ViewNormal
			// Clear selections after work creation
			m.selectedBeads = make(map[string]bool)
			return m, m.executeCreateWork(result.BeadID, result.BranchName, true, result.UseExistingBranch)
		}

		return m, cmd
	case ViewBeadSearch:
		return m.updateBeadSearch(msg)
	case ViewLabelFilter:
		return m.updateLabelFilter(msg)
	case ViewCloseBeadConfirm:
		return m.updateCloseBeadConfirm(msg)
	case ViewDeleteBeadConfirm:
		return m.updateDeleteBeadConfirm(msg)
	case ViewLinearImportInline:
		// Delegate to linear import panel and handle returned action
		cmd, action := m.linearImportPanel.Update(msg)

		switch action {
		case LinearImportActionCancel:
			m.viewMode = ViewNormal
			return m, cmd

		case LinearImportActionSubmit:
			result := m.linearImportPanel.GetResult()
			if result.IssueIDs != "" {
				m.viewMode = ViewNormal
				m.linearImportPanel.SetImporting(true)
				return m, m.importLinearIssue(result.IssueIDs)
			}
			return m, cmd
		}

		return m, cmd
	case ViewPRImportInline:
		// Delegate to PR import panel and handle returned action
		cmd, action := m.prImportPanel.Update(msg)

		switch action {
		case PRImportActionCancel:
			m.viewMode = ViewNormal
			return m, cmd

		case PRImportActionPreview:
			result := m.prImportPanel.GetResult()
			if result.PRURL != "" {
				m.prImportPanel.SetPreviewing(true)
				return m, m.previewPR(result.PRURL)
			}
			return m, cmd

		case PRImportActionSubmit:
			result := m.prImportPanel.GetResult()
			if result.PRURL != "" {
				m.prImportPanel.SetImporting(true)
				return m, m.importPR(result.PRURL)
			}
			return m, cmd
		}

		return m, cmd
	case ViewDestroyConfirm:
		// Handle destroy confirmation dialog
		switch msg.String() {
		case "y", "Y":
			if m.focusedWorkID != "" {
				// Return to normal mode after destroy
				m.viewMode = ViewNormal
				return m, m.destroyFocusedWork()
			}
		case "n", "N", "esc":
			// Return to normal mode on cancel
			m.viewMode = ViewNormal
		}
		return m, nil
	case ViewHelp:
		m.viewMode = ViewNormal
		return m, nil
	case ViewLinearNotConfigured:
		// Route to linear config panel
		cmd, action := m.linearConfigPanel.Update(msg)
		switch action {
		case linearconfig.ActionCancel:
			m.viewMode = ViewNormal
			return m, nil
		case linearconfig.ActionSubmit:
			m.viewMode = ViewNormal
			return m, m.saveLinearAPIKey(m.linearConfigPanel.Value())
		}
		return m, cmd
	case ViewToolMissing:
		// Any key dismisses the info box
		m.viewMode = ViewNormal
		return m, nil
	}

	// Normal mode key handling

	// Handle [1-9] keys to select work filter by index
	if key := msg.String(); len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		digit := int(key[0] - '0')
		return m.selectWorkByIndex(digit)
	}

	switch msg.String() {
	case "tab":
		// Cycle forward: ViewTabs → WorkHeader (if visible) → Left → Right → ViewTabs
		hasWorkHeader := m.focusedWorkID != ""
		switch m.activePanel {
		case PanelViewTabs:
			m.viewTabsCursor = -1
			if hasWorkHeader {
				m.activePanel = PanelWorkHeader
			} else {
				m.activePanel = PanelLeft
			}
		case PanelWorkHeader:
			m.activePanel = PanelLeft
		case PanelLeft:
			m.activePanel = PanelRight
		case PanelRight:
			m.activePanel = PanelViewTabs
			if m.topView == "prompt" {
				m.viewTabsCursor = 0
			} else {
				m.viewTabsCursor = 1
			}
		}
		return m, nil

	case "shift+tab":
		// Cycle backward: ViewTabs ← WorkHeader (if visible) ← Left ← Right ← ViewTabs
		hasWorkHeader := m.focusedWorkID != ""
		switch m.activePanel {
		case PanelViewTabs:
			m.viewTabsCursor = -1
			m.activePanel = PanelRight
		case PanelWorkHeader:
			m.activePanel = PanelViewTabs
			if m.topView == "prompt" {
				m.viewTabsCursor = 0
			} else {
				m.viewTabsCursor = 1
			}
		case PanelLeft:
			if hasWorkHeader {
				m.activePanel = PanelWorkHeader
			} else {
				m.activePanel = PanelViewTabs
				if m.topView == "prompt" {
					m.viewTabsCursor = 0
				} else {
					m.viewTabsCursor = 1
				}
			}
		case PanelRight:
			m.activePanel = PanelLeft
		}
		return m, nil

	case "h", "left":
		if m.activePanel == PanelViewTabs {
			// Move cursor left through view tabs elements
			if m.viewTabsCursor > 0 {
				m.viewTabsCursor--
			}
			return m, nil
		}
		// Simple left navigation in content panels
		if m.activePanel == PanelRight {
			m.activePanel = PanelLeft
		}
		return m, nil

	case "l", "right":
		if m.activePanel == PanelViewTabs {
			// Move cursor right through view tabs elements
			maxIdx := m.viewTabsElementCount() - 1
			if m.viewTabsCursor < maxIdx {
				m.viewTabsCursor++
			}
			return m, nil
		}
		// Simple right navigation in content panels
		if m.activePanel == PanelLeft {
			m.activePanel = PanelRight
		}
		return m, nil

	case "j", "down":
		switch m.activePanel {
		case PanelViewTabs:
			// Move down from view tabs → work header (if visible) or left panel
			if m.focusedWorkID != "" {
				m.activePanel = PanelWorkHeader
			} else {
				m.activePanel = PanelLeft
			}
			m.viewTabsCursor = -1
		case PanelWorkHeader:
			// Move down from work header → left panel
			m.activePanel = PanelLeft
		default:
			// Navigate down within issues list
			if m.beadsCursor < len(m.beadItems)-1 {
				m.beadsCursor++
			}
		}
		return m, nil

	case "k", "up":
		switch m.activePanel {
		case PanelViewTabs:
			// Already at top — no-op
		case PanelWorkHeader:
			// Move up from work header → view tabs
			m.activePanel = PanelViewTabs
			// Set cursor to current active tab
			if m.topView == "prompt" {
				m.viewTabsCursor = 0
			} else {
				m.viewTabsCursor = 1
			}
		case PanelLeft, PanelRight:
			if m.beadsCursor > 0 {
				// Navigate up within list
				m.beadsCursor--
			} else {
				// At top of list — move to work header (if visible) or view tabs
				if m.focusedWorkID != "" {
					m.activePanel = PanelWorkHeader
				} else {
					m.activePanel = PanelViewTabs
					if m.topView == "prompt" {
						m.viewTabsCursor = 0
					} else {
						m.viewTabsCursor = 1
					}
				}
			}
		default:
			if m.beadsCursor > 0 {
				m.beadsCursor--
			}
		}
		return m, nil

	case "enter":
		// Activate element in view tabs when focused
		if m.activePanel == PanelViewTabs {
			return m.activateViewTabsCursor()
		}
		return m, nil

	case "n":
		// Create new bead inline — requires bd
		if !m.splashConfig.HasBd {
			m.infoBoxTitle = "beads (bd) Not Installed"
			m.infoBoxBody = "Creating issues requires the bd CLI.\n\n" +
				"See https://github.com/beads-project/beads"
			m.viewMode = ViewToolMissing
			return m, nil
		}
		// Switch to issues view if on prompt tab (form renders in issues layout)
		if m.topView == "prompt" {
			m.topView = "issues"
			m.promptInput.Blur()
		}
		m.viewMode = ViewCreateBeadInline
		m.beadFormPanel.Reset()
		return m, m.beadFormPanel.Init()

	case "x":
		// Close selected bead(s)
		if len(m.beadItems) > 0 {
			// Check if we have any selected beads
			hasSelection := false
			for _, item := range m.beadItems {
				if m.selectedBeads[item.ID] {
					hasSelection = true
					break
				}
			}
			// If we have selected beads or a cursor bead, show confirmation
			if hasSelection || m.beadsCursor < len(m.beadItems) {
				m.viewMode = ViewCloseBeadConfirm
			}
		}
		return m, nil

	case "d":
		// Delete selected bead(s) permanently
		if len(m.beadItems) > 0 {
			// Check if we have any selected beads
			hasSelection := false
			for _, item := range m.beadItems {
				if m.selectedBeads[item.ID] {
					hasSelection = true
					break
				}
			}
			// If we have selected beads or a cursor bead, show confirmation
			if hasSelection || m.beadsCursor < len(m.beadItems) {
				m.viewMode = ViewDeleteBeadConfirm
			}
		}
		return m, nil

	case "/":
		// Search
		m.viewMode = ViewBeadSearch
		m.textInput.Reset()
		m.textInput.SetValue(m.filters.searchText)
		m.textInput.Focus()
		return m, nil

	case "L":
		// Label filter
		m.viewMode = ViewLabelFilter
		m.textInput.Reset()
		m.textInput.SetValue(m.filters.label)
		m.textInput.Focus()
		return m, nil

	case "*":
		// Show all issues (clear status filter AND work selection filter)
		m.filters.status = "all"
		m.filters.task = ""
		m.filters.children = ""
		m.workSelectionCleared = true // Prevent auto-restore on refresh
		return m, m.refreshData()

	case "o":
		m.filters.status = beads.StatusOpen
		return m, m.refreshData()

	case "c":
		// Filter to closed issues (work details panel handles 'c' for agent)
		m.filters.status = beads.StatusClosed
		return m, m.refreshData()

	case "r":
		// Filter to ready issues (work details panel handles 'r' for Run)
		m.filters.status = "ready"
		return m, m.refreshData()

	case "s":
		// Cycle sort mode
		switch m.filters.sortBy {
		case "default":
			m.filters.sortBy = "priority"
		case "priority":
			m.filters.sortBy = "title"
		default:
			m.filters.sortBy = "default"
		}
		return m, m.refreshData()

	case "v":
		m.beadsExpanded = !m.beadsExpanded
		return m, nil

	case "[":
		// Decrease column ratio (make issues column narrower)
		if m.columnRatio > 0.3 {
			m.columnRatio -= 0.1
			if m.columnRatio < 0.3 {
				m.columnRatio = 0.3
			}
		}
		return m, nil

	case "]":
		// Increase column ratio (make issues column wider)
		if m.columnRatio < 0.5 {
			m.columnRatio += 0.1
			if m.columnRatio > 0.5 {
				m.columnRatio = 0.5
			}
		}
		return m, nil

	case " ":
		// Toggle bead selection for multi-select
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			bead := m.beadItems[m.beadsCursor]
			// Prevent selecting already-assigned beads
			if bead.assignedWorkID != "" {
				m.statusMessage = fmt.Sprintf("Cannot select: already assigned to %s", bead.assignedWorkID)
				m.statusIsError = true
				return m, nil
			}
			m.selectedBeads[bead.ID] = !m.selectedBeads[bead.ID]
		}
		return m, nil

	case "p":
		// Spawn/resume planning session for selected bead (work details panel handles 'p' for Plan)
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			beadID := m.beadItems[m.beadsCursor].ID
			return m, m.spawnPlanSession(beadID)
		}
		return m, nil

	case "w":
		// Create work from cursor bead - show dialog
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			bead := m.beadItems[m.beadsCursor]
			if bead.assignedWorkID != "" {
				m.statusMessage = fmt.Sprintf("Cannot create work: %s already assigned to %s", bead.ID, bead.assignedWorkID)
				m.statusIsError = true
				return m, nil
			}
			// Generate proposed branch name from cursor bead
			branchBeads := []*beadsForBranch{{ID: bead.ID, Title: bead.Title}}
			branchName := generateBranchNameFromBeadsForBranch(branchBeads)
			m.createWorkPanel.Reset(bead.ID, branchName)
			// Load available branches for the "existing branch" mode
			if branches, err := git.NewOperations().ListBranches(m.ctx, m.proj.MainRepoPath()); err == nil {
				m.createWorkPanel.SetBranches(branches)
			}
			m.viewMode = ViewCreateWork
			return m, m.createWorkPanel.Init()
		}
		return m, nil

	case "a":
		// Add child issue to selected issue
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			m.beadFormPanel.SetAddChildMode(m.beadItems[m.beadsCursor].ID)
			m.viewMode = ViewAddChildBead
			return m, m.beadFormPanel.Init()
		}
		return m, nil

	case "e":
		// Edit selected issue using the unified bead form
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			bead := m.beadItems[m.beadsCursor]
			m.beadFormPanel.SetEditMode(bead.ID, bead.Title, bead.Description, bead.Type, bead.Priority, bead.Status)
			m.viewMode = ViewEditBead
			return m, m.beadFormPanel.Init()
		}
		return m, nil

	case "E":
		// Edit selected issue in external editor
		if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			bead := m.beadItems[m.beadsCursor]
			return m, m.openInEditor(bead.ID)
		}
		return m, nil

	case "i":
		// Import Linear issue inline - check for API key first
		var apiKey string
		if m.proj.Config != nil {
			apiKey = m.proj.Config.Linear.APIKey
		}
		if apiKey == "" {
			m.linearConfigPanel.SetSize(m.width, m.height)
			m.viewMode = ViewLinearNotConfigured
			return m, m.linearConfigPanel.Init()
		}
		m.viewMode = ViewLinearImportInline
		m.linearImportPanel.Reset()
		return m, m.linearImportPanel.Init()

	case "I":
		// Import GitHub PR inline
		m.viewMode = ViewPRImportInline
		m.prImportPanel.Reset()
		return m, m.prImportPanel.Init()

	case "A":
		// Add selected issue(s) to the focused work
		if m.focusedWorkID == "" {
			m.statusMessage = "Select a work first (press 1-9 to select a work)"
			m.statusIsError = true
			return m, nil
		}
		if len(m.beadItems) > 0 {
			// Collect selected beads or use cursor bead
			var beadsToAdd []string
			hasSelection := false
			for _, item := range m.beadItems {
				if m.selectedBeads[item.ID] {
					hasSelection = true
					// Check if already assigned
					if item.assignedWorkID != "" {
						m.statusMessage = fmt.Sprintf("Issue %s already assigned to %s", item.ID, item.assignedWorkID)
						m.statusIsError = true
						return m, nil
					}
					beadsToAdd = append(beadsToAdd, item.ID)
				}
			}

			// If no selection, use cursor bead
			if !hasSelection && m.beadsCursor < len(m.beadItems) {
				bead := m.beadItems[m.beadsCursor]
				if bead.assignedWorkID != "" {
					m.statusMessage = fmt.Sprintf("Issue %s already assigned to %s", bead.ID, bead.assignedWorkID)
					m.statusIsError = true
					return m, nil
				}
				beadsToAdd = append(beadsToAdd, bead.ID)
			}

			if len(beadsToAdd) > 0 {
				// Add issues directly to the focused work
				m.selectedBeads = make(map[string]bool) // Clear selection after adding
				return m, m.addBeadsToWork(beadsToAdd, m.focusedWorkID)
			}
		}
		return m, nil

	// Work commands (Shift keys, available when a work filter is active)
	case "T":
		if m.focusedWorkID != "" {
			return m, m.openConsole()
		}
		return m, nil

	case "C":
		if m.focusedWorkID != "" {
			return m, m.openAgent()
		}
		return m, nil

	case "R":
		if m.focusedWorkID != "" {
			return m, m.runFocusedWork(false)
		}
		return m, nil

	case "V":
		if m.focusedWorkID != "" {
			return m, m.createReviewTask()
		}
		return m, nil

	case "P":
		if m.focusedWorkID != "" {
			return m, m.createPRTask()
		}
		return m, nil

	case "F":
		if m.focusedWorkID != "" {
			return m, m.checkPRFeedback()
		}
		return m, nil

	case "D":
		if m.focusedWorkID != "" {
			m.viewMode = ViewDestroyConfirm
			return m, nil
		}
		return m, nil

	case "O":
		if m.focusedWorkID != "" {
			return m, m.restartOrchestrator()
		}
		return m, nil

	case "?":
		m.viewMode = ViewHelp
		return m, nil

	case "q":
		// Clean up resources before quitting
		m.cleanup()
		return m, tea.Quit
	}

	return m, nil
}

// activateViewTabsCursor activates the element at the current viewTabsCursor position.
// Indices: 0=Prompt, 1=Issues, 2=All breadcrumb, 3+=work breadcrumbs.
func (m *planModel) activateViewTabsCursor() (tea.Model, tea.Cmd) {
	switch m.viewTabsCursor {
	case 0:
		// Switch to Prompt tab
		m.topView = "prompt"
		if !m.promptAgentRunning {
			m.promptInput.Focus()
		}
		m.activePanel = PanelLeft
		m.viewTabsCursor = -1
		return m, m.loadMessages()
	case 1:
		// Switch to Issues tab
		m.topView = "issues"
		m.promptInput.Blur()
		m.activePanel = PanelLeft
		m.viewTabsCursor = -1
		return m, nil
	case 2:
		// "All" breadcrumb — clear work filter and switch to Issues tab
		m.topView = "issues"
		m.promptInput.Blur()
		m.focusedWorkID = ""
		m.filters.task = ""
		m.filters.children = ""
		m.filters.rootIssue = ""
		m.workSelectionCleared = false
		m.activePanel = PanelLeft
		m.viewTabsCursor = -1
		m.statusMessage = "Showing all issues"
		m.statusIsError = false
		return m, m.refreshData()
	default:
		// Work breadcrumb — index 3+ maps to workTiles[index-3]
		workTiles := m.workTabsBar.GetWorkTiles()
		workIdx := m.viewTabsCursor - 3
		if workIdx >= 0 && workIdx < len(workTiles) && workTiles[workIdx] != nil {
			workID := workTiles[workIdx].Work.ID
			// Always switch to Issues tab when activating a work breadcrumb
			m.topView = "issues"
			m.promptInput.Blur()
			// Toggle: same work deselects
			if m.focusedWorkID == workID {
				m.focusedWorkID = ""
				m.filters.task = ""
				m.filters.children = ""
				m.filters.rootIssue = ""
				m.workSelectionCleared = false
				m.activePanel = PanelLeft
				m.viewTabsCursor = -1
				m.statusMessage = "Showing all issues"
				m.statusIsError = false
				return m, m.refreshData()
			}
			m.focusedWorkID = workID
			m.workSelectionCleared = false
			m.viewMode = ViewNormal
			m.activePanel = PanelLeft
			m.viewTabsCursor = -1
			m.statusMessage = fmt.Sprintf("Filtered to work %s", workID)
			m.statusIsError = false
			_ = m.proj.DB.MarkWorkPRSeen(m.ctx, workID)
			return m, m.updateWorkSelectionFilter()
		}
	}
	return m, nil
}

// cleanup releases resources when the TUI exits
func (m *planModel) cleanup() {
	// Stop the beads watcher if it's running
	if m.beadsWatcher != nil {
		_ = m.beadsWatcher.Stop()
	}
	// Note: m.proj.Beads is owned by the Project and closed by proj.Close()
	// which is deferred in runTUI. Do not close it here to avoid double-close.
}

// syncPanels synchronizes data from planModel to the panel components
func (m *planModel) syncPanels() {
	// Calculate column widths
	totalContentWidth := m.width - 4
	issuesWidth := int(float64(totalContentWidth) * m.columnRatio)
	detailsWidth := totalContentWidth - issuesWidth

	// Status bar always uses issues context (no split view)
	statusBarCtx := StatusBarContextIssues

	// Sync status bar
	m.statusBar.SetSize(m.width)
	m.statusBar.SetContext(statusBarCtx)
	m.statusBar.SetStatus(m.statusMessage, m.statusIsError)
	m.statusBar.SetLoading(m.loading)
	m.statusBar.SetLastUpdate(m.lastUpdate)
	m.statusBar.SetHoveredButton(m.hoveredButton)

	// Sync issues panel
	m.issuesPanel.SetSize(issuesWidth, m.height)
	m.issuesPanel.SetFocus(m.activePanel == PanelLeft)
	m.issuesPanel.SetData(
		m.beadItems,
		m.beadsCursor,
		m.filters,
		m.beadsExpanded,
		m.selectedBeads,
		m.activeBeadSessions,
		m.newBeads,
	)
	m.issuesPanel.SetWorkContext(m.focusedWorkID)
	m.issuesPanel.SetHoveredIssue(m.hoveredIssue)

	// Sync details panel
	m.detailsPanel.SetSize(detailsWidth, m.height)
	m.detailsPanel.SetFocus(m.activePanel == PanelRight)
	// Get focused bead and build child lookup map
	var focusedBead *beadItem
	var hasActiveSession bool
	childBeadMap := make(map[string]*beadItem)
	if len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
		focusedBead = &m.beadItems[m.beadsCursor]
		hasActiveSession = m.activeBeadSessions[focusedBead.ID]
		// Build map for child lookup
		for i := range m.beadItems {
			childBeadMap[m.beadItems[i].ID] = &m.beadItems[i]
		}
	}
	m.detailsPanel.SetData(focusedBead, hasActiveSession, childBeadMap)

	// Sync work tabs bar
	m.workTabsBar.SetSize(m.width)
	// Note: Work tiles are set asynchronously when work tiles are loaded
	m.workTabsBar.SetFocusedWorkID(m.focusedWorkID)
	// Note: Orchestrator health is set asynchronously when work tiles are loaded

	// Sync Linear import panel
	m.linearImportPanel.SetSize(detailsWidth, m.height)
	m.linearImportPanel.SetFocus(m.activePanel == PanelRight && m.viewMode == ViewLinearImportInline)
	m.linearImportPanel.SetHoveredButton(m.hoveredDialogButton)

	// Sync PR import panel
	m.prImportPanel.SetSize(detailsWidth, m.height)
	m.prImportPanel.SetFocus(m.activePanel == PanelRight && m.viewMode == ViewPRImportInline)
	m.prImportPanel.SetHoveredButton(m.hoveredDialogButton)

	// Sync bead form panel
	m.beadFormPanel.SetSize(detailsWidth, m.height)
	m.beadFormPanel.SetFocus(m.activePanel == PanelRight)
	m.beadFormPanel.SetHoveredButton(m.hoveredDialogButton)

	// Sync create work panel
	m.createWorkPanel.SetSize(detailsWidth, m.height)
	m.createWorkPanel.SetFocus(m.activePanel == PanelRight && m.viewMode == ViewCreateWork)
	m.createWorkPanel.SetHoveredButton(m.hoveredDialogButton)
}

// View implements tea.Model
func (m *planModel) View() string {
	// Handle dialogs
	switch m.viewMode {
	case ViewCreateBead, ViewCreateBeadInline, ViewAddChildBead, ViewEditBead:
		// All bead form modes render inline in the details panel
		// Fall through to normal rendering
	case ViewCreateWork:
		// Create work now renders inline in the details panel
		// Fall through to normal rendering
	case ViewBeadSearch:
		// Inline search mode - render normal view with search bar in status area
		// Fall through to normal rendering
	case ViewLabelFilter:
		return m.renderWithDialog(m.renderLabelFilterDialogContent())
	case ViewCloseBeadConfirm:
		return m.renderWithDialog(m.renderCloseBeadConfirmContent())
	case ViewDeleteBeadConfirm:
		return m.renderWithDialog(m.renderDeleteBeadConfirmContent())
	case ViewDestroyConfirm:
		return m.renderWithDialog(m.renderDestroyConfirmContent())
	case ViewLinearImportInline:
		// Inline import mode - render normal view with import form in details area
		// Fall through to normal rendering
	case ViewPRImportInline:
		// Inline PR import mode - render normal view with import form in details area
		// Fall through to normal rendering
	case ViewHelp:
		return m.renderHelp()
	case ViewLinearNotConfigured:
		m.linearConfigPanel.SetSize(m.width, m.height)
		return m.linearConfigPanel.Render()
	case ViewToolMissing:
		return splash.RenderInfoBox(m.width, m.height, m.infoBoxTitle, m.infoBoxBody, m.splashConfig)
	}

	// Show splash screen (full-screen, no tabs) when project is truly empty
	if m.shouldShowSplash() {
		return splash.RenderWithPrompt(m.width, m.height, splash.PromptConfig{
			Config:    m.splashConfig,
			InputView: m.promptInput.View(),
			Timeline:  m.promptTimeline,
			Busy:      m.promptAgentRunning,
			Focused:   m.activePanel != PanelViewTabs && m.activePanel != PanelWorkHeader,
		})
	}

	// Render work tabs bar (always visible)
	workTabsBar := m.workTabsBar.Render()
	tabsBarHeight := m.workTabsBar.Height()

	// Render view tabs bar (Prompt | Issues) — bubbletea tabs pattern (3 lines)
	viewTabsBar := m.renderViewTabs()
	viewTabsHeight := lipgloss.Height(viewTabsBar)

	// Total chrome height above content
	chromeHeight := tabsBarHeight + viewTabsHeight

	if m.topView == "prompt" {
		// Prompt view: render chat UI in the content area
		promptHeight := m.height - chromeHeight
		promptContent := splash.RenderWithPrompt(m.width, promptHeight, splash.PromptConfig{
			Config:    m.splashConfig,
			InputView: m.promptInput.View(),
			Timeline:  m.promptTimeline,
			Busy:      m.promptAgentRunning,
			Compact:   true, // Inside tab, not full splash — halve top margin
			Focused:   m.activePanel != PanelViewTabs && m.activePanel != PanelWorkHeader,
		})
		return lipgloss.JoinVertical(lipgloss.Left, workTabsBar, viewTabsBar, promptContent)
	}

	// Issues view: render normal two-column layout
	originalHeight := m.height

	// Work context panel (shown above issues when a work filter is active)
	var workContextPanel string
	workContextHeight := 0
	if m.focusedWorkID != "" {
		workContextPanel = m.renderWorkContextPanel()
		if workContextPanel != "" {
			workContextHeight = lipgloss.Height(workContextPanel)
		}
	}

	m.height = m.height - chromeHeight - workContextHeight
	m.syncPanels() // Sync all panels including status bar before rendering
	content := m.renderTwoColumnLayout()
	m.height = originalHeight

	// Render status bar AFTER syncPanels to ensure status message is set
	statusBar := m.statusBar.Render()

	if workContextPanel != "" {
		return lipgloss.JoinVertical(lipgloss.Left, workTabsBar, viewTabsBar, workContextPanel, content, statusBar)
	}
	return lipgloss.JoinVertical(lipgloss.Left, workTabsBar, viewTabsBar, content, statusBar)
}

// beadsForBranch is a minimal struct for branch name generation
type beadsForBranch struct {
	ID    string
	Title string
}

// generateBranchNameFromBeadsForBranch generates a branch name from beads
func generateBranchNameFromBeadsForBranch(beads []*beadsForBranch) string {
	if len(beads) == 0 {
		return ""
	}
	// Use the same logic as generateBranchNameFromBeads but with local struct
	var titles []string
	for _, b := range beads {
		titles = append(titles, b.Title)
	}
	combined := strings.Join(titles, " ")
	// Sanitize for branch name
	combined = strings.ToLower(combined)
	combined = strings.ReplaceAll(combined, " ", "-")
	// Remove special characters
	var result strings.Builder
	for _, c := range combined {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result.WriteRune(c)
		}
	}
	branchName := result.String()
	// Limit length
	branchName = ansi.Truncate(branchName, 50, "")
	// Remove trailing dashes
	branchName = strings.TrimRight(branchName, "-")
	return "feat/" + branchName
}

// updateWorkSelectionFilter updates the bead filter based on the current work details selection
// and triggers a data refresh
func (m *planModel) updateWorkSelectionFilter() tea.Cmd {
	// Save old filter values to detect actual changes
	oldChildren := m.filters.children
	oldRootIssue := m.filters.rootIssue

	// Clear existing entity filters
	m.filters.task = ""
	m.filters.children = ""
	m.filters.rootIssue = ""

	if m.focusedWorkID == "" {
		return nil
	}

	focusedWork := m.findWorkByID(m.focusedWorkID)
	if focusedWork == nil {
		// Work data is still loading - restore old filters to avoid
		// clearing the issues panel while async data loads
		m.filters.task = oldTask
		m.filters.children = oldChildren
		m.filters.rootIssue = oldRootIssue
		return nil
	}

	// Set children filter to show the root issue and its dependents
	if focusedWork.Work.RootIssueID != "" {
		m.filters.rootIssue = focusedWork.Work.RootIssueID
		m.filters.children = focusedWork.Work.RootIssueID
	}

	// Only reset cursor when filter actually changes (not on every refresh)
	if m.filters.children != oldChildren || m.filters.rootIssue != oldRootIssue {
		m.beadsCursor = 0
	}

	return m.refreshData()
}

// selectWorkByIndex selects a work by its index in the work tiles array.
// Key mapping: 1-9 map to indices 0-8.
// Returns a command to load work tiles if they're not loaded yet.
func (m *planModel) selectWorkByIndex(digit int) (tea.Model, tea.Cmd) {
	// Map digit to index: 1->0, 2->1, ..., 9->8
	index := digit - 1

	works := m.workTiles

	// If no works loaded yet, load them first and store pending selection
	if len(works) == 0 {
		m.loading = true
		m.pendingWorkSelectIndex = index
		return m, m.loadWorkTiles()
	}

	return m.doSelectWorkAtIndex(index)
}

// doSelectWorkAtIndex performs the actual work selection at a given index.
// This is called either directly from selectWorkByIndex or after work tiles are loaded.
func (m *planModel) doSelectWorkAtIndex(index int) (tea.Model, tea.Cmd) {
	works := m.workTiles

	// Check if index is valid
	if index >= len(works) {
		m.statusMessage = fmt.Sprintf("No work at position %d (have %d works)", index+1, len(works))
		m.statusIsError = true
		return m, nil
	}

	work := works[index]
	if work == nil {
		return m, nil
	}

	// Toggle: pressing same number clears the filter
	if m.focusedWorkID == work.Work.ID {
		m.focusedWorkID = ""
		m.filters.task = ""
		m.filters.children = ""
		m.filters.rootIssue = ""
		m.workSelectionCleared = false
		m.activePanel = PanelLeft
		m.statusMessage = "Showing all issues"
		m.statusIsError = false
		return m, m.refreshData()
	}

	// Select the work as a filter
	m.focusedWorkID = work.Work.ID
	m.workSelectionCleared = false
	m.viewMode = ViewNormal
	m.activePanel = PanelLeft
	m.statusMessage = fmt.Sprintf("Filtered to work %s", m.focusedWorkID)
	m.statusIsError = false

	// Clear unseen PR changes flag for this work
	_ = m.proj.DB.MarkWorkPRSeen(m.ctx, m.focusedWorkID)

	// Update the filter and refresh
	return m, m.updateWorkSelectionFilter()
}

// findWorkByID finds a work by its ID in the cached work tiles.
// Returns nil if not found.
func (m *planModel) findWorkByID(id string) *progress.WorkProgress {
	for _, work := range m.workTiles {
		if work != nil && work.Work.ID == id {
			return work
		}
	}
	return nil
}

// buildSplashConfig creates a splash.Config from the current theme and tool checks.
func buildSplashConfig() splash.Config {
	t := CurrentTheme()
	return splash.Config{
		Gradient: t.SplashGradient,
		Accent:   t.Accent,
		Error:    t.Error,
		Warning:  t.Warning,
		Dim:      t.Dim,
		Text:     t.Text,
		Border:   t.DialogBorder,
		HasBd:    isToolAvailable("bd"),
		HasGh:    isToolAvailable("gh"),
		Platform: splash.DetectPlatform(),
	}
}

// isToolAvailable checks if a CLI tool is on PATH.
func isToolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// shouldShowSplash returns true when the splash screen should be shown:
// no issues, no works, not loading, and initial data fetch completed.
func (m *planModel) shouldShowSplash() bool {
	return m.viewMode == ViewNormal &&
		len(m.beadItems) == 0 &&
		len(m.workTiles) == 0 &&
		!m.loading &&
		!m.lastUpdate.IsZero()
}

// handleSplashPromptKey handles keyboard input when the splash screen with prompt is visible.
func (m *planModel) handleSplashPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tl := m.promptTimeline

	// Block all input while the prompt agent is running
	if m.promptAgentRunning {
		// Only allow browsing existing messages and quitting
		switch msg.Type {
		case tea.KeyUp:
			if tl.MessageCount() > 0 {
				tl.SelectUp()
			}
			return m, nil
		case tea.KeyDown:
			if tl.SelectedIdx() >= 0 {
				tl.SelectDown()
			}
			return m, nil
		case tea.KeyEsc:
			if tl.SelectedIdx() >= 0 {
				tl.ClearSelection()
			}
			return m, nil
		}
		return m, nil
	}

	if tl.SelectedIdx() >= 0 {
		// A message is selected — navigate messages or return to input
		switch msg.Type {
		case tea.KeyUp:
			if tl.SelectedIdx() == 0 {
				// Already at the top message — navigate to view tabs
				tl.ClearSelection()
				m.activePanel = PanelViewTabs
				m.viewTabsCursor = 0 // Prompt tab
				m.promptInput.Blur()
				return m, nil
			}
			tl.SelectUp()
			return m, nil
		case tea.KeyDown:
			if tl.SelectDown() {
				// Past last message — move back to input
				tl.ClearSelection()
				m.promptInput.Focus()
			}
			return m, nil
		case tea.KeyTab:
			// Suspend selection and focus prompt (remembers position)
			tl.SuspendSelection()
			m.promptInput.Focus()
			return m, nil
		case tea.KeyEsc:
			tl.ClearSelection()
			m.promptInput.Focus()
			return m, nil
		case tea.KeyEnter:
			// Enter on a selected message — navigate to linked entity (same as 'o')
			if selected := tl.SelectedMessage(); selected != nil {
				if cmd := m.navigateToLinkedEntity(selected); cmd != nil {
					return m, cmd
				}
			}
			return m, nil
		case tea.KeyRunes:
			switch msg.String() {
			case "o":
				// Open — navigate to the linked work/task/bead
				if selected := tl.SelectedMessage(); selected != nil {
					if cmd := m.navigateToLinkedEntity(selected); cmd != nil {
						return m, cmd
					}
				}
				return m, nil
			case "d":
				// Delete selected message
				if id := tl.DeleteSelected(); id > 0 {
					ctx := m.ctx
					db := m.proj.DB
					return m, func() tea.Msg {
						_ = messages.Delete(ctx, db, id)
						return nil
					}
				}
				return m, nil
			}
		}
		// Other keys ignored when message is selected
		return m, nil
	}

	// Input is focused — handle text input
	switch msg.Type {
	case tea.KeyUp:
		if tl.MessageCount() > 0 {
			tl.SelectUp()
			m.promptInput.Blur()
		}
		return m, nil
	case tea.KeyShiftTab:
		// Navigate up to view tabs bar
		m.activePanel = PanelViewTabs
		m.viewTabsCursor = 0 // Prompt tab
		m.promptInput.Blur()
		return m, nil
	case tea.KeyEnter:
		text := strings.TrimSpace(m.promptInput.Value())
		if text == "" {
			return m, nil
		}
		m.promptInput.SetValue("")

		// Submitting clears any remembered scroll position
		tl.ResetSuspended()

		// Write user message to DB and show it immediately
		_ = messages.Write(m.ctx, m.proj.DB, messages.WriteParams{
			Source:    messages.SourceUser,
			Text:      text,
			EventType: messages.EventPrompt,
		})
		tl.AppendMessage(ui.ChatMessage{
			Source: ui.MessageFromUser,
			Text:   text,
			Time:   "just now",
		})

		// Block input and show thinking state
		m.promptAgentRunning = true
		m.promptInput.Blur()

		// Spawn headless Claude agent (non-blocking)
		return m, m.runPromptAgent(text)
	case tea.KeyEsc:
		if m.promptInput.Value() == "" {
			return m, nil
		}
		m.promptInput.SetValue("")
		return m, nil
	}

	// Delegate to textinput for character input
	var cmd tea.Cmd
	m.promptInput, cmd = m.promptInput.Update(msg)
	return m, cmd
}

// loadMessages reads messages from the DB and returns them as ChatMessages.
func (m *planModel) loadMessages() tea.Cmd {
	return func() tea.Msg {
		msgs, err := messages.List(m.ctx, m.proj.DB, 50)
		if err != nil {
			return messagesLoadedMsg{err: err}
		}
		chatMsgs := make([]ui.ChatMessage, len(msgs))
		now := time.Now()
		for i, msg := range msgs {
			source := ui.MessageFromSystem
			if msg.Source == messages.SourceUser {
				source = ui.MessageFromUser
			}
			chatMsgs[i] = ui.ChatMessage{
				ID:        msg.ID,
				Source:    source,
				Text:      msg.Text,
				Time:      relativeTime(now, msg.CreatedAt),
				WorkID:    msg.WorkID,
				TaskID:    msg.TaskID,
				BeadID:    msg.BeadID,
				EventType: msg.EventType,
			}
		}
		return messagesLoadedMsg{messages: chatMsgs}
	}
}

// relativeTime returns a human-readable relative timestamp.
func relativeTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

// navigateToLinkedEntity navigates to the work/bead referenced by a chat message.
// Returns a tea.Cmd if navigation happened, nil otherwise.
func (m *planModel) navigateToLinkedEntity(msg *ui.ChatMessage) tea.Cmd {
	// Switch to issues view when navigating from prompt
	m.topView = "issues"
	m.promptInput.Blur()

	if msg.WorkID != "" {
		// Find work in tiles and focus it
		for i, w := range m.workTiles {
			if w != nil && w.Work.ID == msg.WorkID {
				_, cmd := m.doSelectWorkAtIndex(i)
				m.promptTimeline.ClearSelection()
				return cmd
			}
		}
		// Work not loaded yet — try loading tiles then selecting
		m.statusMessage = fmt.Sprintf("Navigating to work %s...", msg.WorkID)
		m.statusIsError = false
		return nil
	}

	if msg.BeadID != "" {
		// Find bead in beadItems and focus it
		for i, b := range m.beadItems {
			if b.ID == msg.BeadID {
				m.beadsCursor = i
				m.activePanel = PanelLeft
				m.promptTimeline.ClearSelection()
				return nil
			}
		}
		// Bead not in current view — clear filters and reload
		m.filters.status = "all"
		m.promptTimeline.ClearSelection()
		m.statusMessage = fmt.Sprintf("Looking for %s...", msg.BeadID)
		m.statusIsError = false
		return m.refreshData()
	}

	return nil
}

// runPromptAgent spawns a headless Claude agent to process the user's prompt.
func (m *planModel) runPromptAgent(text string) tea.Cmd {
	return func() tea.Msg {
		// Fetch recent messages for conversation context
		recentMsgs, _ := messages.List(m.ctx, m.proj.DB, 50)
		var historyLines []string
		// Take the last 10 prompt-context messages (skip the one we just wrote)
		start := 0
		if len(recentMsgs) > 10 {
			start = len(recentMsgs) - 10
		}
		for _, msg := range recentMsgs[start:] {
			// Only include prompt-context messages (user prompts and system responses)
			if msg.EventType != "" && msg.EventType != messages.EventPrompt &&
				msg.EventType != messages.EventError && msg.EventType != messages.EventInfo {
				continue
			}
			role := "sarge"
			if msg.Source == messages.SourceUser {
				role = "user"
			}
			historyLines = append(historyLines, fmt.Sprintf("%s: %s", role, msg.Text))
		}
		recentHistory := strings.Join(historyLines, "\n")

		agent := claude.New()
		prompt, err := agent.BuildPrompt(types.TaskParams{
			Type:           types.TaskTypePrompt,
			UserPrompt:     text,
			ProjectDir:     m.proj.Root,
			RecentMessages: recentHistory,
		})
		if err != nil {
			// Write fallback error message
			_ = messages.Write(m.ctx, m.proj.DB, messages.WriteParams{
				Source:    messages.SourceSystem,
				Text:      fmt.Sprintf("Failed to build prompt: %v", err),
				EventType: messages.EventError,
			})
			return promptAgentDoneMsg{err: err}
		}

		args := []string{"--print"}
		if m.proj.Config != nil && m.proj.Config.Claude.ShouldSkipPermissions() {
			args = append([]string{"--dangerously-skip-permissions"}, args...)
		}
		args = append(args, prompt)

		cmd := exec.CommandContext(m.ctx, "claude", args...)
		cmd.Dir = m.proj.MainRepoPath()

		// Set BEADS_DIR so bd commands work
		cmd.Env = append(os.Environ(), "BEADS_DIR="+m.proj.BeadsPath())

		output, err := cmd.CombinedOutput()
		if err != nil {
			// Write fallback error message if claude failed
			errText := fmt.Sprintf("Agent error: %v", err)
			if len(output) > 0 {
				// Trim to reasonable length for chat bubble
				outStr := strings.TrimSpace(string(output))
				if len(outStr) > 200 {
					outStr = outStr[:200] + "..."
				}
				errText = outStr
			}
			_ = messages.Write(m.ctx, m.proj.DB, messages.WriteParams{
				Source:    messages.SourceSystem,
				Text:      errText,
				EventType: messages.EventError,
			})
			return promptAgentDoneMsg{err: err}
		}

		return promptAgentDoneMsg{}
	}
}

// buildChatBubbleColors creates ChatBubbleColors from the current theme.
func buildChatBubbleColors() ui.ChatBubbleColors {
	t := CurrentTheme()
	return ui.ChatBubbleColors{
		UserBg:       t.ChatUserBg,
		UserFg:       t.ChatUserFg,
		SystemBorder: t.ChatSystemBrd,
		SystemFg:     t.ChatSystemFg,
		TimeDim:      t.Dim,
		SelectedBg:   t.Accent,
	}
}
