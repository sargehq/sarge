package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sargehq/sarge/internal/beads"
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
	workDetails       *WorkDetailsPanel
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
	workDetailsFocusLeft   bool            // Whether left panel has focus in work details (true=left, false=right)
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
	hoveredWorkItem     int       // index of hovered work detail item, -1 if none
	hoveredDialogButton string    // which dialog button is hovered ("ok", "cancel")
	hoveredTabID        string    // which work tab is hovered

	// Database watcher for cache invalidation
	beadsWatcher    *beadswatcher.Watcher
	trackingWatcher *trackingwatcher.Watcher

	// New bead animation tracking
	newBeads map[string]time.Time // beadID -> creation timestamp for animation
}

// newPlanModel creates a new Plan Mode model
func newPlanModel(ctx context.Context, proj *project.Project) *planModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

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
		hoveredWorkItem:        -1,   // No work item hovered initially
		pendingWorkSelectIndex: -1,   // No pending work selection
		workDetailsFocusLeft:   true, // Start with left panel focused
		beadsWatcher:           beadsWatcher,
		trackingWatcher:        trackingWatcher,
		filters: beadFilters{
			status: "open",
			sortBy: "default",
		},
	}

	// Initialize panels
	m.statusBar = NewStatusBar()
	m.issuesPanel = NewIssuesPanel()
	m.detailsPanel = NewIssueDetailsPanel()
	m.workDetails = NewWorkDetailsPanel()
	m.workTabsBar = NewWorkTabsBar()
	m.linearImportPanel = NewLinearImportPanel()
	m.prImportPanel = NewPRImportPanel()
	m.beadFormPanel = NewBeadFormPanel()
	m.createWorkPanel = NewCreateWorkPanel()

	// Set up status bar data providers
	m.statusBar.SetDataProviders(
		func() []beadItem { return m.beadItems },
		func() int { return m.beadsCursor },
		func() map[string]bool { return m.activeBeadSessions },
		func() ViewMode { return m.viewMode },
		func() string { return m.textInput.View() },
	)

	// Set up the failed task selected provider for work detail context
	m.statusBar.SetFailedTaskSelectedProvider(func() bool {
		return m.workDetails.IsSelectedTaskFailed()
	})

	return m
}

