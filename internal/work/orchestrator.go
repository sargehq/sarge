package work

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"
	"time"

	"github.com/sargehq/sarge/internal/bridge"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/ptysession"
)

// WorkSession represents an active session for a work unit.
type WorkSession struct {
	Name        string // Session name/ID
	TabName     string // Parsed tab name, e.g. "orch-w123"
	Type        string // Session type: "orch", "console", "agent", "task"
	DisplayName string // Human-friendly label for the picker
}

// OrchestratorManager provides operations for managing work orchestrators and related sessions.
// This interface enables dependency injection and testing of orchestrator management.
//
//go:generate moq -stub -out orchestrator_mock.go . OrchestratorManager:OrchestratorManagerMock
type OrchestratorManager interface {
	// EnsureWorkOrchestrator checks if a work orchestrator exists and spawns one if not.
	// Returns true if the orchestrator was spawned, false if it was already running.
	EnsureWorkOrchestrator(ctx context.Context, workID, projName, workDir, friendlyName string, w io.Writer) (bool, error)

	// SpawnWorkOrchestrator spawns a work orchestrator process.
	SpawnWorkOrchestrator(ctx context.Context, workID, projName, workDir, friendlyName string, w io.Writer) error

	// TerminateWorkTabs terminates all sessions associated with a work unit.
	TerminateWorkTabs(ctx context.Context, workID, projName string, w io.Writer) error

	// SpawnPlanSession creates an interactive PTY session and sends a plan prompt for a bean.
	SpawnPlanSession(ctx context.Context, beanID, projName, mainRepoPath string, w io.Writer) (*ptysession.Session, error)

	// OpenConsole opens a shell session in the work's worktree.
	OpenConsole(ctx context.Context, workID, projName, workDir, friendlyName string, hooksEnv []string, w io.Writer) error

	// OpenAgentSession creates an interactive PTY session for agent use.
	OpenAgentSession(ctx context.Context, workID, projName, workDir, friendlyName string, hooksEnv []string, cfg *project.Config, w io.Writer) (*ptysession.Session, error)

	// ListWorkSessions returns active sessions for a given work unit.
	ListWorkSessions(ctx context.Context, workID, projectName string) ([]WorkSession, error)

	// AttachToSession attaches to a named session.
	AttachToSession(ctx context.Context, sessionName string) error
}

// DefaultOrchestratorManager is the default implementation of OrchestratorManager.
// It holds the database reference needed for orchestrator heartbeat checking.
type DefaultOrchestratorManager struct {
	database   *db.DB
	bridge     *bridge.Bridge           // Optional: headless RPC sessions (sequencer)
	ptyManager *ptysession.Manager      // PTY sessions for interactive use (plan/agent/console)
	projCfg    *project.Config
	beansPath  string // Path to beans directory (needed for plan prompts)
}

// NewOrchestratorManager creates a new DefaultOrchestratorManager with the given database and config.
func NewOrchestratorManager(database *db.DB, cfg *project.Config) OrchestratorManager {
	return &DefaultOrchestratorManager{
		database: database,
		projCfg:  cfg,
	}
}

// NewOrchestratorManagerWithBridge creates a DefaultOrchestratorManager that routes
// plan and agent sessions through the given bridge (pi RPC) for headless use,
// and uses PTY sessions for interactive use.
func NewOrchestratorManagerWithBridge(database *db.DB, cfg *project.Config, b *bridge.Bridge, ptyMgr *ptysession.Manager, beansPath string) OrchestratorManager {
	return &DefaultOrchestratorManager{
		database:   database,
		bridge:     b,
		ptyManager: ptyMgr,
		projCfg:    cfg,
		beansPath:  beansPath,
	}
}

// NewOrchestratorManagerWithDeps creates a new DefaultOrchestratorManager with explicit dependencies.
// This is the preferred constructor for testing.
func NewOrchestratorManagerWithDeps(database *db.DB) OrchestratorManager {
	return &DefaultOrchestratorManager{
		database: database,
	}
}

