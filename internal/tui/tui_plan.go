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
	"github.com/sargehq/sarge/internal/beans"
	beanswatcher "github.com/sargehq/sarge/internal/beans/watcher"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/project"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
	"github.com/sargehq/sarge/internal/work"
	"github.com/sargehq/sarge/internal/bridge"

)

// bridgeEventMsg wraps a bridge event for the Bubbletea message loop.
type bridgeEventMsg struct {
	sessionID string
	event     bridge.Event
	closed    bool // true when the session's event channel was closed
}

// watcherEventMsg wraps beans watcher events for tea.Msg
type watcherEventMsg beanswatcher.WatcherEvent

// trackingWatcherEventMsg wraps tracking watcher events for tea.Msg
type trackingWatcherEventMsg trackingwatcher.WatcherEvent

// planModel is the Plan Mode model focused on issue/bean management
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
	beanFormPanel     *BeanFormPanel
	createWorkPanel   *CreateWorkPanel



	// Bridge session support
	bridgeClient        *bridge.Bridge
	sessionPanel        *SessionPanel
	sessionPicker       *SessionPickerPanel
	activeSessionID     string // Currently viewed bridge session ID

	// Panel state
	activePanel Panel
	beansCursor int

	// Data
	beanItems     []beanItem
	filters       beanFilters
	beansExpanded bool

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
	addChildToWorkID       string          // Work ID to add newly created child bean to (for add-child-and-run flow)

	// Multi-select state
	selectedBeans map[string]bool // beanID -> is selected

	// Loading state
	loading bool

	// Search sequence tracking to handle async refresh race conditions
	searchSeq uint64 // Incremented on each search change

	// Per-bean session tracking
	activeBeanSessions map[string]bool // beanID -> has active session


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
	beansWatcher    *beanswatcher.Watcher
	trackingWatcher *trackingwatcher.Watcher

	// New bean animation tracking
	newBeans map[string]time.Time // beanID -> creation timestamp for animation
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

	// Initialize beans database watcher
	beansDir := proj.BeansPath()
	beansWatcher, err := beanswatcher.New(beanswatcher.DefaultConfig(beansDir))
	if err != nil {
		// Log error but continue without watcher
		fmt.Fprintf(os.Stderr, "Warning: Failed to initialize beans watcher: %v\n", err)
		beansWatcher = nil
	} else {
		if err := beansWatcher.Start(); err != nil {
			// Log error and disable watcher
			fmt.Fprintf(os.Stderr, "Warning: Failed to start beans watcher: %v\n", err)
			beansWatcher = nil
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
		activeBeanSessions:     make(map[string]bool),
		selectedBeans:          make(map[string]bool),
		newBeans:               make(map[string]time.Time),

		columnRatio:            0.4,  // Default 40/60 split (issues/details)
		hoveredIssue:           -1,   // No issue hovered initially
		hoveredWorkItem:        -1,   // No work item hovered initially
		pendingWorkSelectIndex: -1,   // No pending work selection
		workDetailsFocusLeft:   true, // Start with left panel focused
		beansWatcher:           beansWatcher,
		trackingWatcher:        trackingWatcher,
		filters: beanFilters{
			status: beans.StatusTodo,
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
	m.beanFormPanel = NewBeanFormPanel()
	m.createWorkPanel = NewCreateWorkPanel()

	m.sessionPanel = NewSessionPanel()
	m.sessionPicker = NewSessionPickerPanel()
	m.bridgeClient = bridge.NewBridge()

	// Wire bridge into the WorkService's OrchestratorManager so plan/agent
	// sessions are routed through pi RPC via the bridge.
	m.workService.OrchestratorManager = work.NewOrchestratorManagerWithBridge(
		proj.DB, proj.Config, m.bridgeClient, proj.BeansPath(),
	)

	// Set up status bar data providers
	m.statusBar.SetDataProviders(
		func() []beanItem { return m.beanItems },
		func() int { return m.beansCursor },
		func() map[string]bool { return m.activeBeanSessions },
		func() ViewMode { return m.viewMode },
		func() string { return m.textInput.View() },
	)

	// Set up the failed task selected provider for work detail context
	m.statusBar.SetFailedTaskSelectedProvider(func() bool {
		return m.workDetails.IsSelectedTaskFailed()
	})

	// Set up the active panel provider for panel-aware key labels
	m.statusBar.SetActivePanelProvider(func() Panel {
		return m.activePanel
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
	if m.beansWatcher != nil {
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
	if m.beansWatcher == nil {
		return nil
	}

	return func() tea.Msg {
		sub := m.beansWatcher.Broker().Subscribe(m.ctx)

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
	case bridgeEventMsg:
		if msg.closed {
			// Session ended - update streaming state
			if msg.sessionID == m.activeSessionID {
				m.sessionPanel.SetStreaming(false)
			}
			return m, nil
		}
		// Route event to session panel if it's the active session
		if msg.sessionID == m.activeSessionID {
			m.sessionPanel.HandleEvent(msg.event)
		}
		// Continue listening for more events from this session
		return m, m.waitForBridgeEvent(msg.sessionID)

	case watcherEventMsg:
		// Handle watcher events
		if msg.Type == beanswatcher.BeansChanged {
			// Flush cache and trigger data reload
			if m.proj.Beans != nil {
				_ = m.proj.Beans.FlushCache(m.ctx)
			}
			// Trigger data reload and wait for next watcher event
			return m, tea.Batch(m.refreshData(), m.waitForWatcherEvent())
		} else if msg.Type == beanswatcher.WatcherError {
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
						// Submit bean form - inline the logic
						result := m.beanFormPanel.GetResult()
						if result.Title == "" {
							return m, nil
						}
						m.viewMode = ViewNormal
						m.beanFormPanel.Blur()

						// Determine mode and call appropriate action
						if result.EditBeanID != "" {
							// Edit mode
							return m, m.saveBeanEdit(result.EditBeanID, result.Title, result.Description, result.BeanType, result.Status)
						}

						// Create or add-child mode
						isEpic := result.BeanType == "epic"
						return m, m.createBean(result.Title, result.BeanType, result.Priority, isEpic, result.Description, result.ParentID)
					}
				} else if clickedDialogButton == "cancel" {
					// Cancel the form
					if m.viewMode == ViewLinearImportInline {
						m.linearImportPanel.Blur()
					} else if m.viewMode == ViewCreateWork {
						m.createWorkPanel.Blur()
					} else {
						m.beanFormPanel.Blur()
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
						m.selectedBeans = make(map[string]bool)
						return m, m.executeCreateWork(result.BeanID, result.BranchName, false, result.UseExistingBranch)
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
						m.selectedBeans = make(map[string]bool)
						return m, m.executeCreateWork(result.BeanID, result.BranchName, true, result.UseExistingBranch)
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
							// Update filter to show beans for clicked item
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
						if clickedIssue >= 0 && clickedIssue < len(m.beanItems) {
							m.beansCursor = clickedIssue
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
					if clickedIssue >= 0 && clickedIssue < len(m.beanItems) {
						m.beansCursor = clickedIssue
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

		// Detect new beans by comparing with existing list
		if len(m.beanItems) > 0 {
			existingIDs := make(map[string]bool)
			for _, bean := range m.beanItems {
				existingIDs[bean.ID] = true
			}
			for _, bean := range msg.beans {
				// Mark as new if not in existing list and not already animated
				if !existingIDs[bean.ID] && m.newBeans[bean.ID].IsZero() {
					m.newBeans[bean.ID] = now
					expireCmds = append(expireCmds, scheduleNewBeanExpire(bean.ID))
				}
			}
		}

		m.beanItems = msg.beans
		if msg.activeSessions != nil {
			m.activeBeanSessions = msg.activeSessions
		}
		m.loading = false
		m.lastUpdate = time.Now()
		if msg.err != nil {
			m.statusMessage = msg.err.Error()
			m.statusIsError = true
		}

		// Ensure cursor stays within bounds after filter changes
		if m.beansCursor >= len(m.beanItems) {
			if len(m.beanItems) > 0 {
				m.beansCursor = len(m.beanItems) - 1
			} else {
				m.beansCursor = 0
			}
		}

		// Check if we need to add a newly created bean to a work (add-child-and-run flow)
		if m.addChildToWorkID != "" && msg.createdBeanID != "" {
			workID := m.addChildToWorkID
			beanID := msg.createdBeanID
			// Don't clear addChildToWorkID yet - wait for beanAddedToWorkMsg
			cmds := append(expireCmds, m.addBeansToWork([]string{beanID}, workID))
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
		} else if msg.bridgeSessionID != "" {
			// Bridge session - open session viewer
			m.viewBridgeSession(msg.bridgeSessionID)
			m.viewMode = ViewSessionViewer
			m.activePanel = PanelSession
			if msg.resumed {
				m.statusMessage = fmt.Sprintf("Resumed plan session for %s", msg.beanID)
			} else {
				m.statusMessage = fmt.Sprintf("Started plan session for %s", msg.beanID)
			}
			m.statusIsError = false
			return m, tea.Batch(m.refreshData(), m.waitForBridgeEvent(msg.bridgeSessionID))
		} else if msg.resumed {
			m.statusMessage = fmt.Sprintf("Resumed session for %s", msg.beanID)
			m.statusIsError = false
		} else {
			m.statusMessage = fmt.Sprintf("Started session for %s", msg.beanID)
			m.statusIsError = false
		}
		// Refresh to update session indicators
		return m, m.refreshData()

	case agentSessionOpenedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Open Agent failed: %v", msg.err)
			m.statusIsError = true
		} else if msg.bridgeSessionID != "" {
			// Bridge session - open session viewer
			m.viewBridgeSession(msg.bridgeSessionID)
			m.viewMode = ViewSessionViewer
			m.activePanel = PanelSession
			m.statusMessage = fmt.Sprintf("Agent session opened for %s", msg.workID)
			m.statusIsError = false
			return m, tea.Batch(m.refreshData(), m.waitForBridgeEvent(msg.bridgeSessionID))
		} else {
			m.statusMessage = fmt.Sprintf("Agent session opened for %s", msg.workID)
			m.statusIsError = false
		}
		return m, m.refreshData()

	case planWorkCreatedMsg:
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to create work: %v", msg.err)
			m.statusIsError = true
		} else {
			m.statusMessage = fmt.Sprintf("Created work %s from %s", msg.workID, msg.beanID)
			m.statusIsError = false
		}
		// Refresh work tiles to show the new work in the tabs bar
		return m, tea.Batch(m.refreshData(), m.loadWorkTiles())

	case beanAddedToWorkMsg:
		m.viewMode = ViewNormal
		if msg.err != nil {
			m.statusMessage = fmt.Sprintf("Failed to add issue: %v", msg.err)
			m.statusIsError = true
			m.addChildToWorkID = "" // Clear on error
		} else {
			m.statusMessage = fmt.Sprintf("Added %s to work %s", msg.beanID, msg.workID)
			m.statusIsError = false

			// Check if we should run the work (add-child-and-run flow)
			if m.addChildToWorkID != "" && m.addChildToWorkID == msg.workID {
				m.addChildToWorkID = "" // Clear before running
				// Run the work in single-bean mode
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
			m.statusMessage = fmt.Sprintf("%s (%s)", msg.action, msg.workID)
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
			// Rebuild the filter to reflect any changes in work beans
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
			if len(msg.beanIDs) == 1 {
				m.statusMessage = fmt.Sprintf("%s: %s", msg.skipReason, msg.beanIDs[0])
			} else {
				m.statusMessage = msg.skipReason
			}
			m.statusIsError = false
		} else {
			// Single import success or legacy format
			if len(msg.beanIDs) == 1 {
				m.statusMessage = fmt.Sprintf("Successfully imported %s", msg.beanIDs[0])
			} else if len(msg.beanIDs) > 1 {
				m.statusMessage = fmt.Sprintf("Successfully imported %d issues", len(msg.beanIDs))
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

	case newBeanExpireMsg:
		// Remove the bean from the newBeans map to stop animation
		delete(m.newBeans, msg.beanID)
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
	beans          []beanItem
	activeSessions map[string]bool
	err            error
	searchSeq      uint64 // Sequence number to detect stale results
	createdBeanID  string // ID of newly created bean (for add-child-and-run flow)
}

// planStatusMsg is sent to update status text
type planStatusMsg struct {
	message string
	isError bool
}

// planSessionSpawnedMsg indicates a planning session was spawned or resumed
type planSessionSpawnedMsg struct {
	beanID          string
	resumed         bool
	err             error

	bridgeSessionID string // non-empty when session was created via bridge
}

// planWorkCreatedMsg indicates work was created from a bean
type planWorkCreatedMsg struct {
	beanID         string
	workID         string
	err            error
	sessionName    string
}

// beanAddedToWorkMsg indicates a bean was added to a work
type beanAddedToWorkMsg struct {
	beanID string
	workID string
	err    error
}

// editorFinishedMsg is sent when the external editor closes
type editorFinishedMsg struct{}

// linearImportCompleteMsg is sent when a Linear import completes
type linearImportCompleteMsg struct {
	beanIDs      []string // IDs of imported beans
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

// newBeanExpireMsg is sent when the animation for a new bean should expire
type newBeanExpireMsg struct {
	beanID string
}

// workCommandMsg indicates a work command completed
type workCommandMsg struct {
	action string
	workID string
	err    error
}

// newBeanAnimationDuration is how long newly created beans are highlighted
const newBeanAnimationDuration = 5 * time.Second

// scheduleNewBeanExpire returns a command that expires a new bean animation after the duration
func scheduleNewBeanExpire(beanID string) tea.Cmd {
	return tea.Tick(newBeanAnimationDuration, func(t time.Time) tea.Msg {
		return newBeanExpireMsg{beanID: beanID}
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
	case ViewCreateBean, ViewCreateBeanInline, ViewAddChildBean, ViewEditBean:
		// Delegate to bean form panel and handle returned action
		cmd, action := m.beanFormPanel.Update(msg)

		switch action {
		case BeanFormActionCancel:
			m.viewMode = ViewNormal
			return m, cmd

		case BeanFormActionSubmit:
			result := m.beanFormPanel.GetResult()
			if result.Title == "" {
				return m, cmd
			}

			m.viewMode = ViewNormal
			m.beanFormPanel.Blur()

			// Determine mode and call appropriate action
			if result.EditBeanID != "" {
				// Edit mode
				return m, m.saveBeanEdit(result.EditBeanID, result.Title, result.Description, result.BeanType, result.Status)
			}

			// Create or add-child mode
			isEpic := result.BeanType == "epic"
			return m, m.createBean(result.Title, result.BeanType, result.Priority, isEpic, result.Description, result.ParentID)
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
			m.selectedBeans = make(map[string]bool)
			return m, m.executeCreateWork(result.BeanID, result.BranchName, false, result.UseExistingBranch)

		case CreateWorkActionAuto:
			result := m.createWorkPanel.GetResult()
			if result.BranchName == "" {
				m.statusMessage = "Branch name cannot be empty"
				m.statusIsError = true
				return m, nil
			}
			m.viewMode = ViewNormal
			// Clear selections after work creation
			m.selectedBeans = make(map[string]bool)
			return m, m.executeCreateWork(result.BeanID, result.BranchName, true, result.UseExistingBranch)
		}

		return m, cmd
	case ViewBeanSearch:
		return m.updateBeanSearch(msg)
	case ViewLabelFilter:
		return m.updateLabelFilter(msg)
	case ViewCloseBeanConfirm:
		return m.updateCloseBeanConfirm(msg)
	case ViewDeleteBeanConfirm:
		return m.updateDeleteBeanConfirm(msg)
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
	case ViewBridgeSessionPicker:
		cmd, action := m.sessionPicker.Update(msg)
		switch action {
		case SessionPickerActionCancel:
			m.viewMode = ViewNormal
			return m, cmd
		case SessionPickerActionSelect:
			selectedID := m.sessionPicker.SelectedSessionID()
			if selectedID != "" {
				m.viewBridgeSession(selectedID)
				m.viewMode = ViewSessionViewer
				m.activePanel = PanelSession
				return m, m.waitForBridgeEvent(selectedID)
			}
			return m, cmd
		}
		return m, cmd

	case ViewSessionViewer:
		cmd, action := m.sessionPanel.Update(msg)
		switch action {
		case SessionPanelActionAbort:
			if m.activeSessionID != "" {
				session := m.bridgeClient.GetSession(m.activeSessionID)
				if session != nil {
					if err := session.Abort(); err != nil {
						m.statusMessage = fmt.Sprintf("Failed to abort session: %v", err)
						m.statusIsError = true
					}
				}
			}
			return m, cmd
		case SessionPanelActionSteer:
			// Enter steer mode - prompt will be sent as steer
			if m.activeSessionID != "" {
				session := m.bridgeClient.GetSession(m.activeSessionID)
				if session != nil {
					// TODO: Get steer message from input
					if err := session.Steer("Please adjust your approach"); err != nil {
						m.statusMessage = fmt.Sprintf("Failed to steer session: %v", err)
						m.statusIsError = true
					}
				}
			}
			return m, cmd
		case SessionPanelActionPrompt:
			prompt := m.sessionPanel.GetPendingPrompt()
			if prompt != "" && m.activeSessionID != "" {
				session := m.bridgeClient.GetSession(m.activeSessionID)
				if session != nil {
					if err := session.Prompt(prompt); err != nil {
						m.statusMessage = fmt.Sprintf("Failed to send prompt: %v", err)
						m.statusIsError = true
					}
				}
			}
			return m, cmd
		}
		// Handle Esc to leave session viewer
		if msg.String() == "esc" && !m.sessionPanel.inputMode {
			m.viewMode = ViewNormal
			m.activePanel = PanelLeft
			return m, nil
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

	// When a work is focused, route work action keys regardless of active panel.
	// This allows work actions (t, c, i, r, o, f, g, v, p, d, x, a) to fire
	// even when the issues panel is active.
	// Note: 'd' is NOT intercepted here - it conflicts with [d]elete bean.
	// 'd' does [d]estroy when work details/tabs panel is focused,
	// and [d]elete bean when issues panel is focused.
	if m.focusedWorkID != "" {
		isWorkActionKey := false
		switch msg.String() {
		case "t", "c", "i", "r", "o", "f", "g", "v", "p", "x", "a":
			isWorkActionKey = true
		case "d":
			// 'd' is panel-aware: destroy work when work panel is focused,
			// delete bean when issues panel is focused
			if m.activePanel == PanelWorkDetails || m.activePanel == PanelWorkTabs {
				isWorkActionKey = true
			}
		}

		if isWorkActionKey {
			cmd, action := m.workDetails.Update(msg)
			switch action {
			case WorkDetailActionOpenTerminal:
				m.statusMessage = fmt.Sprintf("Opening console for %s...", m.focusedWorkID)
				m.statusIsError = false
				return m, m.openConsole()
			case WorkDetailActionOpenAgent:
				m.statusMessage = fmt.Sprintf("Opening agent for %s...", m.focusedWorkID)
				m.statusIsError = false
				return m, m.openAgent()
			case WorkDetailActionOpenIDE:
				return m, m.openIDE()
			case WorkDetailActionRun:
				m.statusMessage = fmt.Sprintf("Running work %s...", m.focusedWorkID)
				m.statusIsError = false
				focusedWork := m.workDetails.GetFocusedWork()
				useAutoGroup := focusedWork != nil && len(focusedWork.UnassignedBeans) > 1
				return m, m.runFocusedWork(useAutoGroup)
			case WorkDetailActionReview:
				return m, m.createReviewTask()
			case WorkDetailActionPR:
				return m, m.createPRTask()
			case WorkDetailActionRestartOrchestrator:
				if checkOrchestratorHealth(m.ctx, m.proj.DB, m.focusedWorkID) {
					m.statusMessage = fmt.Sprintf("Orchestrator already running (%s)", m.focusedWorkID)
					m.statusIsError = false
					return m, nil
				}
				m.statusMessage = fmt.Sprintf("Spawning orchestrator for %s...", m.focusedWorkID)
				m.statusIsError = false
				return m, m.restartOrchestrator()
			case WorkDetailActionCheckFeedback:
				return m, m.checkPRFeedback()
			case WorkDetailActionDestroy:
				focusedWork := m.workDetails.GetFocusedWork()
				if focusedWork != nil && focusedWork.Work.Status == "processing" {
					m.statusMessage = "Cannot destroy work that is currently processing"
					m.statusIsError = true
					return m, nil
				}
				m.viewMode = ViewDestroyConfirm
				return m, cmd
			case WorkDetailActionAddChildIssue:
				focusedWork := m.workDetails.GetFocusedWork()
				if focusedWork != nil && focusedWork.Work.RootIssueID != "" {
					// Check if root issue type supports children
					rootBean := m.findBeanByID(focusedWork.Work.RootIssueID)
					if rootBean != nil && !beans.CanBeParent(rootBean.Type) {
						m.statusMessage = fmt.Sprintf("Cannot add child — root issue %s (type %q) cannot have children", rootBean.ID, rootBean.Type)
						m.statusIsError = true
						return m, nil
					}
					m.addChildToWorkID = focusedWork.Work.ID
					m.beanFormPanel.SetAddChildMode(focusedWork.Work.RootIssueID)
					m.viewMode = ViewAddChildBean
					return m, m.beanFormPanel.Init()
				}
				return m, nil
			case WorkDetailActionResetTask:
				return m, m.resetSelectedTask()
			case WorkDetailActionAttachTerminal:
				// Open bridge session picker
				if m.bridgeClient != nil {
					sessions := m.bridgeClient.ListSessions()
					if len(sessions) > 0 {
						m.openBridgeSessionPicker()
						return m, nil
					}
				}
				m.statusMessage = "No active sessions for this work"
				m.statusIsError = false
				return m, nil
			case WorkDetailActionPlan:
				beanID := m.workDetails.GetSelectedUnassignedBeanID()
				if beanID != "" {
					return m, m.spawnPlanSession(beanID)
				}
				return m, nil
			case WorkDetailActionNone:
				// Work details returned no action for this key (e.g., 'x' when no failed task).
				// Fall through to issue-level handling below.
			default:
				// For navigation actions or unknown, just return
				return m, cmd
			}
		}
	}

	// Delegate j/k navigation to work details panel when it's active
	if m.activePanel == PanelWorkDetails && m.focusedWorkID != "" {
		switch msg.String() {
		case "j", "down", "k", "up":
			cmd, action := m.workDetails.Update(msg)
			if action == WorkDetailActionNavigateUp || action == WorkDetailActionNavigateDown {
				return m, m.updateWorkSelectionFilter()
			}
			return m, cmd
		}
		// For other keys when on work details, let the right panel handle viewport scrolling
		if !m.workDetailsFocusLeft {
			cmd, _ := m.workDetails.Update(msg)
			return m, cmd
		}
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
		if m.beansCursor < len(m.beanItems)-1 {
			m.beansCursor++
		}
		return m, nil

	case "k", "up":
		// Navigate up in current list (work details is handled above)
		if m.beansCursor > 0 {
			m.beansCursor--
		}
		return m, nil

	case "n":
		// Create new bean inline
		m.viewMode = ViewCreateBeanInline
		m.beanFormPanel.Reset()
		return m, m.beanFormPanel.Init()

	case "x":
		// Close selected bean(s)
		if len(m.beanItems) > 0 {
			// Check if we have any selected beans
			hasSelection := false
			for _, item := range m.beanItems {
				if m.selectedBeans[item.ID] {
					hasSelection = true
					break
				}
			}
			// If we have selected beans or a cursor bean, show confirmation
			if hasSelection || m.beansCursor < len(m.beanItems) {
				m.viewMode = ViewCloseBeanConfirm
			}
		}
		return m, nil

	case "d":
		// Delete selected bean(s) permanently
		if len(m.beanItems) > 0 {
			// Check if we have any selected beans
			hasSelection := false
			for _, item := range m.beanItems {
				if m.selectedBeans[item.ID] {
					hasSelection = true
					break
				}
			}
			// If we have selected beans or a cursor bean, show confirmation
			if hasSelection || m.beansCursor < len(m.beanItems) {
				m.viewMode = ViewDeleteBeanConfirm
			}
		}
		return m, nil

	case "/":
		// Search
		m.viewMode = ViewBeanSearch
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

	case "O":
		m.filters.status = beans.StatusTodo
		return m, m.refreshData()

	case "C":
		m.filters.status = beans.StatusCompleted
		return m, m.refreshData()

	case "R":
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

	case "V":
		m.beansExpanded = !m.beansExpanded
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
		// Toggle bean selection for multi-select
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			bean := m.beanItems[m.beansCursor]
			// Prevent selecting already-assigned beans
			if bean.assignedWorkID != "" {
				m.statusMessage = fmt.Sprintf("Cannot select: already assigned to %s", bean.assignedWorkID)
				m.statusIsError = true
				return m, nil
			}
			m.selectedBeans[bean.ID] = !m.selectedBeans[bean.ID]
		}
		return m, nil

	case "p":
		// Spawn/resume planning session for selected bean (work details panel handles 'p' for Plan)
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			beanID := m.beanItems[m.beansCursor].ID
			return m, m.spawnPlanSession(beanID)
		}
		return m, nil

	case "w":
		// Create work from cursor bean - show dialog
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			bean := m.beanItems[m.beansCursor]
			if bean.assignedWorkID != "" {
				m.statusMessage = fmt.Sprintf("Cannot create work: %s already assigned to %s", bean.ID, bean.assignedWorkID)
				m.statusIsError = true
				return m, nil
			}
			// Generate proposed branch name from cursor bean
			branchBeans := []*beansForBranch{{ID: bean.ID, Title: bean.Title}}
			branchName := generateBranchNameFromBeansForBranch(branchBeans)
			m.createWorkPanel.Reset(bean.ID, branchName)
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
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			parent := m.beanItems[m.beansCursor]
			if !beans.CanBeParent(parent.Type) {
				m.statusMessage = fmt.Sprintf("Cannot add child to %s (type %q) — only milestone, epic, and feature beans can have children", parent.ID, parent.Type)
				m.statusIsError = true
				return m, nil
			}
			m.beanFormPanel.SetAddChildMode(parent.ID)
			m.viewMode = ViewAddChildBean
			return m, m.beanFormPanel.Init()
		}
		return m, nil

	case "e":
		// Edit selected issue using the unified bean form
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			bean := m.beanItems[m.beansCursor]
			m.beanFormPanel.SetEditMode(bean.ID, bean.Title, bean.Body, bean.Type, bean.Priority, bean.Status)
			m.viewMode = ViewEditBean
			return m, m.beanFormPanel.Init()
		}
		return m, nil

	case "E":
		// Edit selected issue in external editor
		if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			bean := m.beanItems[m.beansCursor]
			return m, m.openInEditor(bean.ID)
		}
		return m, nil

	case "m":
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

	case "M":
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
		if len(m.beanItems) > 0 {
			// Collect selected beans or use cursor bean
			var beansToAdd []string
			hasSelection := false
			for _, item := range m.beanItems {
				if m.selectedBeans[item.ID] {
					hasSelection = true
					// Check if already assigned
					if item.assignedWorkID != "" {
						m.statusMessage = fmt.Sprintf("Issue %s already assigned to %s", item.ID, item.assignedWorkID)
						m.statusIsError = true
						return m, nil
					}
					beansToAdd = append(beansToAdd, item.ID)
				}
			}

			// If no selection, use cursor bean
			if !hasSelection && m.beansCursor < len(m.beanItems) {
				bean := m.beanItems[m.beansCursor]
				if bean.assignedWorkID != "" {
					m.statusMessage = fmt.Sprintf("Issue %s already assigned to %s", bean.ID, bean.assignedWorkID)
					m.statusIsError = true
					return m, nil
				}
				beansToAdd = append(beansToAdd, bean.ID)
			}

			if len(beansToAdd) > 0 {
				// Add issues directly to the focused work
				m.selectedBeans = make(map[string]bool) // Clear selection after adding
				return m, m.addBeansToWork(beansToAdd, m.focusedWorkID)
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
	// Stop the beans watcher if it's running
	if m.beansWatcher != nil {
		_ = m.beansWatcher.Stop()
	}
	// Kill all bridge sessions
	if m.bridgeClient != nil {
		_ = m.bridgeClient.KillAll()
	}
	// Note: m.proj.Beans is owned by the Project and closed by proj.Close()
	// which is deferred in runTUI. Do not close it here to avoid double-close.
}

// waitForBridgeEvent returns a tea.Cmd that waits for the next event from a bridge session.
func (m *planModel) waitForBridgeEvent(sessionID string) tea.Cmd {
	session := m.bridgeClient.GetSession(sessionID)
	if session == nil {
		return nil
	}
	return func() tea.Msg {
		evt, ok := <-session.Events()
		if !ok {
			return bridgeEventMsg{sessionID: sessionID, closed: true}
		}
		return bridgeEventMsg{sessionID: sessionID, event: evt}
	}
}

// viewBridgeSession switches the session panel to display a specific bridge session.
func (m *planModel) viewBridgeSession(sessionID string) {
	m.activeSessionID = sessionID
	session := m.bridgeClient.GetSession(sessionID)
	if session == nil {
		return
	}

	// Determine session type from ID
	sessionType := "orch"
	if strings.Contains(sessionID, "agent") {
		sessionType = "agent"
	} else if strings.Contains(sessionID, "plan") {
		sessionType = "plan"
	}

	m.sessionPanel.SetSession(sessionID, sessionType)
	m.sessionPanel.SetStreaming(session.IsStreaming())
}

// openBridgeSessionPicker shows the bridge session picker for the focused work.
func (m *planModel) openBridgeSessionPicker() {
	sessions := m.bridgeClient.ListSessions()
	if len(sessions) == 0 {
		m.statusMessage = "No active bridge sessions"
		m.statusIsError = false
		return
	}
	if len(sessions) == 1 {
		// Only one session - view it directly
		for id := range sessions {
			m.viewBridgeSession(id)
			m.viewMode = ViewSessionViewer
			m.activePanel = PanelSession
		}
		return
	}
	m.sessionPicker.SetSessions(sessions)
	m.sessionPicker.SetSize(m.width/2, m.height/2)
	m.viewMode = ViewBridgeSessionPicker
}

// syncPanels synchronizes data from planModel to the panel components
func (m *planModel) syncPanels() {
	// Calculate column widths
	totalContentWidth := m.width - 4
	issuesWidth := int(float64(totalContentWidth) * m.columnRatio)
	detailsWidth := totalContentWidth - issuesWidth

	// Determine status bar context: merged when work is focused, issues otherwise
	var statusBarCtx StatusBarContext
	if m.focusedWorkID != "" {
		statusBarCtx = StatusBarContextWorkFocused
	} else {
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
		m.beanItems,
		m.beansCursor,
		m.filters,
		m.beansExpanded,
		m.selectedBeans,
		m.activeBeanSessions,
		m.newBeans,
	)
	m.issuesPanel.SetWorkContext(m.focusedWorkID)
	m.issuesPanel.SetHoveredIssue(m.hoveredIssue)

	// Sync details panel
	m.detailsPanel.SetSize(detailsWidth, m.height)
	m.detailsPanel.SetFocus(m.activePanel == PanelRight)
	// Get focused bean and build child lookup map
	var focusedBean *beanItem
	var hasActiveSession bool
	childBeanMap := make(map[string]*beanItem)
	if len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
		focusedBean = &m.beanItems[m.beansCursor]
		hasActiveSession = m.activeBeanSessions[focusedBean.ID]
		// Build map for child lookup
		for i := range m.beanItems {
			childBeanMap[m.beanItems[i].ID] = &m.beanItems[i]
		}
	}
	m.detailsPanel.SetData(focusedBean, hasActiveSession, childBeanMap)

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

	// Sync bean form panel
	m.beanFormPanel.SetSize(detailsWidth, m.height)
	m.beanFormPanel.SetFocus(m.activePanel == PanelRight)
	m.beanFormPanel.SetHoveredButton(m.hoveredDialogButton)

	// Sync create work panel
	m.createWorkPanel.SetSize(detailsWidth, m.height)
	m.createWorkPanel.SetFocus(m.activePanel == PanelRight && m.viewMode == ViewCreateWork)
	m.createWorkPanel.SetHoveredButton(m.hoveredDialogButton)

	// Sync session panel
	m.sessionPanel.SetSize(detailsWidth, m.height)
	m.sessionPanel.SetFocus(m.activePanel == PanelSession || m.viewMode == ViewSessionViewer)
}

// View implements tea.Model
func (m *planModel) View() string {
	// Handle dialogs
	switch m.viewMode {
	case ViewCreateBean, ViewCreateBeanInline, ViewAddChildBean, ViewEditBean:
		// All bean form modes render inline in the details panel
		// Fall through to normal rendering
	case ViewCreateWork:
		// Create work now renders inline in the details panel
		// Fall through to normal rendering
	case ViewBeanSearch:
		// Inline search mode - render normal view with search bar in status area
		// Fall through to normal rendering
	case ViewLabelFilter:
		return m.renderWithDialog(m.renderLabelFilterDialogContent())
	case ViewCloseBeanConfirm:
		return m.renderWithDialog(m.renderCloseBeanConfirmContent())
	case ViewDeleteBeanConfirm:
		return m.renderWithDialog(m.renderDeleteBeanConfirmContent())
	case ViewDestroyConfirm:
		return m.renderWithDialog(m.renderDestroyConfirmContent())
	case ViewBridgeSessionPicker:
		return m.renderWithDialog(m.sessionPicker.View())
	case ViewSessionViewer:
		// Session viewer takes over the right panel (or full width)
		// Fall through to normal rendering - handled in renderTwoColumnLayout
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

// beansForBranch is a minimal struct for branch name generation
type beansForBranch struct {
	ID    string
	Title string
}

// generateBranchNameFromBeansForBranch generates a branch name from beans
func generateBranchNameFromBeansForBranch(beans []*beansForBranch) string {
	if len(beans) == 0 {
		return ""
	}
	// Use the same logic as generateBranchNameFromBeans but with local struct
	var titles []string
	for _, b := range beans {
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

// updateWorkSelectionFilter updates the bean filter based on the current work details selection
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
		// Work data is still loading - restore old filters to avoid
		// clearing the issues panel while async data loads
		m.filters.task = oldTask
		m.filters.children = oldChildren
		m.filters.rootIssue = oldRootIssue
		return nil
	}

	// Always set root issue when work panel is present so it's always visible
	if focusedWork.Work.RootIssueID != "" {
		m.filters.rootIssue = focusedWork.Work.RootIssueID
	}

	if m.workDetails.IsTaskSelected() {
		// Task selected - set task filter to show beans assigned to that task
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
		m.beansCursor = 0
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

func (m *planModel) findBeanByID(id string) *beans.BeanWithDeps {
	for _, item := range m.beanItems {
		if item.ID == id {
			return item.BeanWithDeps
		}
	}
	return nil
}
