package tui

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/process"
	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/zmx"
	workpkg "github.com/sargehq/sarge/internal/work"
)

// sessionName returns the zellij session name for this project
func (m *planModel) sessionName() string {
	return fmt.Sprintf("sarge-%s", m.proj.Config.Project.Name)
}

// spawnPlanSession spawns or resumes a planning session for a specific bead
func (m *planModel) spawnPlanSession(beadID string) tea.Cmd {
	return func() tea.Msg {
		zellijSession := m.sessionName()
		tabName := workpkg.PlanTabName(beadID)
		mainRepoPath := m.proj.MainRepoPath()

		logging.Debug("spawnPlanSession started", "beadID", beadID, "session", zellijSession, "tabName", tabName)

		// Check if session already running for this bead
		running, _ := m.proj.DB.IsPlanSessionRunning(m.ctx, beadID)
		logging.Debug("spawnPlanSession checked if running", "beadID", beadID, "running", running)
		if running {
			// Session exists - just switch to it
			if err := m.zj.Session(zellijSession).SwitchToTab(m.ctx, tabName); err != nil {
				return planSessionSpawnedMsg{beadID: beadID, err: err}
			}
			return planSessionSpawnedMsg{beadID: beadID, resumed: true}
		}

		// Ensure zellij session and control plane are running
		sessionResult, err := control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			logging.Error("spawnPlanSession EnsureControlPlane failed", "beadID", beadID, "error", err)
			return planSessionSpawnedMsg{beadID: beadID, err: err}
		}
		logging.Debug("spawnPlanSession EnsureControlPlane completed",
			"beadID", beadID,
			"sessionCreated", sessionResult.SessionCreated,
			"sessionName", sessionResult.SessionName)

		// Use the orchestrator manager to spawn the plan session
		if err := m.workService.OrchestratorManager.SpawnPlanSession(m.ctx, beadID, m.proj.Config.Project.Name, mainRepoPath, io.Discard); err != nil {
			logging.Error("spawnPlanSession SpawnPlanSession failed", "beadID", beadID, "error", err)
			return planSessionSpawnedMsg{beadID: beadID, err: err}
		}

		msg := planSessionSpawnedMsg{beadID: beadID, resumed: false}
		if sessionResult.SessionCreated {
			msg.sessionCreated = true
			msg.sessionName = sessionResult.SessionName
		}
		logging.Debug("spawnPlanSession completed", "beadID", beadID, "sessionCreated", msg.sessionCreated, "sessionName", msg.sessionName)
		return msg
	}
}

// executeCreateWork creates a work unit with the given branch name.
// This uses the shared CreateWorkFromBead method which handles:
// 1. Expanding the bead to collect all issue IDs
// 2. Creating work record in DB (with auto flag)
// 3. Initializing the zellij session
// 4. Ensuring control plane is running
func (m *planModel) executeCreateWork(beadID string, branchName string, auto bool, useExistingBranch bool) tea.Cmd {
	return func() tea.Msg {
		logging.Debug("executeCreateWork started", "beadID", beadID, "branchName", branchName, "auto", auto, "useExistingBranch", useExistingBranch)

		opts := workpkg.CreateWorkFromBeadOptions{
			BeadID:            beadID,
			BranchName:        branchName,
			BaseBranch:        m.proj.Config.Repo.GetBaseBranch(),
			Auto:              auto,
			UseExistingBranch: useExistingBranch,
		}
		result, err := m.workService.CreateWorkFromBead(m.ctx, opts)
		if err != nil {
			logging.Error("executeCreateWork CreateWorkFromBead failed", "beadID", beadID, "error", err)
			return planWorkCreatedMsg{beadID: beadID, err: err}
		}
		logging.Debug("executeCreateWork completed successfully", "workID", result.WorkID)

		// Ensure control plane is running to process the work
		sessionResult, err := control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			logging.Warn("executeCreateWork EnsureControlPlane failed", "error", err)
			// Non-fatal: work was created but control plane might need manual start
			return planWorkCreatedMsg{beadID: beadID, workID: result.WorkID, err: err}
		}

		msg := planWorkCreatedMsg{beadID: beadID, workID: result.WorkID}
		if sessionResult.SessionCreated {
			msg.sessionCreated = true
			msg.sessionName = sessionResult.SessionName
		}
		return msg
	}
}

