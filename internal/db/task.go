package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// taskRowToLocal converts a GetTaskRow to local Task
func taskRowToLocal(t *sqlc.GetTaskRow) *Task {
	task := &Task{
		ID:               t.ID,
		Status:           t.Status,
		TaskType:         t.TaskType,
		ComplexityBudget: int(t.ComplexityBudget),
		ActualComplexity: int(t.ActualComplexity),
		WorkID:           t.WorkID,
		WorktreePath:     t.WorktreePath,
		PRURL:            t.PrUrl,
		ErrorMessage:     t.ErrorMessage,
		CreatedAt:        t.CreatedAt,
		SpawnStatus:      t.SpawnStatus,
	}
	if t.StartedAt.Valid {
		task.StartedAt = &t.StartedAt.Time
	}
	if t.CompletedAt.Valid {
		task.CompletedAt = &t.CompletedAt.Time
	}
	if t.SpawnedAt.Valid {
		task.SpawnedAt = &t.SpawnedAt.Time
	}
	return task
}

// listTaskRowToLocal converts a ListTasksRow/ListTasksByStatusRow to local Task
func listTaskRowToLocal(id string, status string, taskType string, complexityBudget int64, actualComplexity int64,
	workID string, worktreePath string, prURL string, errorMessage string,
	startedAt sql.NullTime, completedAt sql.NullTime, createdAt time.Time,
	spawnedAt sql.NullTime, spawnStatus string) *Task {

	task := &Task{
		ID:               id,
		Status:           status,
		TaskType:         taskType,
		ComplexityBudget: int(complexityBudget),
		ActualComplexity: int(actualComplexity),
		WorkID:           workID,
		WorktreePath:     worktreePath,
		PRURL:            prURL,
		ErrorMessage:     errorMessage,
		CreatedAt:        createdAt,
		SpawnStatus:      spawnStatus,
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if spawnedAt.Valid {
		task.SpawnedAt = &spawnedAt.Time
	}
	return task
}

// Task represents a virtual task (group of beads) in the database.
type Task struct {
	ID               string
	Status           string
	TaskType         string
	ComplexityBudget int
	ActualComplexity int
	WorkID           string
	WorktreePath     string
	PRURL            string
	ErrorMessage     string
	StartedAt        *time.Time
	CompletedAt      *time.Time
	CreatedAt        time.Time
	SpawnedAt        *time.Time
	SpawnStatus      string
}

// TaskBead represents a bead within a task.
type TaskBead struct {
	TaskID string
	BeadID string
	Status string
}

// CreateTask creates a new task with the given beads.
func (db *DB) CreateTask(ctx context.Context, id string, taskType string, beadIDs []string, complexityBudget int, workID string, taskNumber int) error {
	// Use a transaction for atomicity
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Insert task
	err = qtx.CreateTask(ctx, sqlc.CreateTaskParams{
		ID:               id,
		TaskType:         taskType,
		ComplexityBudget: int64(complexityBudget),
		WorkID:           workID,
		TaskNumber:       int64(taskNumber),
	})
	if err != nil {
		return fmt.Errorf("failed to create task %s: %w", id, err)
	}

	// Insert task_beads
	for _, beadID := range beadIDs {
		err = qtx.CreateTaskBead(ctx, sqlc.CreateTaskBeadParams{
			TaskID: id,
			BeadID: beadID,
		})
		if err != nil {
			return fmt.Errorf("failed to add bead %s to task %s: %w", beadID, id, err)
		}
	}

	// Create work_tasks junction entry if workID is provided
	if workID != "" {
		// Get the current number of tasks to determine position
		existingTasks, err := qtx.GetWorkTasks(ctx, workID)
		if err != nil {
			return fmt.Errorf("failed to get existing tasks for work: %w", err)
		}

		err = qtx.AddTaskToWork(ctx, sqlc.AddTaskToWorkParams{
			WorkID:   workID,
			TaskID:   id,
			Position: int64(len(existingTasks)),
		})
		if err != nil {
			return fmt.Errorf("failed to link task %s to work %s: %w", id, workID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// StartTask marks a task as processing and sets its worktree path.
func (db *DB) StartTask(ctx context.Context, id string, worktreePath string) error {
	rows, err := db.queries.StartTask(ctx, sqlc.StartTaskParams{
		WorktreePath: worktreePath,
		StartedAt:    sql.NullTime{Time: time.Now(), Valid: true},
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task %s not found", id)
	}
	return nil
}

// CompleteTask marks a task as completed.
func (db *DB) CompleteTask(ctx context.Context, id string, prURL string) error {
	rows, err := db.queries.CompleteTask(ctx, sqlc.CompleteTaskParams{
		PrUrl:       prURL,
		CompletedAt: sql.NullTime{Time: time.Now(), Valid: true},
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task %s not found", id)
	}
	return nil
}

// FailTask marks a task as failed with an error message.
func (db *DB) FailTask(ctx context.Context, id string, errorMessage string) error {
	rows, err := db.queries.FailTask(ctx, sqlc.FailTaskParams{
		ErrorMessage: errorMessage,
		CompletedAt:  sql.NullTime{Time: time.Now(), Valid: true},
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to fail task: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task %s not found", id)
	}
	return nil
}

// ResetTaskStatus resets a task status to pending.
func (db *DB) ResetTaskStatus(ctx context.Context, taskID string) error {
	rows, err := db.queries.ResetTaskStatus(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to reset task status: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task %s not found", taskID)
	}
	return nil
}

// GetTask retrieves a task by ID.
func (db *DB) GetTask(ctx context.Context, id string) (*Task, error) {
	task, err := db.queries.GetTask(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return taskRowToLocal(&task), nil
}

// GetTaskBeads returns the list of bead IDs for a task.
func (db *DB) GetTaskBeads(ctx context.Context, taskID string) ([]string, error) {
	beadIDs, err := db.queries.GetTaskBeads(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads: %w", err)
	}
	return beadIDs, nil
}

// GetTaskForBead returns the task ID that contains a specific bead.
func (db *DB) GetTaskForBead(ctx context.Context, beadID string) (string, error) {
	taskID, err := db.queries.GetTaskForBead(ctx, beadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get task for bead: %w", err)
	}
	return taskID, nil
}

// CompleteTaskBead marks a specific bead within a task as completed.
func (db *DB) CompleteTaskBead(ctx context.Context, taskID, beadID string) error {
	rows, err := db.queries.CompleteTaskBead(ctx, sqlc.CompleteTaskBeadParams{
		TaskID: taskID,
		BeadID: beadID,
	})
	if err != nil {
		return fmt.Errorf("failed to complete task bead: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bead %s/%s not found", taskID, beadID)
	}
	return nil
}

// FailTaskBead marks a specific bead within a task as failed.
func (db *DB) FailTaskBead(ctx context.Context, taskID, beadID string) error {
	rows, err := db.queries.FailTaskBead(ctx, sqlc.FailTaskBeadParams{
		TaskID: taskID,
		BeadID: beadID,
	})
	if err != nil {
		return fmt.Errorf("failed to fail task bead: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bead %s/%s not found", taskID, beadID)
	}
	return nil
}

// CountTaskBeadStatuses returns the total and completed count of beads in a task.
func (db *DB) CountTaskBeadStatuses(ctx context.Context, taskID string) (total int, completed int, err error) {
	row, err := db.queries.CountTaskBeadStatuses(ctx, taskID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count task bead statuses: %w", err)
	}
	return int(row.Total), int(row.Completed), nil
}

// GetTaskBeadStatus returns the status of a specific bead within a task.
func (db *DB) GetTaskBeadStatus(ctx context.Context, taskID, beadID string) (string, error) {
	status, err := db.queries.GetTaskBeadStatus(ctx, sqlc.GetTaskBeadStatusParams{
		TaskID: taskID,
		BeadID: beadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("task bead %s/%s not found", taskID, beadID)
		}
		return "", fmt.Errorf("failed to get task bead status: %w", err)
	}
	return status, nil
}

// TaskBeadInfo represents a task bead with its status.
type TaskBeadInfo struct {
	TaskID string
	BeadID string
	Status string
}

// GetTaskBeadsForWork returns all task beads for a work in a single query.
func (db *DB) GetTaskBeadsForWork(ctx context.Context, workID string) ([]TaskBeadInfo, error) {
	rows, err := db.queries.GetTaskBeadsForWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads for work: %w", err)
	}
	result := make([]TaskBeadInfo, len(rows))
	for i, row := range rows {
		result[i] = TaskBeadInfo{
			TaskID: row.TaskID,
			BeadID: row.BeadID,
			Status: row.Status,
		}
	}
	return result, nil
}

// ListTasks returns all tasks.
func (db *DB) ListTasks(ctx context.Context, statusFilter string) ([]*Task, error) {
	var tasks []*Task

	if statusFilter == "" {
		rows, err := db.queries.ListTasks(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list tasks: %w", err)
		}
		for _, row := range rows {
			tasks = append(tasks, listTaskRowToLocal(
				row.ID, row.Status, row.TaskType, row.ComplexityBudget, row.ActualComplexity,
				row.WorkID, row.WorktreePath, row.PrUrl, row.ErrorMessage,
				row.StartedAt, row.CompletedAt, row.CreatedAt,
				row.SpawnedAt, row.SpawnStatus,
			))
		}
	} else {
		rows, err := db.queries.ListTasksByStatus(ctx, statusFilter)
		if err != nil {
			return nil, fmt.Errorf("failed to list tasks by status: %w", err)
		}
		for _, row := range rows {
			tasks = append(tasks, listTaskRowToLocal(
				row.ID, row.Status, row.TaskType, row.ComplexityBudget, row.ActualComplexity,
				row.WorkID, row.WorktreePath, row.PrUrl, row.ErrorMessage,
				row.StartedAt, row.CompletedAt, row.CreatedAt,
				row.SpawnedAt, row.SpawnStatus,
			))
		}
	}

	return tasks, nil
}

// SpawnTask updates spawn metadata for a task.
func (db *DB) SpawnTask(ctx context.Context, taskID string, status string) error {
	_, err := db.queries.SpawnTask(ctx, sqlc.SpawnTaskParams{
		SpawnedAt:   sql.NullTime{Time: time.Now(), Valid: true},
		SpawnStatus: status,
		ID:          taskID,
	})
	if err != nil {
		return fmt.Errorf("failed to update spawn status: %w", err)
	}
	return nil
}

// DeleteTask deletes a task and its associated records.
func (db *DB) DeleteTask(ctx context.Context, taskID string) error {
	// Use a transaction for atomicity
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Delete work_tasks junction (foreign key constraint)
	_, err = qtx.DeleteWorkTaskByTask(ctx, taskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// It's OK if the junction doesn't exist
		return fmt.Errorf("failed to delete work_tasks for task %s: %w", taskID, err)
	}

	// Delete task_beads (foreign key constraint)
	_, err = qtx.DeleteTaskBeadsByTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task_beads for task %s: %w", taskID, err)
	}

	// Delete the task itself
	rows, err := qtx.DeleteTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task %s: %w", taskID, err)
	}
	if rows == 0 {
		return fmt.Errorf("task %s not found", taskID)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// ResetTaskBeadStatuses resets all bead statuses for a task to pending.
func (db *DB) ResetTaskBeadStatuses(ctx context.Context, taskID string) error {
	_, err := db.queries.ResetTaskBeadStatuses(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to reset task bead statuses: %w", err)
	}
	return nil
}

// GetTaskBeadsWithStatus returns all beads in a task with their status.
func (db *DB) GetTaskBeadsWithStatus(ctx context.Context, taskID string) ([]TaskBeadInfo, error) {
	rows, err := db.queries.GetTaskBeadsWithStatus(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads with status: %w", err)
	}

	result := make([]TaskBeadInfo, len(rows))
	for i, row := range rows {
		result[i] = TaskBeadInfo{
			TaskID: row.TaskID,
			BeadID: row.BeadID,
			Status: row.Status,
		}
	}
	return result, nil
}

// ResetTaskBeadStatus resets a single bead's status in a task to pending.
func (db *DB) ResetTaskBeadStatus(ctx context.Context, taskID, beadID string) error {
	rows, err := db.queries.ResetTaskBeadStatus(ctx, sqlc.ResetTaskBeadStatusParams{
		TaskID: taskID,
		BeadID: beadID,
	})
	if err != nil {
		return fmt.Errorf("failed to reset task bead status: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bead %s/%s not found", taskID, beadID)
	}
	return nil
}

// UpdateTaskActivity updates the last_activity timestamp for a processing task.
func (db *DB) UpdateTaskActivity(ctx context.Context, taskID string, timestamp time.Time) error {
	_, err := db.queries.UpdateTaskActivity(ctx, sqlc.UpdateTaskActivityParams{
		LastActivity: sql.NullTime{Time: timestamp, Valid: true},
		ID:           taskID,
	})
	if err != nil {
		return fmt.Errorf("failed to update task activity: %w", err)
	}
	return nil
}

// CheckAndCompleteTask checks if all beads in a task are completed and marks the task as complete if so.
// Returns true if the task was auto-completed, false if it still has pending beads.
func (db *DB) CheckAndCompleteTask(ctx context.Context, taskID string, prURL string) (bool, error) {
	// Count the bead statuses
	total, completed, err := db.CountTaskBeadStatuses(ctx, taskID)
	if err != nil {
		return false, err
	}

	// If all beads are complete, mark the task as complete
	if total > 0 && total == completed {
		if err := db.CompleteTask(ctx, taskID, prURL); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

// GetPRTaskForWork returns the most recent PR task for a work, if one exists.
// Only returns tasks with status pending, processing, or completed.
// Returns nil if no PR task exists.
func (db *DB) GetPRTaskForWork(ctx context.Context, workID string) (*Task, error) {
	task, err := db.queries.GetPRTaskForWork(ctx, workID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get PR task for work: %w", err)
	}

	result := &Task{
		ID:               task.ID,
		Status:           task.Status,
		TaskType:         task.TaskType,
		ComplexityBudget: int(task.ComplexityBudget),
		ActualComplexity: int(task.ActualComplexity),
		WorkID:           task.WorkID,
		WorktreePath:     task.WorktreePath,
		PRURL:            task.PrUrl,
		ErrorMessage:     task.ErrorMessage,
		CreatedAt:        task.CreatedAt,
		SpawnStatus:      task.SpawnStatus,
	}
	if task.StartedAt.Valid {
		result.StartedAt = &task.StartedAt.Time
	}
	if task.CompletedAt.Valid {
		result.CompletedAt = &task.CompletedAt.Time
	}
	if task.SpawnedAt.Valid {
		result.SpawnedAt = &task.SpawnedAt.Time
	}
	return result, nil
}
