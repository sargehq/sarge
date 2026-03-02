// Package taskseq provides an in-process task sequencer that replaces the
// orchestrate command. It watches all works for ready tasks and executes them
// via bridge sessions, handling post-task logic (post-estimation, review loops)
// inline.
package taskseq

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/sargehq/sarge/internal/agents"
	agenttypes "github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/bridge"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/orchestration"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/work"
)

// Sequencer watches all works for ready tasks and executes them via bridge sessions.
// It replaces the per-work orchestrator process with a single in-process goroutine.
type Sequencer struct {
	proj   *project.Project
	bridge *bridge.Bridge

	mu          sync.Mutex
	activeTasks map[string]string // workID → taskID currently executing

	// done is closed when the sequencer loop exits.
	done chan struct{}

	// notify is used to wake the sequencer when DB changes occur.
	notify chan struct{}
}

// New creates a new Sequencer. The bridge is shared with the TUI for session viewing.
func New(proj *project.Project, b *bridge.Bridge) *Sequencer {
	return &Sequencer{
		proj:        proj,
		bridge:      b,
		activeTasks: make(map[string]string),
		done:        make(chan struct{}),
		notify:      make(chan struct{}, 1),
	}
}

// Notify wakes the sequencer to check for new ready tasks.
// Safe to call from any goroutine; non-blocking.
func (s *Sequencer) Notify() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Run starts the main sequencer loop. It blocks until ctx is cancelled.
// Call Notify() when the DB changes to trigger immediate task checks.
func (s *Sequencer) Run(ctx context.Context) error {
	defer close(s.done)

	logging.Info("Task sequencer started")

	// Reset stuck processing tasks on startup for all active works
	s.resetAllStuckTasks(ctx)

	// Periodic safety-net timer
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logging.Info("Task sequencer stopping")
			s.killAllActiveSessions()
			return nil
		case <-s.notify:
			s.poll(ctx)
		case <-ticker.C:
			s.poll(ctx)
		}
	}
}

// Wait blocks until the sequencer loop exits.
func (s *Sequencer) Wait() {
	<-s.done
}

// ActiveTasks returns a snapshot of currently executing tasks (workID → taskID).
func (s *Sequencer) ActiveTasks() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string, len(s.activeTasks))
	for k, v := range s.activeTasks {
		result[k] = v
	}
	return result
}

// poll checks all works for ready tasks and starts execution for any that are idle.
func (s *Sequencer) poll(ctx context.Context) {
	works, err := s.proj.DB.ListWorks(ctx, "")
	if err != nil {
		logging.Warn("sequencer: failed to list works", "error", err)
		return
	}

	for _, w := range works {
		// Skip destroyed/completed works
		if w.Status == db.StatusCompleted {
			continue
		}

		s.mu.Lock()
		_, hasActive := s.activeTasks[w.ID]
		s.mu.Unlock()

		if hasActive {
			continue // already executing a task for this work
		}

		s.tryStartNextTask(ctx, w)
	}
}

// tryStartNextTask picks the next ready task for a work and starts it.
func (s *Sequencer) tryStartNextTask(ctx context.Context, w *db.Work) {
	readyTasks, err := s.proj.DB.GetReadyTasksForWork(ctx, w.ID)
	if err != nil {
		logging.Warn("sequencer: failed to get ready tasks", "error", err, "work_id", w.ID)
		return
	}

	if len(readyTasks) == 0 {
		// Check if we should transition work to idle/failed
		s.checkWorkStatus(ctx, w)
		return
	}

	t := readyTasks[0]

	// Mark this work as having an active task
	s.mu.Lock()
	s.activeTasks[w.ID] = t.ID
	s.mu.Unlock()

	// Execute in background goroutine
	go s.executeTask(ctx, w, t)
}