func (m *planModel) addBeadsToWork(beadIDs []string, workID string) tea.Cmd {
	return func() tea.Msg {
		// Use WorkService to add beads
		_, err := m.workService.AddBeads(m.ctx, workID, beadIDs)
		if err != nil {
			beadIDsStr := strings.Join(beadIDs, ", ")
			return beadAddedToWorkMsg{beadID: beadIDsStr, workID: workID, err: fmt.Errorf("failed to add issues to work: %w", err)}
		}

		beadIDsStr := strings.Join(beadIDs, ", ")
		return beadAddedToWorkMsg{beadID: beadIDsStr, workID: workID}
	}
}

// workTilesLoadedMsg indicates work tiles have been loaded
type workTilesLoadedMsg struct {
	works              []*progress.WorkProgress
	orchestratorHealth map[string]bool // workID -> orchestrator alive
	err                error
}

// loadWorkTiles loads work data for the work tabs bar
func (m *planModel) loadWorkTiles() tea.Cmd {
	return func() tea.Msg {
		works, err := progress.FetchAllWorksPollData(m.ctx, m.proj)
		if err != nil {
			return workTilesLoadedMsg{err: err}
		}

		// Compute orchestrator health for all works (async)
		orchestratorHealth := make(map[string]bool)
		for _, work := range works {
			if work != nil {
				orchestratorHealth[work.Work.ID] = checkOrchestratorHealth(m.ctx, m.proj.DB, work.Work.ID)
			}
		}

		return workTilesLoadedMsg{works: works, orchestratorHealth: orchestratorHealth}
	}
}

// Helper functions for work commands

// destroyWork schedules a work destruction task via the control plane
func (m *planModel) destroyWork(workID string) tea.Cmd {
	return func() tea.Msg {
		if err := control.ScheduleDestroyWorktree(m.ctx, m.proj, workID); err != nil {
			return workCommandMsg{action: "Destroy work", workID: workID, err: err}
		}

		// Ensure control plane is running to process the destroy task
		if _, err := control.EnsureControlPlane(m.ctx, m.proj); err != nil {
			// Non-fatal: task was scheduled but control plane might need manual start
			return workCommandMsg{action: "Destroy work scheduled", workID: workID, err: fmt.Errorf("destroy scheduled but control plane failed: %w", err)}
		}

		return workCommandMsg{action: "Destroy work scheduled", workID: workID}
	}
}

// destroyFocusedWork destroys the currently focused work (used by confirmation dialog)
func (m *planModel) destroyFocusedWork() tea.Cmd {
	return m.destroyWork(m.focusedWorkID)
}

// runFocusedWork creates tasks for the currently focused work and ensures orchestrator is running
func (m *planModel) runFocusedWork(autoGroup bool) tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Check if worktree is ready (it's created asynchronously by control plane)
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Run work", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Run work", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}
		if work.WorktreePath == "" {
			return workCommandMsg{action: "Run work", workID: workID, err: fmt.Errorf("worktree is still being created, please wait a moment")}
		}

		if autoGroup {
			// Use auto mode - creates estimate task and lets orchestrator handle grouping
			_, err := m.workService.RunWorkAuto(m.ctx, workID, io.Discard)
			if err != nil {
				return workCommandMsg{action: "Run work", workID: workID, err: err}
			}
		} else {
			// Use direct mode - creates one task per bead
			_, err := m.workService.RunWork(m.ctx, workID, false, io.Discard)
			if err != nil {
				return workCommandMsg{action: "Run work", workID: workID, err: err}
			}
		}
		return workCommandMsg{action: "Run work", workID: workID}
	}
}

// createReviewTask creates a review task for the currently focused work
func (m *planModel) createReviewTask() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Create review", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Create review", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}

		// Generate task ID for review
		reviewTaskNum, err := m.proj.DB.GetNextTaskNumber(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Create review", workID: workID, err: fmt.Errorf("failed to get next task number: %w", err)}
		}
		reviewTaskID := fmt.Sprintf("%s.%d", workID, reviewTaskNum)

		// Create the review task
		err = m.proj.DB.CreateTask(m.ctx, reviewTaskID, "review", []string{}, 0, workID)
		if err != nil {
			return workCommandMsg{action: "Create review", workID: workID, err: fmt.Errorf("failed to create review task: %w", err)}
		}

		return workCommandMsg{action: "Create review", workID: workID}
	}
}

