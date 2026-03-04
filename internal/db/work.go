package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sargehq/sarge/internal/db/sqlc"
)

// workToLocal converts an sqlc.Work to local Work
func workToLocal(w *sqlc.Work) *Work {
	work := &Work{
		ID:                 w.ID,
		Status:             w.Status,
		Name:               w.Name,
		WorktreePath:       w.WorktreePath,
		BranchName:         w.BranchName,
		BaseBranch:         w.BaseBranch,
		RootIssueID:        w.RootIssueID,
		PRURL:              w.PrUrl,
		ErrorMessage:       w.ErrorMessage,
		CreatedAt:          w.CreatedAt,
		Auto:               w.Auto,
		CIStatus:           w.CiStatus,
		ApprovalStatus:     w.ApprovalStatus,
		Approvers:          w.Approvers,
		HasUnseenPRChanges: w.HasUnseenPrChanges,
		PRState:            w.PrState,
		MergeableState:     w.MergeableState,
	}
	if w.StartedAt.Valid {
		work.StartedAt = &w.StartedAt.Time
	}
	if w.CompletedAt.Valid {
		work.CompletedAt = &w.CompletedAt.Time
	}
	if w.LastPrPollAt.Valid {
		work.LastPRPollAt = &w.LastPrPollAt.Time
	}
	return work
}

// Work represents a work unit (group of tasks) in the database.
type Work struct {
	ID                 string
	Status             string
	Name               string
	WorktreePath       string
	BranchName         string
	BaseBranch         string
	RootIssueID        string
	PRURL              string
	ErrorMessage       string
	StartedAt          *time.Time
	CompletedAt        *time.Time
	CreatedAt          time.Time
	Auto               bool
	CIStatus           string     // pending, success, failure
	ApprovalStatus     string     // pending, approved, changes_requested
	Approvers          string     // JSON array of usernames
	LastPRPollAt       *time.Time
	HasUnseenPRChanges bool
	PRState            string // open, closed, merged
	MergeableState     string // CLEAN, DIRTY, BLOCKED, BEHIND, DRAFT, UNSTABLE, UNKNOWN
}