// executeTask runs a single task via bridge session and handles post-task logic.
func (s *Sequencer) executeTask(ctx context.Context, w *db.Work, t *db.Task) {
	defer func() {
		s.mu.Lock()
		delete(s.activeTasks, w.ID)
		s.mu.Unlock()
		// Wake up to check for next task
		s.Notify()
	}()

	logging.Info("Executing task via bridge",
		"task_id", t.ID,
		"task_type", t.TaskType,
		"work_id", w.ID)

	// Update activity
	if err := s.proj.DB.UpdateTaskActivity(ctx, t.ID, time.Now()); err != nil {
		logging.Warn("failed to update task activity", "error", err, "task_id", t.ID)
	}

	// Set up automated workflow if needed (auto work with no tasks yet)
	if w.Auto {
		tasks, err := s.proj.DB.GetWorkTasks(ctx, w.ID)
		if err == nil && len(tasks) == 0 {
			logging.Info("Setting up automated workflow", "work_id", w.ID)
			workSvc := work.NewWorkService(s.proj)
			if err := workSvc.CreateEstimateTaskFromWorkBeans(ctx, w.ID, os.Stdout); err != nil {
				logging.Warn("failed to create estimate task", "error", err, "work_id", w.ID)
			}
			return // Let the next poll pick up the new estimate task
		}
	}

	// Build task input
	taskInput, err := s.buildTaskInput(ctx, t, w)
	if err != nil {
		logging.Error("Failed to build task input", "error", err, "task_id", t.ID)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("failed to build task input: %v", err)); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr, "task_id", t.ID)
		}
		return
	}

	// Clean up temp file after execution
	if taskInput.TempFilePath != "" {
		defer func() { _ = os.Remove(taskInput.TempFilePath) }()
	}

	// Build prompt
	agent, err := agents.NewAgent(s.proj.Config)
	if err != nil {
		logging.Error("Failed to create agent", "error", err)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("failed to create agent: %v", err)); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr)
		}
		return
	}

	prompt, err := agent.BuildPrompt(taskInput.Params)
	if err != nil {
		logging.Error("Failed to build prompt", "error", err, "task_id", t.ID)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("failed to build prompt: %v", err)); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr)
		}
		return
	}

	// Mark task as started
	if err := s.proj.DB.StartTask(ctx, t.ID, w.WorktreePath); err != nil {
		logging.Error("Failed to start task", "error", err, "task_id", t.ID)
		return
	}

	// Set up timeout
	timeout := s.proj.Config.Workflow.GetTaskTimeout()
	taskCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Spawn bridge session
	sessionID := fmt.Sprintf("task-%s", t.ID)
	sessionCfg := bridge.SessionConfigFromProject(w.WorktreePath, s.proj.Config)
	sessionCfg.Env = append(sessionCfg.Env, fmt.Sprintf("SARGE_TASK_ID=%s", t.ID))
	sessionCfg.Env = append(sessionCfg.Env, fmt.Sprintf("BEANS_PATH=%s", s.proj.BeansPath()))

	// Apply hooks.env
	for _, e := range s.proj.Config.Hooks.Env {
		expandedValue := os.ExpandEnv(e)
		sessionCfg.Env = append(sessionCfg.Env, expandedValue)
	}

	// Use task-specific args (e.g. model override for log_analysis)
	if t.TaskType == "log_analysis" && s.proj.Config.LogParser.GetModel() != "" {
		sessionCfg.Model = s.proj.Config.LogParser.GetModel()
	}

	session, err := s.bridge.SpawnSession(sessionID, sessionCfg)
	if err != nil {
		logging.Error("Failed to spawn bridge session", "error", err, "task_id", t.ID)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("failed to spawn bridge session: %v", err)); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr)
		}
		return
	}

	// Send prompt
	if err := session.Prompt(prompt); err != nil {
		logging.Error("Failed to send prompt", "error", err, "task_id", t.ID)
		_ = s.bridge.KillSession(sessionID)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("failed to send prompt: %v", err)); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr)
		}
		return
	}

	// Wait for agent to complete
	startTime := time.Now()
	err = s.waitForCompletion(taskCtx, session, t.ID)

	// Clean up session
	_ = s.bridge.KillSession(sessionID)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logging.Warn("Task timed out", "task_id", t.ID, "timeout", timeout)
			if dbErr := s.proj.DB.FailTask(context.Background(), t.ID, fmt.Sprintf("task timed out after %v", timeout)); dbErr != nil {
				logging.Warn("failed to mark timed out task as failed", "error", dbErr)
			}
			return
		}
		if errors.Is(err, context.Canceled) {
			logging.Info("Task cancelled", "task_id", t.ID)
			return
		}
		// Agent exited with error - check if task was already marked complete/failed
		updatedTask, dbErr := s.proj.DB.GetTask(ctx, t.ID)
		if dbErr == nil && updatedTask != nil && (updatedTask.Status == db.StatusCompleted || updatedTask.Status == db.StatusFailed) {
			elapsed := time.Since(startTime)
			logging.Info("Task finished", "task_id", t.ID, "status", updatedTask.Status, "elapsed", elapsed.Round(time.Second))
		} else {
			logging.Error("Agent exited with error", "error", err, "task_id", t.ID)
			if dbErr := s.proj.DB.FailTask(ctx, t.ID, fmt.Sprintf("agent exited with error: %v", err)); dbErr != nil {
				logging.Warn("failed to mark task as failed", "error", dbErr)
			}
			return
		}
	}

	// Check final task status
	updatedTask, err := s.proj.DB.GetTask(ctx, t.ID)
	if err != nil {
		logging.Warn("failed to re-read task status", "error", err, "task_id", t.ID)
		return
	}

	// Auto-complete log_analysis tasks if agent exited without calling sarge complete
	if t.TaskType == "log_analysis" && updatedTask != nil && updatedTask.Status == db.StatusProcessing {
		logging.Warn("auto-completing log_analysis task that the agent did not mark complete", "task_id", t.ID)
		if err := s.proj.DB.CompleteTask(ctx, t.ID, ""); err != nil {
			logging.Warn("failed to auto-complete task", "error", err, "task_id", t.ID)
		}
	}

	// Agent exited cleanly but never called sarge complete — mark as failed
	if updatedTask != nil && updatedTask.Status == db.StatusProcessing {
		logging.Warn("agent exited without completing task, marking as failed", "task_id", t.ID)
		if dbErr := s.proj.DB.FailTask(ctx, t.ID, "agent exited without completing task"); dbErr != nil {
			logging.Warn("failed to mark task as failed", "error", dbErr)
		}
	}

	// Post-execution handling
	switch t.TaskType {
	case "estimate":
		if err := s.handlePostEstimation(ctx, t, w); err != nil {
			logging.Error("Failed post-estimation handling", "error", err, "task_id", t.ID)
		}
	case "review":
		if err := s.handleReviewFixLoop(ctx, t, w); err != nil {
			logging.Error("Failed review fix loop handling", "error", err, "task_id", t.ID)
		}
	}

	elapsed := time.Since(startTime)
	logging.Info("Task execution complete",
		"task_id", t.ID,
		"task_type", t.TaskType,
		"elapsed", elapsed.Round(time.Second))
}