// createPRTask creates a PR task for the currently focused work
func (m *planModel) createPRTask() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}

		// Check if work is completed
		if work.Status != db.StatusCompleted {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("work %s is not completed (status: %s)", workID, work.Status)}
		}

		// Check if PR already exists
		if work.PRURL != "" {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("PR already exists: %s", work.PRURL)}
		}

		// Generate task ID for PR
		prTaskNum, err := m.proj.DB.GetNextTaskNumber(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("failed to get next task number: %w", err)}
		}
		prTaskID := fmt.Sprintf("%s.%d", workID, prTaskNum)

		// Create the PR task
		err = m.proj.DB.CreateTask(m.ctx, prTaskID, "pr", []string{}, 0, workID)
		if err != nil {
			return workCommandMsg{action: "Create PR", workID: workID, err: fmt.Errorf("failed to create PR task: %w", err)}
		}

		return workCommandMsg{action: "Create PR", workID: workID}
	}
}

// openConsole opens a terminal/console tab for the focused work
func (m *planModel) openConsole() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Open console", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Open console", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}

		// Ensure control plane is running (creates session if needed)
		_, err = control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			return workCommandMsg{action: "Control plane", workID: workID, err: err}
		}

		err = m.workService.OrchestratorManager.OpenConsole(m.ctx, workID, m.proj.Config.Project.Name, work.WorktreePath, work.Name, m.proj.Config.Hooks.Env, io.Discard)
		if err != nil {
			return workCommandMsg{action: "Open console", workID: workID, err: err}
		}

		return workCommandMsg{action: "Open console", workID: workID}
	}
}

// openAgent opens an agent session tab for the focused work
func (m *planModel) openAgent() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Open Agent", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Open Agent", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}

		// Ensure control plane is running (creates session if needed)
		_, err = control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			return workCommandMsg{action: "Control plane", workID: workID, err: err}
		}

		err = m.workService.OrchestratorManager.OpenAgentSession(m.ctx, workID, m.proj.Config.Project.Name, work.WorktreePath, work.Name, m.proj.Config.Hooks.Env, m.proj.Config, io.Discard)
		if err != nil {
			return workCommandMsg{action: "Open Agent", workID: workID, err: err}
		}

		return workCommandMsg{action: "Open Agent", workID: workID}
	}
}

// checkOrchestratorHealth checks if the orchestrator has a recent heartbeat for a work
func checkOrchestratorHealth(ctx context.Context, database *db.DB, workID string) bool {
	// Check if an orchestrator has a recent heartbeat in the database
	alive, err := database.IsOrchestratorAlive(ctx, workID, db.DefaultStalenessThreshold)
	if err != nil {
		return false
	}
	return alive
}

// restartOrchestrator kills and restarts the orchestrator for the focused work
func (m *planModel) restartOrchestrator() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		workRec, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Restart orchestrator", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if workRec == nil {
			return workCommandMsg{action: "Restart orchestrator", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}

		// Kill any existing orchestrator process using pattern-based kill
		// (we use pattern-based kill since we need to actually terminate the process,
		// database check only tells us if it's alive)
		pattern := fmt.Sprintf("sarge orchestrate --work %s", workID)
		if alive := checkOrchestratorHealth(m.ctx, m.proj.DB, workID); alive {
			_ = process.KillProcess(m.ctx, pattern)
			time.Sleep(500 * time.Millisecond)
		}

		// Ensure control plane is running (may have been killed along with zellij)
		if _, err := control.EnsureControlPlane(m.ctx, m.proj); err != nil {
			return workCommandMsg{action: "Restart orchestrator", workID: workID, err: fmt.Errorf("failed to ensure control plane: %w", err)}
		}

		// Spawn a new orchestrator
		spawned, err := m.workService.OrchestratorManager.EnsureWorkOrchestrator(
			m.ctx,
			workID,
			m.proj.Config.Project.Name,
			workRec.WorktreePath,
			workRec.Name,
			io.Discard,
		)
		if err != nil {
			return workCommandMsg{action: "Restart orchestrator", workID: workID, err: err}
		}

		status := "already running"
		if spawned {
			status = "restarted"
		}
		return workCommandMsg{action: fmt.Sprintf("Orchestrator %s", status), workID: workID}
	}
}

