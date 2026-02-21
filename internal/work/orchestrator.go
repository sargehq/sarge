package work

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/zellij"
	"github.com/sargehq/sarge/internal/zmx"
)

// tabBelongsToWork returns true if a tab name belongs to the given work ID.
// Matches orch, task, console, claude, and pi tabs for this work.
func tabBelongsToWork(tabName, workID string) bool {
	for _, prefix := range []string{"orch-", "task-", "console-", "claude-", "pi-"} {
		if strings.HasPrefix(tabName, prefix+workID) {
			return true
		}
	}
	return false
}

// OrchestratorManager provides operations for managing work orchestrators and related tabs.
// This interface enables dependency injection and testing of orchestrator management.
//
//go:generate moq -stub -out orchestrator_mock.go . OrchestratorManager:OrchestratorManagerMock
type OrchestratorManager interface {
	// EnsureWorkOrchestrator checks if a work orchestrator tab exists and spawns one if not.
	// Returns true if the orchestrator was spawned, false if it was already running.
	EnsureWorkOrchestrator(ctx context.Context, workID, projName, workDir, friendlyName string, w io.Writer) (bool, error)

	// SpawnWorkOrchestrator creates a zellij tab and runs the orchestrate command for a work unit.
	SpawnWorkOrchestrator(ctx context.Context, workID, projName, workDir, friendlyName string, w io.Writer) error

	// TerminateWorkTabs terminates all zellij tabs associated with a work unit.
	TerminateWorkTabs(ctx context.Context, workID, projName string, w io.Writer) error

	// SpawnPlanSession creates a zellij tab and runs the plan command for a bead.
	SpawnPlanSession(ctx context.Context, beadID, projName, mainRepoPath string, w io.Writer) error

	// OpenConsole creates a zellij tab with a shell in the work's worktree.
	OpenConsole(ctx context.Context, workID, projName, workDir, friendlyName string, hooksEnv []string, w io.Writer) error

	// OpenAgentSession creates a zellij tab with an interactive agent session.
	OpenAgentSession(ctx context.Context, workID, projName, workDir, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) error
}

// DefaultOrchestratorManager is the default implementation of OrchestratorManager.
// It holds the database reference needed for orchestrator heartbeat checking.
type DefaultOrchestratorManager struct {
	database  *db.DB
	zellij    zellij.SessionManager
	zmx       zmx.Client
	muxConfig *project.MultiplexerConfig
}

// NewOrchestratorManager creates a new DefaultOrchestratorManager with the given database and config.
func NewOrchestratorManager(database *db.DB, cfg *project.Config) OrchestratorManager {
	var muxCfg *project.MultiplexerConfig
	if cfg != nil {
		muxCfg = &cfg.Multiplexer
	}
	return &DefaultOrchestratorManager{
		database:  database,
		zellij:    zellij.New(),
		zmx:       zmx.New(),
		muxConfig: muxCfg,
	}
}

// NewOrchestratorManagerWithDeps creates a new DefaultOrchestratorManager with explicit dependencies.
// This is the preferred constructor for testing.
func NewOrchestratorManagerWithDeps(database *db.DB, zc zellij.SessionManager, zmxClient zmx.Client, muxCfg *project.MultiplexerConfig) OrchestratorManager {
	return &DefaultOrchestratorManager{
		database:  database,
		zellij:    zc,
		zmx:       zmxClient,
		muxConfig: muxCfg,
	}
}

// isZmx returns true if the multiplexer is configured as zmx.
func (m *DefaultOrchestratorManager) isZmx() bool {
	return m.muxConfig != nil && m.muxConfig.IsZmx()
}

// tabExists checks if a tab with the given name exists in the session.
func (m *DefaultOrchestratorManager) tabExists(ctx context.Context, sessionName, tabName string) bool {
	if m.isZmx() {
		projectName, _ := zmx.ParseSessionName(sessionName)
		if projectName == "" {
			// sessionName is a project name, not a zmx session name
			projectName = strings.TrimPrefix(sessionName, "sarge-")
		}
		zmxName := zmx.SessionName(projectName, tabName)
		exists, _ := m.zmx.SessionExists(ctx, zmxName)
		return exists
	}
	exists, _ := m.zellij.Session(sessionName).TabExists(ctx, tabName)
	return exists
}