// waitForCompletion blocks until the bridge session ends or context is cancelled.
func (s *Sequencer) waitForCompletion(ctx context.Context, session *bridge.Session, _ string) error {
	for {
		select {
		case <-ctx.Done():
			_ = session.Abort()
			return ctx.Err()
		case evt, ok := <-session.Events():
			if !ok {
				// Session events channel closed = process exited
				return session.Err()
			}
			if evt.Type == bridge.EventAgentEnd {
				// Agent finished - wait for process to exit
				return session.Wait()
			}
		}
	}
}

// checkWorkStatus transitions work to idle/failed based on task statuses.
func (s *Sequencer) checkWorkStatus(ctx context.Context, w *db.Work) {
	allTasks, err := s.proj.DB.GetWorkTasks(ctx, w.ID)
	if err != nil {
		return
	}

	pendingCount, processingCount, failedCount, completedCount := 0, 0, 0, 0
	for _, t := range allTasks {
		switch t.Status {
		case db.StatusPending:
			pendingCount++
		case db.StatusProcessing:
			processingCount++
		case db.StatusFailed:
			failedCount++
		case db.StatusCompleted:
			completedCount++
		}
	}

	// Still running/blocked - nothing to do
	if processingCount > 0 || pendingCount > 0 {
		return
	}

	// Mark failed if there are failures
	if failedCount > 0 && w.Status != db.StatusFailed {
		if err := s.proj.DB.FailWork(ctx, w.ID, fmt.Sprintf("%d task(s) failed", failedCount)); err != nil {
			logging.Warn("failed to mark work as failed", "error", err, "work_id", w.ID)
		}
		return
	}

	// All tasks completed - transition to idle
	if completedCount > 0 && w.Status != db.StatusIdle {
		prURL := w.PRURL
		if prURL == "" {
			for _, t := range allTasks {
				if t.TaskType == "pr" && t.Status == db.StatusCompleted && t.PRURL != "" {
					prURL = t.PRURL
					break
				}
			}
		}
		if err := s.proj.DB.IdleWorkWithPR(ctx, w.ID, prURL); err != nil {
			logging.Warn("failed to mark work as idle", "error", err, "work_id", w.ID)
		}
	}
}

