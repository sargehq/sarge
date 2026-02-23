package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/process"
	"github.com/sargehq/sarge/internal/progress"

	workpkg "github.com/sargehq/sarge/internal/work"
)

// sessionName returns the zellij session name for this project
func (m *planModel) sessionName() string {
	return fmt.Sprintf("sarge-%s", m.proj.Config.Project.Name)
}

// spawnPlanSession spawns or resumes a planning session for a specific bean
func (m *planModel) spawnPlanSession(beanID string) tea.Cmd {
	return func() tea.Msg {
		tabName := workpkg.PlanTabName(beanID)
		mainRepoPath := m.proj.MainRepoPath()

		logging.Debug("spawnPlanSession started", "beanID", beanID, "tabName", tabName)

		// Check if session already running for this bean
		running, _ := m.proj.DB.IsPlanSessionRunning(m.ctx, beanID)
		logging.Debug("spawnPlanSession checked if running", "beanID", beanID, "running", running)
		if running {
			// Session exists - for zellij, switch to it; for zmx, it's a no-op (attach handled separately)
			if !m.proj.Config.Multiplexer.IsZmx() {
				zellijSession := m.sessionName()
				if err := m.zj.Session(zellijSession).SwitchToTab(m.ctx, tabName); err != nil {
					return planSessionSpawnedMsg{beanID: beanID, err: err}
				}
			}
			return planSessionSpawnedMsg{beanID: beanID, resumed: true}
		}

		// Ensure control plane is running
		sessionResult, err := control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			logging.Error("spawnPlanSession EnsureControlPlane failed", "beanID", beanID, "error", err)
			return planSessionSpawnedMsg{beanID: beanID, err: err}
		}
		logging.Debug("spawnPlanSession EnsureControlPlane completed",
			"beanID", beanID,
			"sessionCreated", sessionResult.SessionCreated,
			"sessionName", sessionResult.SessionName)

		// Use the orchestrator manager to spawn the plan session
		if err := m.workService.OrchestratorManager.SpawnPlanSession(m.ctx, beanID, m.proj.Config.Project.Name, mainRepoPath, io.Discard); err != nil {
			logging.Error("spawnPlanSession SpawnPlanSession failed", "beanID", beanID, "error", err)
			return planSessionSpawnedMsg{beanID: beanID, err: err}
		}

		msg := planSessionSpawnedMsg{beanID: beanID, resumed: false}
		if sessionResult.SessionCreated {
			msg.sessionCreated = true
			msg.sessionName = sessionResult.SessionName
		}
		logging.Debug("spawnPlanSession completed", "beanID", beanID, "sessionCreated", msg.sessionCreated, "sessionName", msg.sessionName)
		return msg
	}
}

// executeCreateWork creates a work unit with the given branch name.
// This uses the shared CreateWorkFromBean method which handles:
// 1. Expanding the bean to collect all issue IDs
// 2. Creating work record in DB (with auto flag)
// 3. Initializing the zellij session
// 4. Ensuring control plane is running
func (m *planModel) executeCreateWork(beanID string, branchName string, auto bool, useExistingBranch bool) tea.Cmd {
	return func() tea.Msg {
		logging.Debug("executeCreateWork started", "beanID", beanID, "branchName", branchName, "auto", auto, "useExistingBranch", useExistingBranch)

		opts := workpkg.CreateWorkFromBeanOptions{
			BeanID:            beanID,
			BranchName:        branchName,
			BaseBranch:        m.proj.Config.Repo.GetBaseBranch(),
			Auto:              auto,
			UseExistingBranch: useExistingBranch,
		}
		result, err := m.workService.CreateWorkFromBean(m.ctx, opts)
		if err != nil {
			logging.Error("executeCreateWork CreateWorkFromBean failed", "beanID", beanID, "error", err)
			return planWorkCreatedMsg{beanID: beanID, err: err}
		}
		logging.Debug("executeCreateWork completed successfully", "workID", result.WorkID)

		// Ensure control plane is running to process the work
		sessionResult, err := control.EnsureControlPlane(m.ctx, m.proj)
		if err != nil {
			logging.Warn("executeCreateWork EnsureControlPlane failed", "error", err)
			// Non-fatal: work was created but control plane might need manual start
			return planWorkCreatedMsg{beanID: beanID, workID: result.WorkID, err: err}
		}

		msg := planWorkCreatedMsg{beanID: beanID, workID: result.WorkID}
		if sessionResult.SessionCreated {
			msg.sessionCreated = true
			msg.sessionName = sessionResult.SessionName
		}
		return msg
	}
}

func (m *planModel) addBeansToWork(beanIDs []string, workID string) tea.Cmd {
	return func() tea.Msg {
		// Use WorkService to add beans
		_, err := m.workService.AddBeans(m.ctx, workID, beanIDs)
		if err != nil {
			beanIDsStr := strings.Join(beanIDs, ", ")
			return beanAddedToWorkMsg{beanID: beanIDsStr, workID: workID, err: fmt.Errorf("failed to add issues to work: %w", err)}
		}

		beanIDsStr := strings.Join(beanIDs, ", ")
		return beanAddedToWorkMsg{beanID: beanIDsStr, workID: workID}
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
			// Use direct mode - creates one task per bean
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
		err = m.proj.DB.CreateTask(m.ctx, reviewTaskID, "review", []string{}, 0, workID, reviewTaskNum)
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
		err = m.proj.DB.CreateTask(m.ctx, prTaskID, "pr", []string{}, 0, workID, prTaskNum)
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

		if spawned {
			return workCommandMsg{action: "Orchestrator spawned", workID: workID}
		}
		return workCommandMsg{action: "Orchestrator already running", workID: workID}
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
		// Reset all bean statuses for this task
		if err := m.proj.DB.ResetTaskBeanStatuses(m.ctx, taskID); err != nil {
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

// listZmxSessions fires an async command to list zmx sessions for the focused work.
func (m *planModel) listZmxSessions() tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		if !m.proj.Config.Multiplexer.IsZmx() {
			return zmxSessionsLoadedMsg{err: fmt.Errorf("session picker requires [multiplexer] type = \"zmx\" in config")}
		}
		if workID == "" {
			return zmxSessionsLoadedMsg{err: fmt.Errorf("no work focused")}
		}

		sessions, err := m.workService.OrchestratorManager.ListWorkSessions(m.ctx, workID, m.proj.Config.Project.Name)
		return zmxSessionsLoadedMsg{sessions: sessions, err: err}
	}
}

// attachToZmxSession fires an async command to attach to a specific zmx session.
func (m *planModel) attachToZmxSession(sessionName string) tea.Cmd {
	workID := m.focusedWorkID
	return func() tea.Msg {
		err := m.workService.OrchestratorManager.AttachToSession(m.ctx, sessionName)
		if err != nil {
			return zmxSessionAttachedMsg{sessionName: sessionName, err: err}
		}
		return workCommandMsg{action: "Attached to session", workID: workID}
	}
}