// TerminateWorkTabs terminates all zellij tabs associated with a work unit.
// This includes the work orchestrator tab (orch-<workID>), task tabs (task-<workID>.*),
// console tabs (console-<workID>*), and claude tabs (claude-<workID>*).
//
// For the orchestrator tab, the process is sent SIGTERM via its tracked PID before
// the tab is closed. For all other tabs, the tab is closed directly (which kills
// the processes in it). This avoids sending Ctrl+C via "zellij action write",
// which can hit the wrong pane after focus shifts.
//
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) TerminateWorkTabs(ctx context.Context, workID string, projectName string, w io.Writer) error {
	if m.isZmx() {
		return m.terminateWorkTabsZmx(ctx, workID, projectName, w)
	}
	return m.terminateWorkTabsZellij(ctx, workID, projectName, w)
}

// terminateWorkTabsZmx terminates all zmx sessions associated with a work unit.
func (m *DefaultOrchestratorManager) terminateWorkTabsZmx(ctx context.Context, workID string, projectName string, w io.Writer) error {
	logging.Debug("terminateWorkTabsZmx starting", "work_id", workID)

	// SIGTERM the orchestrator process first so it shuts down cleanly
	m.terminateOrchestratorProcess(ctx, workID)

	// List all sessions for this project, kill any belonging to this work
	projectPrefix := zmx.SessionName(projectName, "")
	sessions, _ := m.zmx.ListSessions(ctx, projectPrefix)
	for _, name := range sessions {
		_, tabName := zmx.ParseSessionName(name)
		if tabName != "" && tabBelongsToWork(tabName, workID) {
			if err := m.zmx.KillSession(ctx, name); err == nil {
				fmt.Fprintf(w, "  Killed session: %s\n", name)
			}
		}
	}

	logging.Debug("terminateWorkTabsZmx completed", "work_id", workID)
	return nil
}

// terminateWorkTabsZellij terminates all zellij tabs associated with a work unit.
func (m *DefaultOrchestratorManager) terminateWorkTabsZellij(ctx context.Context, workID string, projectName string, w io.Writer) error {
	sessionName := project.SessionNameForProject(projectName)

	logging.Debug("TerminateWorkTabs starting",
		"work_id", workID,
		"session_name", sessionName)

	// Check if session exists
	exists, err := m.zellij.SessionExists(ctx, sessionName)
	if err != nil || !exists {
		logging.Debug("Session does not exist, nothing to terminate",
			"work_id", workID,
			"session_name", sessionName,
			"exists", exists,
			"error", err)
		return nil
	}

	// Get list of all tab names
	session := m.zellij.Session(sessionName)
	tabNames, err := session.QueryTabNames(ctx)
	if err != nil {
		logging.Warn("Failed to query tab names",
			"work_id", workID,
			"session_name", sessionName,
			"error", err)
		return nil
	}

	logging.Debug("Queried tab names",
		"work_id", workID,
		"tab_count", len(tabNames),
		"tabs", tabNames)

	// Find tabs belonging to this work
	var tabsToClose []string
	for _, tabName := range tabNames {
		tabName = strings.TrimSpace(tabName)
		if tabName != "" && tabBelongsToWork(tabName, workID) {
			tabsToClose = append(tabsToClose, tabName)
		}
	}

	if len(tabsToClose) == 0 {
		logging.Debug("No matching tabs to close", "work_id", workID)
		return nil
	}

	// SIGTERM the orchestrator process first so it shuts down cleanly
	m.terminateOrchestratorProcess(ctx, workID)

	fmt.Fprintf(w, "Terminating %d zellij tab(s) for work %s...\n", len(tabsToClose), workID)

	for _, tabName := range tabsToClose {

		if err := session.CloseTabByName(ctx, tabName); err != nil {
			logging.Warn("Failed to close tab",
				"work_id", workID,
				"tab_name", tabName,
				"error", err)
			fmt.Fprintf(w, "Warning: failed to close tab %s: %v\n", tabName, err)
			// Continue with other tabs
		} else {
			logging.Debug("Tab closed successfully",
				"work_id", workID,
				"tab_name", tabName)
			fmt.Fprintf(w, "  Closed tab: %s\n", tabName)
		}
	}

	logging.Debug("TerminateWorkTabs completed", "work_id", workID)
	return nil
}

