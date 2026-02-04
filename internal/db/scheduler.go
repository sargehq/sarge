package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sargehq/sarge/internal/db/sqlc"
)

// ScheduledTask represents a scheduled task in the database.
type ScheduledTask struct {
	ID             string
	WorkID         string
	TaskType       string
	ScheduledAt    time.Time
	ExecutedAt     *time.Time
	Status         string
	ErrorMessage   *string
	Metadata       map[string]string
	AttemptCount   int
	MaxAttempts    int
	IdempotencyKey *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Task types for the scheduler
const (
	TaskTypePRFeedback          = "pr_feedback"
	TaskTypeCommentResolution   = "comment_resolution"
	TaskTypeGitPush             = "git_push"
	TaskTypeGitHubComment       = "github_comment"
	TaskTypeGitHubResolveThread = "github_resolve_thread"
	TaskTypeCreateWorktree      = "create_worktree"
	TaskTypeImportPR            = "import_pr"
	TaskTypeSpawnOrchestrator   = "spawn_orchestrator"
	TaskTypeDestroyWorktree     = "destroy_worktree"
	TaskTypeWatchWorkflowRun    = "watch_workflow_run"
)

// DefaultMaxAttempts is the default max attempts for retry tasks.
const DefaultMaxAttempts = 5

// OptimisticExecutionDelay is how long to wait before the scheduler picks up
// a task that was scheduled with optimistic execution. This prevents a race
// condition where both the optimistic execution and the scheduler try to
// execute the same task concurrently.
const OptimisticExecutionDelay = 30 * time.Second

// Task statuses
const (
	TaskStatusPending   = "pending"
	TaskStatusExecuting = "executing"
	TaskStatusCompleted = "completed"
	TaskStatusFailed    = "failed"
)

// ScheduleTask schedules a new task for a work.
func (db *DB) ScheduleTask(ctx context.Context, workID string, taskType string, scheduledAt time.Time, metadata map[string]string) (*ScheduledTask, error) {
	id := uuid.New().String()

	metadataJSON := "{}"
	if metadata != nil {
		data, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = string(data)
	}

	err := db.queries.CreateScheduledTask(ctx, sqlc.CreateScheduledTaskParams{
		ID:          id,
		WorkID:      workID,
		TaskType:    taskType,
		ScheduledAt: scheduledAt,
		Status:      TaskStatusPending,
		Metadata:    metadataJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to schedule task: %w", err)
	}

	return &ScheduledTask{
		ID:          id,
		WorkID:      workID,
		TaskType:    taskType,
		ScheduledAt: scheduledAt,
		Status:      TaskStatusPending,
		Metadata:    metadata,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}, nil
}

// GetNextScheduledTask gets the next pending task that's ready to run.
func (db *DB) GetNextScheduledTask(ctx context.Context) (*ScheduledTask, error) {
	task, err := db.queries.GetNextScheduledTask(ctx)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get next scheduled task: %w", err)
	}

	return schedulerToLocal(&task), nil
}

// GetScheduledTasksForWork gets all pending scheduled tasks for a work.
func (db *DB) GetScheduledTasksForWork(ctx context.Context, workID string) ([]*ScheduledTask, error) {
	tasks, err := db.queries.GetScheduledTasksForWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduled tasks: %w", err)
	}

	result := make([]*ScheduledTask, len(tasks))
	for i, t := range tasks {
		result[i] = schedulerToLocal(&t)
	}
	return result, nil
}