// resetAllStuckTasks resets stuck processing tasks for all works.
func (s *Sequencer) resetAllStuckTasks(ctx context.Context) {
	works, err := s.proj.DB.ListWorks(ctx, "")
	if err != nil {
		logging.Warn("sequencer: failed to list works for reset", "error", err)
		return
	}

	for _, w := range works {
		if err := orchestration.ResetStuckProcessingTasks(ctx, s.proj, w.ID); err != nil {
			logging.Warn("failed to reset stuck tasks", "error", err, "work_id", w.ID)
		}
	}
}

// killAllActiveSessions kills bridge sessions for all active tasks.
func (s *Sequencer) killAllActiveSessions() {
	s.mu.Lock()
	tasks := make(map[string]string, len(s.activeTasks))
	for k, v := range s.activeTasks {
		tasks[k] = v
	}
	s.mu.Unlock()

	for _, taskID := range tasks {
		sessionID := fmt.Sprintf("task-%s", taskID)
		_ = s.bridge.KillSession(sessionID)
	}
}

// TaskInput contains the params and any associated resources for a task.
type TaskInput struct {
	Params       agenttypes.TaskParams
	TempFilePath string
}

// buildTaskInput creates the TaskInput for a task. This mirrors cmd/task_processing.go's
// taskInputForTask but lives in the sequencer package to avoid circular imports.
func (s *Sequencer) buildTaskInput(ctx context.Context, t *db.Task, w *db.Work) (*TaskInput, error) {
	baseBranch := w.BaseBranch
	if baseBranch == "" {
		baseBranch = s.proj.Config.Repo.GetBaseBranch()
	}

	if t.TaskType == "log_analysis" {
		return s.logAnalysisInput(ctx, t, w)
	}

	params := agenttypes.TaskParams{
		TaskID:     t.ID,
		WorkID:     w.ID,
		BranchName: w.BranchName,
		BaseBranch: baseBranch,
		BeansPath:  s.proj.BeansPath(),
	}

	switch t.TaskType {
	case "estimate":
		beanIDs, err := s.beanIDsForTask(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		params.Type = agenttypes.TaskTypeEstimate
		params.BeanIDs = beanIDs

	case "implement":
		beanIDs, err := s.beanIDsForTask(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		params.Type = agenttypes.TaskTypeImplement
		params.BeanIDs = beanIDs

	case "review":
		params.Type = agenttypes.TaskTypeReview
		params.RootIssueID = w.RootIssueID

	case "pr":
		params.Type = agenttypes.TaskTypePR

	case "update-pr-description":
		if w.PRURL == "" {
			return nil, fmt.Errorf("work %s has no PR URL set", w.ID)
		}
		params.Type = agenttypes.TaskTypeUpdatePRDescription
		params.PRURL = w.PRURL

	default:
		return nil, fmt.Errorf("unknown task type: %s", t.TaskType)
	}

	return &TaskInput{Params: params}, nil
}

func (s *Sequencer) beanIDsForTask(ctx context.Context, taskID string) ([]string, error) {
	beanIDs, err := s.proj.DB.GetTaskBeans(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}
	return beanIDs, nil
}

func (s *Sequencer) logAnalysisInput(ctx context.Context, t *db.Task, w *db.Work) (*TaskInput, error) {
	workflowName, _ := s.proj.DB.GetTaskMetadata(ctx, t.ID, "workflow_name")
	jobName, _ := s.proj.DB.GetTaskMetadata(ctx, t.ID, "job_name")
	branchName, _ := s.proj.DB.GetTaskMetadata(ctx, t.ID, "branch_name")
	if branchName == "" {
		branchName = w.BranchName
	}
	rootIssueID, _ := s.proj.DB.GetTaskMetadata(ctx, t.ID, "root_issue_id")
	if rootIssueID == "" {
		rootIssueID = w.RootIssueID
	}

	logContent, err := s.proj.DB.GetTaskMetadata(ctx, t.ID, "log_content")
	if err != nil || logContent == "" {
		return nil, fmt.Errorf("log_content metadata is missing for task %s", t.ID)
	}

	logFile, err := os.CreateTemp("", "ci-log-*.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	if _, err := logFile.WriteString(logContent); err != nil {
		_ = logFile.Close()
		_ = os.Remove(logFile.Name())
		return nil, fmt.Errorf("failed to write log content: %w", err)
	}
	_ = logFile.Close()

	return &TaskInput{
		Params: agenttypes.TaskParams{
			Type:         agenttypes.TaskTypeLogAnalysis,
			TaskID:       t.ID,
			WorkID:       w.ID,
			BranchName:   branchName,
			RootIssueID:  rootIssueID,
			WorkflowName: workflowName,
			JobName:      jobName,
			LogFilePath:  logFile.Name(),
			BeansPath:    s.proj.BeansPath(),
		},
		TempFilePath: logFile.Name(),
	}, nil
}

// handlePostEstimation creates implement, review, and PR tasks after estimation completes.
func (s *Sequencer) handlePostEstimation(ctx context.Context, estimateTask *db.Task, w *db.Work) error {
	logging.Info("Creating post-estimation tasks", "task_id", estimateTask.ID, "work_id", w.ID)

	beanIDs, err := s.proj.DB.GetTaskBeans(ctx, estimateTask.ID)
	if err != nil {
		return fmt.Errorf("failed to get task beans: %w", err)
	}
	if len(beanIDs) == 0 {
		return fmt.Errorf("no beans found for estimate task %s", estimateTask.ID)
	}

	issuesResult, err := s.proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		return fmt.Errorf("failed to get bean details: %w", err)
	}

	for _, beanID := range beanIDs {
		if _, found := issuesResult.Beans[beanID]; !found {
			return fmt.Errorf("bean %s not found", beanID)
		}
	}

	beanList := make([]beans.Bean, 0, len(issuesResult.Beans))
	for _, b := range issuesResult.Beans {
		beanList = append(beanList, b)
	}

	estimator := task.NewLLMEstimator(s.proj.DB, w.WorktreePath, s.proj.Config.Project.Name, w.ID)
	planner := task.NewDefaultPlanner(estimator)

	const tokenBudget = 120000
	tasks, err := planner.Plan(ctx, beanList, issuesResult.Dependencies, tokenBudget)
	if err != nil {
		return fmt.Errorf("failed to plan tasks: %w", err)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("planner returned no tasks for %d beans", len(beanIDs))
	}

	var implementTaskIDs []string
	beanToTask := make(map[string]string)
	for _, t := range tasks {
		nextNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
		if err != nil {
			return fmt.Errorf("failed to get next task number: %w", err)
		}
		taskID := fmt.Sprintf("%s.%d", w.ID, nextNum)

		if err := s.proj.DB.CreateTask(ctx, taskID, "implement", t.BeanIDs, t.Complexity, w.ID, nextNum); err != nil {
			return fmt.Errorf("failed to create implement task: %w", err)
		}
		if err := s.proj.DB.AddTaskDependency(ctx, taskID, estimateTask.ID); err != nil {
			return fmt.Errorf("failed to add dependency for %s: %w", taskID, err)
		}
		for _, beanID := range t.BeanIDs {
			beanToTask[beanID] = taskID
		}
		implementTaskIDs = append(implementTaskIDs, taskID)
		logging.Info("Created implement task", "task_id", taskID, "complexity", t.Complexity, "beans", t.BeanIDs)
	}

	// Compute inter-task dependencies from bean dependencies
	interTaskDeps := make(map[string]map[string]bool)
	for beanID, deps := range issuesResult.Dependencies {
		taskID, ok := beanToTask[beanID]
		if !ok {
			continue
		}
		for _, dep := range deps {
			depTaskID, ok := beanToTask[dep.BlockedByID]
			if !ok || taskID == depTaskID {
				continue
			}
			if interTaskDeps[taskID] == nil {
				interTaskDeps[taskID] = make(map[string]bool)
			}
			interTaskDeps[taskID][depTaskID] = true
		}
	}
	for taskID, depTaskIDs := range interTaskDeps {
		for depTaskID := range depTaskIDs {
			if err := s.proj.DB.AddTaskDependency(ctx, taskID, depTaskID); err != nil {
				return fmt.Errorf("failed to add inter-task dependency %s -> %s: %w", taskID, depTaskID, err)
			}
		}
	}

	// Create review task
	reviewTaskNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for review: %w", err)
	}
	reviewTaskID := fmt.Sprintf("%s.%d", w.ID, reviewTaskNum)
	if err := s.proj.DB.CreateTask(ctx, reviewTaskID, "review", nil, 0, w.ID, reviewTaskNum); err != nil {
		return fmt.Errorf("failed to create review task: %w", err)
	}
	for _, implID := range implementTaskIDs {
		if err := s.proj.DB.AddTaskDependency(ctx, reviewTaskID, implID); err != nil {
			return fmt.Errorf("failed to add dependency for review: %w", err)
		}
	}
	logging.Info("Created review task", "task_id", reviewTaskID, "depends_on", len(implementTaskIDs))

	return nil
}