// terminateOrchestratorProcess sends SIGTERM to the orchestrator process for a work unit.
// If the process is not found or already dead, this is a no-op.
func (m *DefaultOrchestratorManager) terminateOrchestratorProcess(ctx context.Context, workID string) {
	proc, err := m.database.GetOrchestratorProcess(ctx, workID)
	if err != nil || proc == nil {
		logging.Debug("No orchestrator process found to terminate",
			"work_id", workID, "error", err)
		return
	}

	logging.Debug("Sending SIGTERM to orchestrator process",
		"work_id", workID, "pid", proc.PID)

	if err := syscall.Kill(proc.PID, syscall.SIGTERM); err != nil {
		logging.Debug("Failed to send SIGTERM (process may already be dead)",
			"work_id", workID, "pid", proc.PID, "error", err)
		return
	}

	// Brief wait for the process to handle the signal
	time.Sleep(500 * time.Millisecond)
}

// SpawnWorkOrchestrator creates a zellij tab and runs the orchestrate command for a work unit.
// The tab is named "orch-<work-id>" or "orch-<work-id> (friendlyName)" for easy identification.
// The function returns immediately after spawning - the orchestrator runs in the tab.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
//
// IMPORTANT: The zellij session must already exist before calling this function.
// Callers should use control.EnsureControlPlane to ensure
// the session exists with the control plane running.
func (m *DefaultOrchestratorManager) SpawnWorkOrchestrator(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) error {
	if m.isZmx() {
		return m.spawnWorkOrchestratorZmx(ctx, workID, projectName, workDir, friendlyName, w)
	}
	return m.spawnWorkOrchestratorZellij(ctx, workID, projectName, workDir, friendlyName, w)
}

// spawnWorkOrchestratorZmx creates a zmx session running the orchestrate command.
func (m *DefaultOrchestratorManager) spawnWorkOrchestratorZmx(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) error {
	logging.Debug("spawnWorkOrchestratorZmx called", "workID", workID, "projectName", projectName, "workDir", workDir)
	tabName := project.FormatTabName("orch", workID, friendlyName)
	zmxName := zmx.SessionName(projectName, tabName)

	// Kill existing session if present
	exists, _ := m.zmx.SessionExists(ctx, zmxName)
	if exists {
		_ = m.zmx.KillSession(ctx, zmxName)
		fmt.Fprintf(w, "Session %s already existed, killed and recreating...\n", zmxName)
	}

	// Create a new zmx session with the orchestrate command
	fmt.Fprintf(w, "Creating zmx session: %s\n", zmxName)
	if err := m.zmx.RunSession(ctx, zmxName, "sarge", []string{"orchestrate", "--work", workID}, workDir); err != nil {
		return fmt.Errorf("failed to create zmx session: %w", err)
	}

	logging.Debug("spawnWorkOrchestratorZmx completed", "workID", workID, "zmxName", zmxName)
	fmt.Fprintf(w, "Work orchestrator spawned in zmx session %s\n", zmxName)
	return nil
}