// CreateWork creates a new work unit.
func (db *DB) CreateWork(ctx context.Context, id, name, worktreePath, branchName, baseBranch, rootIssueID string, auto bool) error {
	// Use transaction to create work and initialize counter atomically
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	err = qtx.CreateWork(ctx, sqlc.CreateWorkParams{
		ID:           id,
		Name:         name,
		WorktreePath: worktreePath,
		BranchName:   branchName,
		BaseBranch:   baseBranch,
		RootIssueID:  rootIssueID,
		Auto:         auto,
	})
	if err != nil {
		return fmt.Errorf("failed to create work %s: %w", id, err)
	}

	// Initialize task counter for this work
	err = qtx.InitializeTaskCounter(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to initialize task counter for work %s: %w", id, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// CreateWorkAndSchedulePush creates a work record and schedules a git push task atomically.
// This implements the transactional outbox pattern to ensure both operations succeed or fail together.
// Returns the idempotency key for the scheduled push task.
func (db *DB) CreateWorkAndSchedulePush(ctx context.Context, id, name, worktreePath, branchName, baseBranch, rootIssueID string, auto bool) (string, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Create work record
	err = qtx.CreateWork(ctx, sqlc.CreateWorkParams{
		ID:           id,
		Name:         name,
		WorktreePath: worktreePath,
		BranchName:   branchName,
		BaseBranch:   baseBranch,
		RootIssueID:  rootIssueID,
		Auto:         auto,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create work %s: %w", id, err)
	}

	// Initialize task counter for this work
	err = qtx.InitializeTaskCounter(ctx, id)
	if err != nil {
		return "", fmt.Errorf("failed to initialize task counter for work %s: %w", id, err)
	}

	// Schedule git push task with idempotency key
	idempotencyKey := fmt.Sprintf("git-push-%s-%s", id, branchName)

	// Check if task already exists (shouldn't happen for new work, but safe to check)
	existing, err := qtx.GetTaskByIdempotencyKey(ctx, sql.NullString{String: idempotencyKey, Valid: true})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("failed to check existing task: %w", err)
	}
	if existing.ID == "" {
		// Create the scheduled task
		taskID := uuid.New().String()
		metadata := map[string]string{
			"branch": branchName,
			"dir":    worktreePath,
		}
		metadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return "", fmt.Errorf("failed to marshal metadata: %w", err)
		}

		err = qtx.CreateScheduledTaskWithRetry(ctx, sqlc.CreateScheduledTaskWithRetryParams{
			ID:             taskID,
			WorkID:         id,
			TaskType:       TaskTypeGitPush,
			ScheduledAt:    time.Now().Add(OptimisticExecutionDelay),
			Status:         TaskStatusPending,
			Metadata:       string(metadataJSON),
			AttemptCount:   0,
			MaxAttempts:    int64(DefaultMaxAttempts),
			IdempotencyKey: sql.NullString{String: idempotencyKey, Valid: true},
		})
		if err != nil {
			return "", fmt.Errorf("failed to schedule git push task: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return idempotencyKey, nil
}

// StartWork marks a work as processing.
func (db *DB) StartWork(ctx context.Context, id string) error {
	now := time.Now()
	rows, err := db.queries.StartWork(ctx, sqlc.StartWorkParams{
		StartedAt: nullTime(now),
		ID:        id,
	})
	if err != nil {
		return fmt.Errorf("failed to start work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// CompleteWork marks a work as completed with a PR URL.
func (db *DB) CompleteWork(ctx context.Context, id, prURL string) error {
	now := time.Now()
	rows, err := db.queries.CompleteWork(ctx, sqlc.CompleteWorkParams{
		PrUrl:       prURL,
		CompletedAt: nullTime(now),
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to complete work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// CompleteWorkAndScheduleFeedback atomically marks a work as completed and schedules
// PR feedback polling if a PR URL is provided. This ensures feedback polling is set up
// exactly when the PR is created, using a transaction to prevent race conditions.
func (db *DB) CompleteWorkAndScheduleFeedback(ctx context.Context, id, prURL string, prFeedbackInterval, commentResolutionInterval time.Duration) error {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Complete the work
	now := time.Now()
	rows, err := qtx.CompleteWork(ctx, sqlc.CompleteWorkParams{
		PrUrl:       prURL,
		CompletedAt: nullTime(now),
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to complete work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}

	// If PR URL provided, schedule feedback polling tasks (if not already scheduled)
	if prURL != "" {
		// Check if PR feedback task already exists
		existingPRFeedback, _ := qtx.GetPendingTaskByType(ctx, sqlc.GetPendingTaskByTypeParams{
			WorkID:   id,
			TaskType: TaskTypePRFeedback,
		})
		if existingPRFeedback.ID == "" {
			// Schedule PR feedback check
			prFeedbackCheckTime := now.Add(prFeedbackInterval)
			err = qtx.CreateScheduledTask(ctx, sqlc.CreateScheduledTaskParams{
				ID:          uuid.New().String(),
				WorkID:      id,
				TaskType:    TaskTypePRFeedback,
				ScheduledAt: prFeedbackCheckTime,
				Status:      TaskStatusPending,
				Metadata:    "{}",
			})
			if err != nil {
				return fmt.Errorf("failed to schedule PR feedback check: %w", err)
			}
		}

		// Check if comment resolution task already exists
		existingCommentRes, _ := qtx.GetPendingTaskByType(ctx, sqlc.GetPendingTaskByTypeParams{
			WorkID:   id,
			TaskType: TaskTypeCommentResolution,
		})
		if existingCommentRes.ID == "" {
			// Schedule comment resolution check
			commentResolutionCheckTime := now.Add(commentResolutionInterval)
			err = qtx.CreateScheduledTask(ctx, sqlc.CreateScheduledTaskParams{
				ID:          uuid.New().String(),
				WorkID:      id,
				TaskType:    TaskTypeCommentResolution,
				ScheduledAt: commentResolutionCheckTime,
				Status:      TaskStatusPending,
				Metadata:    "{}",
			})
			if err != nil {
				return fmt.Errorf("failed to schedule comment resolution check: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// FailWork marks a work as failed with an error message.
func (db *DB) FailWork(ctx context.Context, id, errMsg string) error {
	now := time.Now()
	rows, err := db.queries.FailWork(ctx, sqlc.FailWorkParams{
		ErrorMessage: errMsg,
		CompletedAt:  nullTime(now),
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to mark work %s as failed: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// IdleWork marks a work as idle (all tasks complete, waiting for more).
// Returns nil if the work is already in a terminal status (merged/completed).
func (db *DB) IdleWork(ctx context.Context, id string) error {
	rows, err := db.queries.IdleWork(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to mark work %s as idle: %w", id, err)
	}
	// rows == 0 is valid - work may be in terminal status (merged/completed)
	_ = rows
	return nil
}

// IdleWorkWithPR marks a work as idle and optionally sets the PR URL.
// PR feedback polling is scheduled separately when the PR is first created
// (via SetWorkPRURLAndScheduleFeedback), not when work goes idle.
func (db *DB) IdleWorkWithPR(ctx context.Context, id, prURL string) error {
	// Idle the work (with PR URL if provided)
	var rows int64
	var err error
	if prURL != "" {
		rows, err = db.queries.IdleWorkWithPR(ctx, sqlc.IdleWorkWithPRParams{
			PrUrl: prURL,
			ID:    id,
		})
	} else {
		rows, err = db.queries.IdleWork(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("failed to idle work %s: %w", id, err)
	}
	// rows == 0 is valid - work may be in terminal status (merged/completed)
	_ = rows

	return nil
}

// SetWorkPRURLAndScheduleFeedback atomically sets the PR URL on a work and schedules
// PR feedback polling tasks. This should be called when a PR is first created,
// ensuring feedback polling starts immediately rather than waiting for work to go idle.
// The PR URL is only set if it's not already set (idempotent).
func (db *DB) SetWorkPRURLAndScheduleFeedback(ctx context.Context, id, prURL string, prFeedbackInterval, commentResolutionInterval time.Duration) error {
	if prURL == "" {
		return nil // Nothing to do
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Set the PR URL (only if not already set)
	_, err = qtx.SetWorkPRURL(ctx, sqlc.SetWorkPRURLParams{
		PrUrl: prURL,
		ID:    id,
	})
	if err != nil {
		return fmt.Errorf("failed to set PR URL for work %s: %w", id, err)
	}

	// If PR URL was already set, the tasks should already be scheduled
	// We still check and schedule if missing for robustness
	now := time.Now()

	// Check if PR feedback task already exists
	existingPRFeedback, err := qtx.GetPendingTaskByType(ctx, sqlc.GetPendingTaskByTypeParams{
		WorkID:   id,
		TaskType: TaskTypePRFeedback,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check existing PR feedback task: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || existingPRFeedback.ID == "" {
		// Schedule PR feedback check
		prFeedbackCheckTime := now.Add(prFeedbackInterval)
		err = qtx.CreateScheduledTask(ctx, sqlc.CreateScheduledTaskParams{
			ID:          uuid.New().String(),
			WorkID:      id,
			TaskType:    TaskTypePRFeedback,
			ScheduledAt: prFeedbackCheckTime,
			Status:      TaskStatusPending,
			Metadata:    "{}",
		})
		if err != nil {
			return fmt.Errorf("failed to schedule PR feedback check: %w", err)
		}
	}

	// Check if comment resolution task already exists
	existingCommentRes, err := qtx.GetPendingTaskByType(ctx, sqlc.GetPendingTaskByTypeParams{
		WorkID:   id,
		TaskType: TaskTypeCommentResolution,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check existing comment resolution task: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || existingCommentRes.ID == "" {
		// Schedule comment resolution check
		commentResolutionCheckTime := now.Add(commentResolutionInterval)
		err = qtx.CreateScheduledTask(ctx, sqlc.CreateScheduledTaskParams{
			ID:          uuid.New().String(),
			WorkID:      id,
			TaskType:    TaskTypeCommentResolution,
			ScheduledAt: commentResolutionCheckTime,
			Status:      TaskStatusPending,
			Metadata:    "{}",
		})
		if err != nil {
			return fmt.Errorf("failed to schedule comment resolution check: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RestartWork transitions a failed work back to processing.
// Only works if the work is currently in failed status.
func (db *DB) RestartWork(ctx context.Context, id string) error {
	rows, err := db.queries.RestartWork(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to restart work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found or not in failed status", id)
	}
	return nil
}

// ResumeWork transitions an idle or completed work back to processing.
// Use this when new tasks are added to a work that was idle/completed.
func (db *DB) ResumeWork(ctx context.Context, id string) error {
	rows, err := db.queries.ResumeWork(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to resume work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found or not in idle/completed status", id)
	}
	return nil
}

// UpdateWorkWorktreePath updates the worktree path for a work.
// Used by the control plane after creating a worktree asynchronously.
func (db *DB) UpdateWorkWorktreePath(ctx context.Context, id, worktreePath string) error {
	rows, err := db.queries.UpdateWorkWorktreePath(ctx, sqlc.UpdateWorkWorktreePathParams{
		WorktreePath: worktreePath,
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to update work worktree path: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// GetWork retrieves a work by ID.
func (db *DB) GetWork(ctx context.Context, id string) (*Work, error) {
	work, err := db.queries.GetWork(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	return workToLocal(&work), nil
}

// ListWorks returns all works, optionally filtered by status.
func (db *DB) ListWorks(ctx context.Context, statusFilter string) ([]*Work, error) {
	var works []sqlc.Work
	var err error

	if statusFilter == "" {
		works, err = db.queries.ListWorks(ctx)
	} else {
		works, err = db.queries.ListWorksByStatus(ctx, statusFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list works: %w", err)
	}

	result := make([]*Work, len(works))
	for i, w := range works {
		result[i] = workToLocal(&w)
	}
	return result, nil
}

// GetLastWorkID returns the ID of the most recently created work.
func (db *DB) GetLastWorkID(ctx context.Context) (string, error) {
	id, err := db.queries.GetLastWorkID(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get last work ID: %w", err)
	}
	return id, nil
}

// GetWorkByDirectory returns the work that has a worktree path matching the pattern.
func (db *DB) GetWorkByDirectory(ctx context.Context, pathPattern string) (*Work, error) {
	work, err := db.queries.GetWorkByDirectory(ctx, pathPattern)
  if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get work by directory: %w", err)
	}
	return workToLocal(&work), nil
}

// AddTaskToWork associates a task with a work.
func (db *DB) AddTaskToWork(ctx context.Context, workID, taskID string, position int) error {
	err := db.queries.AddTaskToWork(ctx, sqlc.AddTaskToWorkParams{
		WorkID:   workID,
		TaskID:   taskID,
		Position: int64(position),
	})
	if err != nil {
		return fmt.Errorf("failed to add task %s to work %s: %w", taskID, workID, err)
	}
	return nil
}

// GetWorkTasks returns all tasks for a work in order.
func (db *DB) GetWorkTasks(ctx context.Context, workID string) ([]*Task, error) {
	tasks, err := db.queries.GetWorkTasks(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work tasks: %w", err)
	}

	result := make([]*Task, len(tasks))
	for i, t := range tasks {
		result[i] = listTaskRowToLocal(
			t.ID, t.Status, t.TaskType, t.ComplexityBudget, t.ActualComplexity,
			t.WorkID, t.WorktreePath, t.PrUrl, t.ErrorMessage,
			t.StartedAt, t.CompletedAt, t.CreatedAt,
			t.SpawnedAt, t.SpawnStatus,
		)
	}
	return result, nil
}

// IsWorkCompleted checks if all tasks in a work are completed.
func (db *DB) IsWorkCompleted(workID string) (bool, error) {
	var total, completed int
	err := db.QueryRow(`
		SELECT COUNT(*), COUNT(CASE WHEN t.status = ? THEN 1 END)
		FROM work_tasks wt
		JOIN tasks t ON wt.task_id = t.id
		WHERE wt.work_id = ?
	`, StatusCompleted, workID).Scan(&total, &completed)
	if err != nil {
		return false, fmt.Errorf("failed to check work completion: %w", err)
	}
	if total == 0 {
		return false, nil
	}
	return completed == total, nil
}

// toBase36 converts a byte array to a base36 string.
func toBase36(bytes []byte) string {
	// Convert bytes to a big integer
	num := new(big.Int).SetBytes(bytes)
	// Convert to base36
	return strings.ToLower(num.Text(36))
}

// GenerateWorkID generates a content-based hash ID for a work.
// Uses the branch name as the primary content for hashing.
func (db *DB) GenerateWorkID(ctx context.Context, branchName string, projectName string) (string, error) {
	// Start with a base length of 3 characters for work IDs
	// (we expect fewer works than issues)
	baseLength := 3
	maxAttempts := 30

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Calculate target length based on attempts
		targetLength := baseLength + (attempt / 10)
		if targetLength > 8 {
			targetLength = 8 // Cap at 8 characters
		}

		// Create content for hashing
		nonce := attempt % 10
		content := fmt.Sprintf("%s:%s:%d:%d",
			branchName,
			projectName,
			time.Now().UnixNano(),
			nonce)

		// Generate SHA256 hash
		hash := sha256.Sum256([]byte(content))

		// Convert to base36 and truncate to target length
		hashStr := toBase36(hash[:])
		if len(hashStr) > targetLength {
			hashStr = hashStr[:targetLength]
		}

		// Create work ID with prefix
		workID := fmt.Sprintf("w-%s", hashStr)

		// Check if this ID already exists
		existing, err := db.GetWork(ctx, workID)
		if err != nil {
			return "", fmt.Errorf("failed to check for existing work: %w", err)
		}
		if existing == nil {
			// ID is unique, return it
			return workID, nil
		}
	}

	// If we exhausted all attempts, generate a fallback ID
	return fmt.Sprintf("w-%d", time.Now().UnixNano()/1000000), nil
}

// GenerateNextWorkID generates a work ID using content-based hashing.
// This is a compatibility wrapper that generates a temporary ID.
func (db *DB) GenerateNextWorkID(ctx context.Context) (string, error) {
	// Generate a temporary work ID based on timestamp
	// This should only be used when we don't have branch information yet
	return db.GenerateWorkID(ctx, fmt.Sprintf("temp-%d", time.Now().UnixNano()), "unknown")
}

// GetNextTaskNumber returns the next available task number for a work.
// Tasks are numbered sequentially within each work (w-abc.1, w-abc.2, etc.)
// This uses an atomic counter to avoid race conditions.
func (db *DB) GetNextTaskNumber(ctx context.Context, workID string) (int, error) {
	// First ensure the counter exists (for backwards compatibility with existing works)
	err := db.queries.InitializeTaskCounter(ctx, workID)
	if err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		return 0, fmt.Errorf("failed to initialize task counter: %w", err)
	}

	// Get and increment the counter atomically
	taskNum, err := db.queries.GetAndIncrementTaskCounter(ctx, workID)
	if err != nil {
		return 0, fmt.Errorf("failed to get next task number: %w", err)
	}
	return int(taskNum), nil
}

// DeleteWork deletes a work and all associated records.
// This includes:
// - Task beans associations for all tasks in the work
// - Tasks belonging to the work
// - Work-task relationships
// - Work-bean associations
// - Scheduled tasks for this work
// - The work record itself
func (db *DB) DeleteWork(ctx context.Context, workID string) error {
	// Start a transaction to ensure all deletes happen atomically
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Delete task_beans entries for all tasks in this work
	if _, err := qtx.DeleteTaskBeansForWork(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete task beans for work %s: %w", workID, err)
	}

	// Delete work_tasks relationships
	if _, err := qtx.DeleteWorkTasks(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete work tasks for work %s: %w", workID, err)
	}

	// Delete all tasks belonging to this work
	if _, err := qtx.DeleteTasksForWork(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete tasks for work %s: %w", workID, err)
	}

	// Delete work_beans associations
	if _, err := qtx.DeleteWorkBeans(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete work beans for work %s: %w", workID, err)
	}

	// Delete scheduled tasks for this work
	if _, err := qtx.DeleteSchedulerForWork(ctx, workID); err != nil {
		return fmt.Errorf("failed to delete scheduled tasks for work %s: %w", workID, err)
	}

	// Finally, delete the work itself
	rows, err := qtx.DeleteWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to delete work %s: %w", workID, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", workID)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateWorkPRStatus updates the PR status fields for a work.
// ciStatus: pending, success, failure
// approvalStatus: pending, approved, changes_requested
// approvers: JSON array of approver usernames
// mergeableState: CLEAN, DIRTY, BLOCKED, BEHIND, DRAFT, UNSTABLE, UNKNOWN
func (db *DB) UpdateWorkPRStatus(ctx context.Context, id, ciStatus, approvalStatus, approvers, prState, mergeableState string) error {
	now := time.Now()
	rows, err := db.queries.UpdateWorkPRStatus(ctx, sqlc.UpdateWorkPRStatusParams{
		CiStatus:       ciStatus,
		ApprovalStatus: approvalStatus,
		Approvers:      approvers,
		PrState:        prState,
		MergeableState: mergeableState,
		LastPrPollAt:   nullTime(now),
		ID:             id,
	})
	if err != nil {
		return fmt.Errorf("failed to update PR status for work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// MergeWork marks a work as merged (PR was merged on GitHub).
func (db *DB) MergeWork(ctx context.Context, id string) error {
	now := time.Now()
	rows, err := db.queries.MergeWork(ctx, sqlc.MergeWorkParams{
		CompletedAt: nullTime(now),
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to merge work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// SetWorkHasUnseenPRChanges sets the has_unseen_pr_changes flag for a work.
func (db *DB) SetWorkHasUnseenPRChanges(ctx context.Context, id string, hasChanges bool) error {
	rows, err := db.queries.SetWorkHasUnseenPRChanges(ctx, sqlc.SetWorkHasUnseenPRChangesParams{
		HasUnseenPrChanges: hasChanges,
		ID:                 id,
	})
	if err != nil {
		return fmt.Errorf("failed to set unseen PR changes for work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// MarkWorkPRSeen marks the PR changes as seen for a work.
func (db *DB) MarkWorkPRSeen(ctx context.Context, id string) error {
	rows, err := db.queries.MarkWorkPRSeen(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to mark PR changes as seen for work %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("work %s not found", id)
	}
	return nil
}

// GetWorksWithUnseenChanges returns all works with unseen PR changes.
func (db *DB) GetWorksWithUnseenChanges(ctx context.Context) ([]*Work, error) {
	works, err := db.queries.GetWorksWithUnseenChanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get works with unseen changes: %w", err)
	}

	result := make([]*Work, len(works))
	for i, w := range works {
		result[i] = workToLocal(&w)
	}
	return result, nil
}

// GetWorksWithPRs returns all works that have a PR URL.
func (db *DB) GetWorksWithPRs(ctx context.Context) ([]*Work, error) {
	works, err := db.queries.GetWorksWithPRs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get works with PRs: %w", err)
	}

	result := make([]*Work, len(works))
	for i, w := range works {
		result[i] = workToLocal(&w)
	}
	return result, nil
}

// AddBeanToWork associates a bean with a work.
// The bean is added at the next available position.
func (db *DB) AddBeanToWork(ctx context.Context, workID, beanID string) error {
	// Get the current max position
	maxPos, err := db.queries.GetMaxWorkBeanPosition(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get max position for work %s: %w", workID, err)
	}

	err = db.queries.AddWorkBean(ctx, sqlc.AddWorkBeanParams{
		WorkID:   workID,
		BeanID:   beanID,
		Position: maxPos + 1,
	})
	if err != nil {
		return fmt.Errorf("failed to add bean %s to work %s: %w", beanID, workID, err)
	}
	return nil
}