// handleReviewFixLoop checks if a review task found issues and creates fix tasks.
func (s *Sequencer) handleReviewFixLoop(ctx context.Context, reviewTask *db.Task, w *db.Work) error {
	// Check if manual review
	autoWorkflow, err := s.proj.DB.GetTaskMetadata(ctx, reviewTask.ID, "auto_workflow")
	if err == nil && autoWorkflow == "false" {
		return nil
	}

	// Count review iterations
	reviewCount := orchestration.CountReviewIterations(ctx, s.proj.DB, w.ID)
	maxIterations := s.proj.Config.Workflow.GetMaxReviewIterations()
	if reviewCount >= maxIterations {
		logging.Warn("Maximum review iterations reached, proceeding to PR", "work_id", w.ID, "count", reviewCount)
		return s.createPRTask(ctx, w, reviewTask.ID)
	}

	// Check for issue beans from review
	var beansToFix []beans.Bean
	if w.RootIssueID != "" {
		rootChildrenIssues, err := s.proj.Beans.GetBeanWithChildren(ctx, w.RootIssueID)
		if err != nil {
			return fmt.Errorf("failed to get children of root issue %s: %w", w.RootIssueID, err)
		}

		expectedExternalRef := fmt.Sprintf("review-%s", reviewTask.ID)
		for _, issue := range rootChildrenIssues {
			if issue.ID != w.RootIssueID &&
				beans.IsWorkableStatus(issue.Status) &&
				beans.HasTagValue(issue.Tags, expectedExternalRef) {
				beansToFix = append(beansToFix, issue)
			}
		}
	}

	// If no review issues, check PR feedback
	if len(beansToFix) == 0 && w.PRURL != "" {
		processor := feedback.NewProcessor()
		_, err := processor.ProcessPRFeedback(ctx, s.proj, s.proj.DB, w.ID)
		if err != nil {
			logging.Warn("failed to check PR feedback", "error", err, "work_id", w.ID)
		} else if w.RootIssueID != "" {
			rootChildrenIssues, err := s.proj.Beans.GetBeanWithChildren(ctx, w.RootIssueID)
			if err == nil {
				for _, issue := range rootChildrenIssues {
					if issue.ID != w.RootIssueID && beans.IsWorkableStatus(issue.Status) {
						inTask, _ := s.proj.DB.IsBeanInTask(ctx, w.ID, issue.ID)
						if !inTask {
							beansToFix = append(beansToFix, issue)
						}
					}
				}
			}
		}
	}

	if len(beansToFix) == 0 {
		logging.Info("Review passed, creating PR task", "work_id", w.ID)
		return s.createPRTask(ctx, w, reviewTask.ID)
	}

	logging.Info("Review found issues, creating fix tasks", "work_id", w.ID, "count", len(beansToFix))

	var fixTaskIDs []string
	for _, b := range beansToFix {
		nextNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
		if err != nil {
			return fmt.Errorf("failed to get next task number: %w", err)
		}
		taskID := fmt.Sprintf("%s.%d", w.ID, nextNum)

		if err := s.proj.DB.CreateTask(ctx, taskID, "implement", []string{b.ID}, 0, w.ID, nextNum); err != nil {
			return fmt.Errorf("failed to create fix task: %w", err)
		}
		if err := s.proj.DB.AddTaskDependency(ctx, taskID, reviewTask.ID); err != nil {
			return fmt.Errorf("failed to add dependency for fix task %s: %w", taskID, err)
		}
		fixTaskIDs = append(fixTaskIDs, taskID)
	}

	// Create new review task
	newReviewTaskNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for review: %w", err)
	}
	newReviewTaskID := fmt.Sprintf("%s.%d", w.ID, newReviewTaskNum)
	if err := s.proj.DB.CreateTask(ctx, newReviewTaskID, "review", nil, 0, w.ID, newReviewTaskNum); err != nil {
		return fmt.Errorf("failed to create new review task: %w", err)
	}
	for _, fixID := range fixTaskIDs {
		if err := s.proj.DB.AddTaskDependency(ctx, newReviewTaskID, fixID); err != nil {
			return fmt.Errorf("failed to add dependency for new review task: %w", err)
		}
	}

	return nil
}