// spawnWorkOrchestratorZellij creates a zellij tab running the orchestrate command.
func (m *DefaultOrchestratorManager) spawnWorkOrchestratorZellij(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) error {
	logging.Debug("SpawnWorkOrchestrator called", "workID", workID, "projectName", projectName, "workDir", workDir)
	sessionName := project.SessionNameForProject(projectName)
	tabName := project.FormatTabName("orch", workID, friendlyName)

	// Verify session exists - callers must initialize it with control plane
	logging.Debug("SpawnWorkOrchestrator checking session exists", "sessionName", sessionName)
	exists, err := m.zellij.SessionExists(ctx, sessionName)
	if err != nil {
		logging.Error("SpawnWorkOrchestrator SessionExists check failed", "sessionName", sessionName, "error", err)
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		logging.Error("SpawnWorkOrchestrator session does not exist", "sessionName", sessionName)
		return fmt.Errorf("zellij session %s does not exist - call control.EnsureControlPlane first", sessionName)
	}

	// Check if tab already exists
	session := m.zellij.Session(sessionName)
	tabExists, err := session.TabExists(ctx, tabName)
	if err != nil {
		return fmt.Errorf("failed to check if tab exists: %w", err)
	}
	if tabExists {
		fmt.Fprintf(w, "Tab %s already exists, terminating and recreating...\n", tabName)

		// Terminate and close the existing tab
		if err := session.TerminateAndCloseTab(ctx, tabName); err != nil {
			fmt.Fprintf(w, "Warning: failed to terminate existing tab: %v\n", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Create a new tab with the orchestrate command using a layout
	fmt.Fprintf(w, "Creating tab: %s in session %s\n", tabName, sessionName)
	if err := session.CreateTabWithCommand(ctx, tabName, workDir, "sarge", []string{"orchestrate", "--work", workID}, "orchestrator"); err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}

	logging.Debug("SpawnWorkOrchestrator completed successfully", "workID", workID, "sessionName", sessionName, "tabName", tabName)
	fmt.Fprintf(w, "Work orchestrator spawned in zellij session %s, tab %s\n", sessionName, tabName)
	return nil
}

// EnsureWorkOrchestrator checks if a work orchestrator tab exists and spawns one if not.
// This is used for resilience - if the orchestrator crashes or is killed, it can be restarted.
// Returns true if the orchestrator was spawned, false if it was already running.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) EnsureWorkOrchestrator(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
	tabName := project.FormatTabName("orch", workID, friendlyName)
	logging.Debug("EnsureWorkOrchestrator called",
		"workID", workID, "projectName", projectName, "workDir", workDir,
		"friendlyName", friendlyName, "tabName", tabName, "isZmx", m.isZmx())

	// For zmx, check session existence directly
	var sessionExists bool
	if m.isZmx() {
		zmxName := zmx.SessionName(projectName, tabName)
		logging.Debug("EnsureWorkOrchestrator checking zmx session", "zmxName", zmxName)
		sessionExists, _ = m.zmx.SessionExists(ctx, zmxName)
		logging.Debug("EnsureWorkOrchestrator zmx session check result", "zmxName", zmxName, "exists", sessionExists)
	} else {
		sessionName := project.SessionNameForProject(projectName)
		sessionExists = m.tabExists(ctx, sessionName, tabName)
	}

	// Check if the orchestrator is alive via database heartbeat
	if sessionExists {
		alive, err := m.database.IsOrchestratorAlive(ctx, workID, db.DefaultStalenessThreshold)
		logging.Debug("EnsureWorkOrchestrator heartbeat check", "workID", workID, "alive", alive, "err", err)
		if err == nil && alive {
			fmt.Fprintf(w, "Work orchestrator tab %s already exists and orchestrator is alive\n", tabName)
			return false, nil
		}
		// Tab/session exists but orchestrator is dead - SpawnWorkOrchestrator will terminate and recreate
		fmt.Fprintf(w, "Work orchestrator tab %s exists but orchestrator is dead - restarting...\n", tabName)
	}

	// Spawn the orchestrator (handles existing tab/session termination)
	logging.Debug("EnsureWorkOrchestrator spawning orchestrator", "workID", workID)
	if err := m.SpawnWorkOrchestrator(ctx, workID, projectName, workDir, friendlyName, w); err != nil {
		logging.Error("EnsureWorkOrchestrator spawn failed", "workID", workID, "error", err)
		return false, err
	}

	logging.Debug("EnsureWorkOrchestrator spawn succeeded", "workID", workID)
	return true, nil
}