// TerminateWorkTabs terminates all sessions associated with a work unit.
// This includes the work orchestrator, task, console, and agent sessions.
//
// For the orchestrator, the process is sent SIGTERM via its tracked PID.
// Bridge sessions are killed directly.
//
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) TerminateWorkTabs(ctx context.Context, workID string, projectName string, w io.Writer) error {
	logging.Debug("TerminateWorkTabs starting", "work_id", workID)

	// SIGTERM the orchestrator process first so it shuts down cleanly
	m.terminateOrchestratorProcess(ctx, workID)

	// Kill PTY sessions belonging to this work
	if m.ptyManager != nil {
		sessions := m.ptyManager.List()
		for id := range sessions {
			if strings.Contains(id, workID) {
				if err := m.ptyManager.Kill(id); err == nil {
					fmt.Fprintf(w, "  Killed PTY session: %s\n", id)
				}
			}
		}
	}

	// Kill bridge sessions belonging to this work
	if m.bridge != nil {
		sessions := m.bridge.ListSessions()
		for id := range sessions {
			if strings.Contains(id, workID) {
				if err := m.bridge.KillSession(id); err == nil {
					fmt.Fprintf(w, "  Killed bridge session: %s\n", id)
				}
			}
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

// SpawnWorkOrchestrator spawns a work orchestrator process.
// The function returns immediately after spawning - the orchestrator runs in the background.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) SpawnWorkOrchestrator(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) error {
	logging.Debug("SpawnWorkOrchestrator called", "workID", workID, "projectName", projectName, "workDir", workDir)

	// The orchestrator is spawned as an in-process goroutine or external process
	// by the TUI/CLI layer. This method is a compatibility shim.
	fmt.Fprintf(w, "Work orchestrator spawn requested for %s\n", workID)
	return nil
}

// EnsureWorkOrchestrator checks if a work orchestrator exists and spawns one if not.
// This is used for resilience - if the orchestrator crashes or is killed, it can be restarted.
// Returns true if the orchestrator was spawned, false if it was already running.
// Progress messages are written to the provided writer. Pass io.Discard to suppress output.
func (m *DefaultOrchestratorManager) EnsureWorkOrchestrator(ctx context.Context, workID string, projectName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
	logging.Debug("EnsureWorkOrchestrator called",
		"workID", workID, "projectName", projectName, "workDir", workDir,
		"friendlyName", friendlyName)

	displayTabName := project.FormatTabNameShort("orch", workID)

	// Check if the orchestrator is alive via database heartbeat
	alive, err := m.database.IsOrchestratorAlive(ctx, workID, db.DefaultStalenessThreshold)
	logging.Debug("EnsureWorkOrchestrator heartbeat check", "workID", workID, "alive", alive, "err", err)
	if err == nil && alive {
		fmt.Fprintf(w, "Work orchestrator %s already exists and orchestrator is alive\n", displayTabName)
		return false, nil
	}

	// Spawn the orchestrator
	logging.Debug("EnsureWorkOrchestrator spawning orchestrator", "workID", workID)
	if err := m.SpawnWorkOrchestrator(ctx, workID, projectName, workDir, friendlyName, w); err != nil {
		logging.Error("EnsureWorkOrchestrator spawn failed", "workID", workID, "error", err)
		return false, err
	}

	logging.Debug("EnsureWorkOrchestrator spawn succeeded", "workID", workID)
	return true, nil
}

// ListWorkSessions returns active sessions for a given work unit.
// Returns PTY sessions and bridge sessions for the work.
func (m *DefaultOrchestratorManager) ListWorkSessions(ctx context.Context, workID, projectName string) ([]WorkSession, error) {
	var result []WorkSession

	// Include PTY sessions
	if m.ptyManager != nil {
		ptySessions := m.ptyManager.List()
		for id, state := range ptySessions {
			if !strings.Contains(id, workID) && !strings.HasPrefix(id, "plan-") {
				continue
			}
			sessionType := "orch"
			if strings.Contains(id, "agent") {
				sessionType = "agent"
			} else if strings.Contains(id, "plan") {
				sessionType = "plan"
			} else if strings.Contains(id, "console") {
				sessionType = "console"
			}
			result = append(result, WorkSession{
				Name:        id,
				TabName:     id,
				Type:        sessionType,
				DisplayName: fmt.Sprintf("%s (%s) [%s]", sessionType, id, state),
			})
		}
	}

	// Include bridge sessions (headless tasks from sequencer)
	if m.bridge != nil {
		bridgeSessions := m.bridge.ListSessions()
		for id, state := range bridgeSessions {
			if !strings.Contains(id, workID) {
				continue
			}
			result = append(result, WorkSession{
				Name:        id,
				TabName:     id,
				Type:        "task",
				DisplayName: fmt.Sprintf("task (%s) [%s]", id, state),
			})
		}
	}

	return result, nil
}

// AttachToSession attaches to a named session.
// This is a no-op in the bridge architecture as sessions are managed through the TUI.
func (m *DefaultOrchestratorManager) AttachToSession(ctx context.Context, sessionName string) error {
	logging.Debug("AttachToSession called (no-op in bridge architecture)", "sessionName", sessionName)
	return nil
}