// createPRTask creates the PR task (or update-pr-description) that depends on a review task.
func (s *Sequencer) createPRTask(ctx context.Context, w *db.Work, reviewTaskID string) error {
	// Check for existing PR task
	existingPRTask, err := s.proj.DB.GetPRTaskForWork(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("failed to check for existing PR task: %w", err)
	}

	if existingPRTask != nil {
		switch existingPRTask.Status {
		case db.StatusPending, db.StatusProcessing:
			return nil // Already exists
		case db.StatusCompleted:
			return s.createUpdatePRDescriptionTask(ctx, w, reviewTaskID)
		}
	}

	prTaskNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number for PR: %w", err)
	}
	prTaskID := fmt.Sprintf("%s.%d", w.ID, prTaskNum)

	if err := s.proj.DB.CreateTask(ctx, prTaskID, "pr", nil, 0, w.ID, prTaskNum); err != nil {
		return fmt.Errorf("failed to create PR task: %w", err)
	}
	if err := s.proj.DB.AddTaskDependency(ctx, prTaskID, reviewTaskID); err != nil {
		return fmt.Errorf("failed to add dependency for PR: %w", err)
	}
	return nil
}

func (s *Sequencer) createUpdatePRDescriptionTask(ctx context.Context, w *db.Work, reviewTaskID string) error {
	taskNum, err := s.proj.DB.GetNextTaskNumber(ctx, w.ID)
	if err != nil {
		return fmt.Errorf("failed to get next task number: %w", err)
	}
	taskID := fmt.Sprintf("%s.%d", w.ID, taskNum)

	if err := s.proj.DB.CreateTask(ctx, taskID, "update-pr-description", nil, 0, w.ID, taskNum); err != nil {
		return fmt.Errorf("failed to create update-pr-description task: %w", err)
	}
	if err := s.proj.DB.AddTaskDependency(ctx, taskID, reviewTaskID); err != nil {
		return fmt.Errorf("failed to add dependency: %w", err)
	}
	return nil
}