// checkPRFeedback triggers an immediate PR feedback check for the focused work
func (m *planModel) checkPRFeedback() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		if err := control.TriggerPRFeedbackCheck(m.ctx, m.proj, workID); err != nil {
			return workCommandMsg{action: "Check PR feedback", workID: workID, err: err}
		}

		// Ensure control plane is running to process the feedback check
		if _, err := control.EnsureControlPlane(m.ctx, m.proj); err != nil {
			return workCommandMsg{action: "PR feedback check triggered", workID: workID, err: fmt.Errorf("feedback check scheduled but control plane failed: %w", err)}
		}

		return workCommandMsg{action: "PR feedback check triggered", workID: workID}
	}
}

// resetSelectedTask resets a failed task to pending status
func (m *planModel) resetSelectedTask() tea.Cmd {
	taskID := m.workDetails.GetSelectedTaskID()
	if taskID == "" {
		return nil
	}
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Reset task status to pending
		if err := m.proj.DB.ResetTaskStatus(m.ctx, taskID); err != nil {
			return workCommandMsg{action: "Reset task", workID: workID, err: err}
		}
		// Reset all bead statuses for this task
		if err := m.proj.DB.ResetTaskBeadStatuses(m.ctx, taskID); err != nil {
			return workCommandMsg{action: "Reset task", workID: workID, err: err}
		}
		return workCommandMsg{action: "Reset task " + taskID, workID: workID}
	}
}

// openIDE opens the worktree in the configured IDE
func (m *planModel) openIDE() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Get work details
		work, err := m.proj.DB.GetWork(m.ctx, workID)
		if err != nil {
			return workCommandMsg{action: "Open IDE", workID: workID, err: fmt.Errorf("failed to get work: %w", err)}
		}
		if work == nil {
			return workCommandMsg{action: "Open IDE", workID: workID, err: fmt.Errorf("work %s not found", workID)}
		}
		if work.WorktreePath == "" {
			return workCommandMsg{action: "Open IDE", workID: workID, err: fmt.Errorf("worktree path not set")}
		}

		// Get IDE command from config
		ideCmd := m.proj.Config.IDE.GetIDECommand()
		if ideCmd == "" {
			return workCommandMsg{action: "Open IDE", workID: workID, err: fmt.Errorf("IDE not configured (set [ide] command in config or EDITOR env var)")}
		}

		// Build the command arguments
		args := append(m.proj.Config.IDE.Args, work.WorktreePath)

		// Execute IDE as a detached process
		if err := process.StartDetached(m.ctx, ideCmd, args...); err != nil {
			return workCommandMsg{action: "Open IDE", workID: workID, err: fmt.Errorf("failed to start IDE: %w", err)}
		}

		return workCommandMsg{action: "Opened in IDE", workID: workID}
	}
}

// attachTerminal opens a new terminal window attached to a zmx session based on the current
// work details selection. Only works when multiplexer type is "zmx".
func (m *planModel) attachTerminal() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		// Check if multiplexer is zmx
		if !m.proj.Config.Multiplexer.IsZmx() {
			return workCommandMsg{action: "Attach terminal", workID: workID, err: fmt.Errorf("attach terminal requires [multiplexer] type = \"zmx\" in config")}
		}

		if workID == "" {
			return workCommandMsg{action: "Attach terminal", workID: workID, err: fmt.Errorf("no work focused")}
		}

		projectName := m.proj.Config.Project.Name

		// Determine the zmx session name based on the current work details selection
		// Default: orchestrator session for the focused work
		tabName := fmt.Sprintf("work-%s", workID)

		// Check if a specific task is selected
		if m.workDetails.IsTaskSelected() {
			taskID := m.workDetails.GetSelectedTaskID()
			if taskID != "" {
				tabName = fmt.Sprintf("task-%s", taskID)
			}
		}

		sessionName := zmx.SessionName(projectName, tabName)

		// Build the terminal launch command from config
		cmdTemplate := m.proj.Config.Multiplexer.GetTerminalCommand()
		cmdStr := strings.ReplaceAll(cmdTemplate, "{session}", sessionName)

		// Parse the command string into parts
		parts := strings.Fields(cmdStr)
		if len(parts) == 0 {
			return workCommandMsg{action: "Attach terminal", workID: workID, err: fmt.Errorf("empty terminal command")}
		}

		// Fire-and-forget: start the terminal process
		cmd := exec.Command(parts[0], parts[1:]...) //nolint:gosec // Command comes from user config
		if err := cmd.Start(); err != nil {
			return workCommandMsg{action: "Attach terminal", workID: workID, err: fmt.Errorf("failed to launch terminal: %w", err)}
		}

		return workCommandMsg{action: fmt.Sprintf("Attached terminal to %s", sessionName), workID: workID}
	}
}