// SetSize implements SubModel
func (m *planModel) SetSize(width, height int) {
	m.width = width
	m.height = height
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

// Init implements tea.Model
func (m *planModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.workTabsBar.GetSpinner().Tick, // Tick the tabs bar spinner
		m.refreshData(),
		m.loadWorkTiles(), // Load work tiles for the tabs bar
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
			// Tracking database changed - reload work tiles and work details
			// This is more targeted than a full refresh
			return m, tea.Batch(m.loadWorkTiles(), m.waitForTrackingWatcherEvent())
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
			// Calculate tabs bar position (at top, if there are works)
			tabsBarHeight := m.workTabsBar.Height()

			// Check if hovering over tabs bar
			if tabsBarHeight > 0 && msg.Y < tabsBarHeight {
				m.hoveredTabID = m.workTabsBar.DetectHoveredTab(msg)
				m.workTabsBar.SetHoveredTabID(m.hoveredTabID)
				m.hoveredButton = ""
				m.hoveredIssue = -1
				m.hoveredWorkItem = -1
				m.hoveredDialogButton = ""
				return m, nil
			}

			// Clear tab hover when not over tabs bar
			m.hoveredTabID = ""
			m.workTabsBar.SetHoveredTabID("")

			if msg.Y == statusBarY {
				m.hoveredButton = m.detectCommandsBarButton(msg)
				m.hoveredIssue = -1
				m.hoveredWorkItem = -1
				m.hoveredDialogButton = ""
			} else {
				m.hoveredButton = ""
				// Detect hover over dialog buttons if in form mode
				m.hoveredDialogButton = m.detectDialogButton(msg)
				if m.hoveredDialogButton != "" {
					m.hoveredIssue = -1
					m.hoveredWorkItem = -1
				} else if m.focusedWorkID != "" {
					// Focused work mode: work details panel at top, issues panel at bottom
					// Mouse could be in work details or issues - detect with bubblezone
					m.hoveredWorkItem = m.workDetails.DetectHoveredItem(msg)
					if m.hoveredWorkItem >= 0 {
						m.hoveredIssue = -1
					} else {
						// Check issues panel
						m.hoveredIssue = m.detectHoveredIssueWithOffset(msg)
					}
				} else {
					// Normal mode - detect hover over issue lines
					m.hoveredWorkItem = -1
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
			// Check for clicks on tabs bar
			tabsBarHeight := m.workTabsBar.Height()
			if tabsBarHeight > 0 && msg.Y < tabsBarHeight {
				// Set focus to work tabs panel when clicking on it
				m.activePanel = PanelWorkTabs

				clickedWorkID := m.workTabsBar.HandleClick(msg)
				if clickedWorkID != "" {
					// Focus the clicked work
					if m.focusedWorkID == clickedWorkID {
						// Already focused - unfocus
						m.focusedWorkID = ""
						m.filters.task = "" // Clear work selection filter
						m.filters.children = ""
						m.activePanel = PanelLeft
						m.statusMessage = "Work deselected"
						m.statusIsError = false
						return m, m.refreshData()
					}
					// Focus the new work
					m.focusedWorkID = clickedWorkID
					m.viewMode = ViewNormal
					// Focus the work details panel
					m.activePanel = PanelWorkDetails
					m.statusMessage = fmt.Sprintf("Focused on work %s", m.focusedWorkID)
					m.statusIsError = false

					// Clear unseen PR changes flag for this work
					_ = m.proj.DB.MarkWorkPRSeen(m.ctx, clickedWorkID)

					// Set up the work details panel
					focusedWork := m.findWorkByID(m.focusedWorkID)
					m.workDetails.SetFocusedWork(focusedWork)
					m.workDetails.SetSelectedIndex(0)
					m.workDetails.SetOrchestratorHealth(checkOrchestratorHealth(m.ctx, m.proj.DB, m.focusedWorkID))

					return m, m.updateWorkSelectionFilter()
				}
				return m, nil
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

				// Handle panel clicking in focused work mode
				if m.focusedWorkID != "" {
					clickedPanel := m.detectClickedPanel(msg)
					switch clickedPanel {
					case "work-left":
						// Check if clicking on a task or root issue using bubblezone
						clickedItem := m.workDetails.DetectClickedItem(msg)
						if clickedItem >= 0 {
							m.workDetails.SetSelectedIndex(clickedItem)
							m.activePanel = PanelWorkDetails
							// Update filter to show beads for clicked item
							return m, m.updateWorkSelectionFilter()
						}
						m.activePanel = PanelWorkDetails
						return m, nil
					case "work-right":
						m.activePanel = PanelWorkDetails
						return m, nil
					case "issues-left":
						// Check if clicking on an issue
						clickedIssue := m.detectHoveredIssue(msg)
						if clickedIssue >= 0 && clickedIssue < len(m.beadItems) {
							m.beadsCursor = clickedIssue
						}
						m.activePanel = PanelLeft
						return m, nil
					case "issues-right":
						m.activePanel = PanelRight
						return m, nil
					}
				} else {
					// Normal mode - just check for issue clicks
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

		m.beadItems = msg.beads
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

		// Update work details panel and filter if a work is focused
		if m.focusedWorkID != "" {
			focusedWork := m.findWorkByID(m.focusedWorkID)
			m.workDetails.SetFocusedWork(focusedWork)
			// Use pre-computed orchestrator health
			if health, ok := msg.orchestratorHealth[m.focusedWorkID]; ok {
				m.workDetails.SetOrchestratorHealth(health)
			}
			// Rebuild the filter to reflect any changes in work beads
			// BUT skip if user manually cleared the filter (e.g., pressed '*')
			if !m.workSelectionCleared {
				return m, m.updateWorkSelectionFilter()
			}
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
	// Handle escape key globally for deselecting focused work
	if msg.Type == tea.KeyEsc && m.viewMode == ViewNormal && m.focusedWorkID != "" {
		m.focusedWorkID = ""
		m.filters.task = "" // Clear work selection filter
		m.filters.children = ""
		m.activePanel = PanelLeft // Reset focus to issues panel
		m.statusMessage = "Work deselected"
		m.statusIsError = false
		// Refresh to show all issues again
		return m, m.refreshData()
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
	}

	// Normal mode key handling

	// Delegate to work tabs panel when it's active
	if m.activePanel == PanelWorkTabs && len(m.workTiles) > 0 {
		// Handle navigation in work tabs
		switch msg.String() {
		case "h", "left":
			// Move to previous work tab
			currentIndex := -1
			for i, work := range m.workTiles {
				if work != nil && work.Work.ID == m.focusedWorkID {
					currentIndex = i
					break
				}
			}
			if currentIndex > 0 {
				// Select previous work
				return m.doSelectWorkAtIndex(currentIndex - 1)
			}
			return m, nil

		case "l", "right":
			// Move to next work tab
			currentIndex := -1
			for i, work := range m.workTiles {
				if work != nil && work.Work.ID == m.focusedWorkID {
					currentIndex = i
					break
				}
			}
			if currentIndex >= 0 && currentIndex < len(m.workTiles)-1 {
				// Select next work
				return m.doSelectWorkAtIndex(currentIndex + 1)
			}
			return m, nil

		case "enter":
			// If a work is focused but we're on the tabs bar, ensure we switch to work details
			if m.focusedWorkID != "" {
				m.activePanel = PanelWorkDetails
			}
			return m, nil
		}
	}

	// Delegate to work details panel when it's active
	if m.activePanel == PanelWorkDetails && m.focusedWorkID != "" {
		cmd, action := m.workDetails.Update(msg)
		switch action {
		case WorkDetailActionNavigateUp, WorkDetailActionNavigateDown:
			// Navigation actions - check if selection changed and update filter
			return m, m.updateWorkSelectionFilter()
		case WorkDetailActionOpenTerminal:
			return m, m.openConsole()
		case WorkDetailActionOpenClaude:
			return m, m.openClaude()
		case WorkDetailActionOpenIDE:
			return m, m.openIDE()
		case WorkDetailActionRun:
			// Run work - use auto-group if multiple unassigned beads
			focusedWork := m.workDetails.GetFocusedWork()
			useAutoGroup := focusedWork != nil && len(focusedWork.UnassignedBeads) > 1
			return m, m.runFocusedWork(useAutoGroup)
		case WorkDetailActionReview:
			return m, m.createReviewTask()
		case WorkDetailActionPR:
			return m, m.createPRTask()
		case WorkDetailActionRestartOrchestrator:
			return m, m.restartOrchestrator()
		case WorkDetailActionCheckFeedback:
			return m, m.checkPRFeedback()
		case WorkDetailActionDestroy:
			// Show confirmation dialog for work destruction
			// Check if work is currently processing
			focusedWork := m.workDetails.GetFocusedWork()
			if focusedWork != nil && focusedWork.Work.Status == "processing" {
				m.statusMessage = "Cannot destroy work that is currently processing"
				m.statusIsError = true
				return m, nil
			}
			m.viewMode = ViewDestroyConfirm
			return m, cmd
		case WorkDetailActionAddChildIssue:
			// Add child issue to root issue, then add to work and run
			focusedWork := m.workDetails.GetFocusedWork()
			if focusedWork != nil && focusedWork.Work.RootIssueID != "" {
				m.addChildToWorkID = focusedWork.Work.ID
				m.beadFormPanel.SetAddChildMode(focusedWork.Work.RootIssueID)
				m.viewMode = ViewAddChildBead
				return m, m.beadFormPanel.Init()
			}
			return m, nil
		case WorkDetailActionResetTask:
			return m, m.resetSelectedTask()
		case WorkDetailActionPlan:
			// Start planning session for selected unassigned bead
			beadID := m.workDetails.GetSelectedUnassignedBeadID()
			if beadID != "" {
				return m, m.spawnPlanSession(beadID)
			}
			return m, nil
		}
		// WorkDetailActionNone - fall through to normal handling
	}

	// Handle [1-9] keys to select work by index (works from issues panel and work details panel)
	if key := msg.String(); len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		digit := int(key[0] - '0')
		return m.selectWorkByIndex(digit)
	}

	switch msg.String() {
	case "tab":
		// In focused work mode: cycle between work details (left panel only) and issues
		// Tab does NOT navigate to work tabs bar or the right panel of work details
		if m.focusedWorkID != "" {
			switch m.activePanel {
			case PanelWorkDetails, PanelWorkTabs:
				// Move from work details (or tabs) to issues panel
				m.activePanel = PanelLeft
			case PanelLeft:
				// Move from issues to work details
				m.activePanel = PanelWorkDetails
				m.workDetailsFocusLeft = true // Always focus left panel
				// Reset the cleared flag and restore work selection filter when entering work details
				if m.workSelectionCleared {
					m.workSelectionCleared = false
					return m, m.updateWorkSelectionFilter()
				}
			default:
				m.activePanel = PanelWorkDetails
				m.workDetailsFocusLeft = true
			}
		}
		return m, nil

	case "shift+tab":
		// In focused work mode: cycle backward between issues and work details (left panel only)
		// Shift+Tab does NOT navigate to work tabs bar or the right panel of work details
		if m.focusedWorkID != "" {
			switch m.activePanel {
			case PanelLeft:
				// Move from issues to work details
				m.activePanel = PanelWorkDetails
				m.workDetailsFocusLeft = true // Always focus left panel
				// Reset the cleared flag and restore work selection filter when entering work details
				if m.workSelectionCleared {
					m.workSelectionCleared = false
					return m, m.updateWorkSelectionFilter()
				}
			case PanelWorkDetails, PanelWorkTabs:
				// Move from work details (or tabs) to issues panel
				m.activePanel = PanelLeft
			default:
				m.activePanel = PanelLeft
			}
		}
		return m, nil

	case "h", "left":
		// Simple left navigation in panels
		if m.activePanel == PanelRight {
			m.activePanel = PanelLeft
		}
		return m, nil

	case "l", "right":
		// Simple right navigation in panels
		if m.activePanel == PanelLeft {
			m.activePanel = PanelRight
		}
		return m, nil

	case "j", "down":
		// Navigate down in current list (work details is handled above)
		if m.beadsCursor < len(m.beadItems)-1 {
			m.beadsCursor++
		}
		return m, nil

	case "k", "up":
		// Navigate up in current list (work details is handled above)
		if m.beadsCursor > 0 {
			m.beadsCursor--
		}
		return m, nil

	case "n":
		// Create new bead inline
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
		// Filter to closed issues (work details panel handles 'c' for Claude)
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
			m.statusMessage = "Linear API key not configured (set [linear] api_key in config.toml)"
			m.statusIsError = true
			return m, nil
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

	// Determine status bar context based on focused panel
	var statusBarCtx StatusBarContext
	switch m.activePanel {
	case PanelWorkDetails:
		statusBarCtx = StatusBarContextWorkDetail
	default:
		statusBarCtx = StatusBarContextIssues
	}

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
	m.workTabsBar.SetActivePanel(m.activePanel)
	// Note: Work tiles are set asynchronously when work tiles are loaded
	m.workTabsBar.SetFocusedWorkID(m.focusedWorkID)
	// Note: Orchestrator health is set asynchronously when work tiles are loaded

	// Sync work details (for focused work split view)
	if m.focusedWorkID != "" {
		// Calculate the correct work panel height (same formula as renderFocusedWorkSplitView)
		workPanelHeight := m.calculateWorkPanelHeight() + 2 // +2 for border
		m.workDetails.SetSize(m.width, workPanelHeight)
		m.workDetails.SetColumnRatio(m.columnRatio) // Use same ratio as issues panel
		// Pass focus state based on whether work details panel is active and which sub-panel has focus
		leftFocused := m.activePanel == PanelWorkDetails && m.workDetailsFocusLeft
		rightFocused := m.activePanel == PanelWorkDetails && !m.workDetailsFocusLeft
		m.workDetails.SetFocus(leftFocused, rightFocused)
		focusedWork := m.findWorkByID(m.focusedWorkID)
		m.workDetails.SetFocusedWork(focusedWork)
		m.workDetails.SetHoveredItem(m.hoveredWorkItem)
	}

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
	}

	// Render work tabs bar (always visible)
	workTabsBar := m.workTabsBar.Render()
	tabsBarHeight := m.workTabsBar.Height()

	// Adjust content height for tabs bar
	originalHeight := m.height
	m.height = m.height - tabsBarHeight
	m.syncPanels() // Sync all panels including status bar before rendering
	content := m.renderTwoColumnLayout()
	m.height = originalHeight

	// Render status bar AFTER syncPanels to ensure status message is set
	statusBar := m.statusBar.Render()

	// Always include tab bar at top
	return lipgloss.JoinVertical(lipgloss.Left, workTabsBar, content, statusBar)
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
	oldTask := m.filters.task
	oldChildren := m.filters.children
	oldRootIssue := m.filters.rootIssue

	// Clear existing entity filters
	m.filters.task = ""
	m.filters.children = ""
	m.filters.rootIssue = ""

	if m.focusedWorkID == "" {
		return nil
	}

	focusedWork := m.workDetails.GetFocusedWork()
	if focusedWork == nil {
		return nil
	}

	// Always set root issue when work panel is present so it's always visible
	if focusedWork.Work.RootIssueID != "" {
		m.filters.rootIssue = focusedWork.Work.RootIssueID
	}

	if m.workDetails.IsTaskSelected() {
		// Task selected - set task filter to show beads assigned to that task
		selectedTaskID := m.workDetails.GetSelectedTaskID()
		if selectedTaskID != "" {
			m.filters.task = selectedTaskID
		}
	} else {
		// Root issue selected - set children filter to show dependents
		if focusedWork.Work.RootIssueID != "" {
			m.filters.children = focusedWork.Work.RootIssueID
		}
	}

	// Only reset cursor when filter actually changes (not on every refresh)
	if m.filters.task != oldTask || m.filters.children != oldChildren || m.filters.rootIssue != oldRootIssue {
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

	// Select the work
	m.focusedWorkID = work.Work.ID
	m.viewMode = ViewNormal
	// If we're already on work tabs, stay there, otherwise go to work details
	if m.activePanel != PanelWorkTabs {
		m.activePanel = PanelWorkDetails
	}
	m.statusMessage = fmt.Sprintf("Focused on work %s", m.focusedWorkID)
	m.statusIsError = false

	// Clear unseen PR changes flag for this work
	_ = m.proj.DB.MarkWorkPRSeen(m.ctx, m.focusedWorkID)

	// Set up the work details panel
	m.workDetails.SetFocusedWork(work)
	m.workDetails.SetSelectedIndex(0)
	m.workDetails.SetOrchestratorHealth(checkOrchestratorHealth(m.ctx, m.proj.DB, m.focusedWorkID))

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