// GetPendingTaskByType gets the next pending task of a specific type for a work.
func (db *DB) GetPendingTaskByType(ctx context.Context, workID string, taskType string) (*ScheduledTask, error) {
	task, err := db.queries.GetPendingTaskByType(ctx, sqlc.GetPendingTaskByTypeParams{
		WorkID:   workID,
		TaskType: taskType,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get pending task by type: %w", err)
	}

	return schedulerToLocal(&task), nil
}

// UpdateScheduledTaskTime updates the scheduled time for a task.
func (db *DB) UpdateScheduledTaskTime(ctx context.Context, taskID string, scheduledAt time.Time) error {
	err := db.queries.UpdateScheduledTaskTime(ctx, sqlc.UpdateScheduledTaskTimeParams{
		ID:          taskID,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return fmt.Errorf("failed to update scheduled task time: %w", err)
	}
	return nil
}

// MarkTaskExecuting marks a task as currently executing.
func (db *DB) MarkTaskExecuting(ctx context.Context, taskID string) error {
	err := db.queries.MarkTaskExecuting(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to mark task as executing: %w", err)
	}
	return nil
}

// MarkTaskCompleted marks a task as completed.
func (db *DB) MarkTaskCompleted(ctx context.Context, taskID string) error {
	err := db.queries.MarkTaskCompleted(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to mark task as completed: %w", err)
	}
	return nil
}

// MarkTaskFailed marks a task as failed with an error message.
func (db *DB) MarkTaskFailed(ctx context.Context, taskID string, errorMessage string) error {
	err := db.queries.MarkTaskFailed(ctx, sqlc.MarkTaskFailedParams{
		ID:           taskID,
		ErrorMessage: sql.NullString{String: errorMessage, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to mark task as failed: %w", err)
	}
	return nil
}

// ScheduleOrUpdateTask schedules a new task or updates existing one.
// If a pending task of the same type exists, it updates the scheduled time.
// Otherwise, it creates a new scheduled task.
func (db *DB) ScheduleOrUpdateTask(ctx context.Context, workID string, taskType string, scheduledAt time.Time) (*ScheduledTask, error) {
	// Check if there's already a pending task of this type
	existing, err := db.GetPendingTaskByType(ctx, workID, taskType)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update the existing task's scheduled time
		err = db.UpdateScheduledTaskTime(ctx, existing.ID, scheduledAt)
		if err != nil {
			return nil, err
		}
		existing.ScheduledAt = scheduledAt
		return existing, nil
	}

	// Create a new scheduled task
	return db.ScheduleTask(ctx, workID, taskType, scheduledAt, nil)
}

// TriggerTaskNow schedules a task to run immediately.
func (db *DB) TriggerTaskNow(ctx context.Context, workID string, taskType string) (*ScheduledTask, error) {
	return db.ScheduleOrUpdateTask(ctx, workID, taskType, time.Now())
}

// WatchSchedulerChanges returns tasks that have been updated since the given time.
func (db *DB) WatchSchedulerChanges(ctx context.Context, since time.Time) ([]*ScheduledTask, error) {
	tasks, err := db.queries.WatchSchedulerChanges(ctx, since)
	if err != nil {
		return nil, fmt.Errorf("failed to watch scheduler changes: %w", err)
	}

	result := make([]*ScheduledTask, len(tasks))
	for i, t := range tasks {
		result[i] = schedulerToLocal(&t)
	}
	return result, nil
}

// CleanupOldTasks removes completed/failed tasks older than the specified duration.
func (db *DB) CleanupOldTasks(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	err := db.queries.DeleteCompletedTasksOlderThan(ctx, sql.NullTime{Time: cutoff, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to cleanup old tasks: %w", err)
	}
	return nil
}

// Helper function to convert SQLC Scheduler to local ScheduledTask
func schedulerToLocal(s *sqlc.Scheduler) *ScheduledTask {
	task := &ScheduledTask{
		ID:           s.ID,
		WorkID:       s.WorkID,
		TaskType:     s.TaskType,
		ScheduledAt:  s.ScheduledAt,
		Status:       s.Status,
		AttemptCount: int(s.AttemptCount),
		MaxAttempts:  int(s.MaxAttempts),
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}

	if s.ExecutedAt.Valid {
		task.ExecutedAt = &s.ExecutedAt.Time
	}

	if s.ErrorMessage.Valid {
		task.ErrorMessage = &s.ErrorMessage.String
	}

	if s.IdempotencyKey.Valid {
		task.IdempotencyKey = &s.IdempotencyKey.String
	}

	// Parse metadata JSON
	task.Metadata = make(map[string]string)
	if s.Metadata != "" && s.Metadata != "{}" {
		_ = json.Unmarshal([]byte(s.Metadata), &task.Metadata)
	}

	return task
}

// ScheduleTaskWithRetry schedules a new task with retry support and an idempotency key.
// If a task with the same idempotency key already exists, it returns that task instead.
func (db *DB) ScheduleTaskWithRetry(ctx context.Context, workID, taskType string, scheduledAt time.Time, metadata map[string]string, idempotencyKey string, maxAttempts int) error {
	if idempotencyKey == "" {
		return fmt.Errorf("idempotency key is required for scheduling task with retry")
	}

	existing, err := db.GetTaskByIdempotencyKey(ctx, idempotencyKey)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	id := uuid.New().String()

	metadataJSON := "{}"
	if metadata != nil {
		data, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		metadataJSON = string(data)
	}

	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}

	err = db.queries.CreateScheduledTaskWithRetry(ctx, sqlc.CreateScheduledTaskWithRetryParams{
		ID:             id,
		WorkID:         workID,
		TaskType:       taskType,
		ScheduledAt:    scheduledAt,
		Status:         TaskStatusPending,
		Metadata:       metadataJSON,
		AttemptCount:   0,
		MaxAttempts:    int64(maxAttempts),
		IdempotencyKey: sql.NullString{String: idempotencyKey, Valid: idempotencyKey != ""},
	})
	if err != nil {
		return fmt.Errorf("failed to schedule task with retry: %w", err)
	}

	return nil
}

// GetTaskByIdempotencyKey retrieves a task by its idempotency key.
func (db *DB) GetTaskByIdempotencyKey(ctx context.Context, idempotencyKey string) (*ScheduledTask, error) {
	task, err := db.queries.GetTaskByIdempotencyKey(ctx, sql.NullString{String: idempotencyKey, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get task by idempotency key: %w", err)
	}
	return schedulerToLocal(&task), nil
}

// ShouldRetry returns true if the task should be retried based on attempt count.
func (task *ScheduledTask) ShouldRetry() bool {
	return task.AttemptCount < task.MaxAttempts
}

// RescheduleWithBackoff reschedules a failed task with exponential backoff.
// The backoff formula is: baseDelay * 2^attemptCount (capped at maxDelay).
func (db *DB) RescheduleWithBackoff(ctx context.Context, taskID string, errorMessage string) error {
	// Get the current task to check attempt count
	task, err := db.queries.GetScheduledTaskByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Calculate exponential backoff: 30s, 1m, 2m, 4m, 8m (capped at 10m)
	baseDelay := 30 * time.Second
	maxDelay := 10 * time.Minute

	backoffMultiplier := 1 << uint(task.AttemptCount) // 2^attemptCount
	delay := time.Duration(backoffMultiplier) * baseDelay
	if delay > maxDelay {
		delay = maxDelay
	}

	nextScheduledAt := time.Now().Add(delay)

	err = db.queries.IncrementAttemptAndReschedule(ctx, sqlc.IncrementAttemptAndRescheduleParams{
		ScheduledAt:  nextScheduledAt,
		ErrorMessage: sql.NullString{String: errorMessage, Valid: errorMessage != ""},
		ID:           taskID,
	})
	if err != nil {
		return fmt.Errorf("failed to reschedule task with backoff: %w", err)
	}

	return nil
}

// MarkTaskCompletedByIdempotencyKey marks a task as completed using its idempotency key.
func (db *DB) MarkTaskCompletedByIdempotencyKey(ctx context.Context, idempotencyKey string) error {
	err := db.queries.MarkTaskCompletedByIdempotencyKey(ctx, sql.NullString{String: idempotencyKey, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to mark task as completed by idempotency key: %w", err)
	}
	return nil
}

// ResetExecutingTasksToPending resets any scheduled tasks stuck in 'executing' status back to 'pending'.
// This is used when the control plane starts up to recover from a crash that left tasks in an incomplete state.
func (db *DB) ResetExecutingTasksToPending(ctx context.Context) (int64, error) {
	count, err := db.queries.ResetExecutingTasksToPending(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to reset executing tasks: %w", err)
	}
	return count, nil
}
