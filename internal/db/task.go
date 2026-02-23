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

// Task represents a virtual task (group of beans) in the database.
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

// TaskBean represents a bean within a task.
type TaskBean struct {
	TaskID string
	BeanID string
	Status string
}

// CreateTask creates a new task with the given beans.
func (db *DB) CreateTask(ctx context.Context, id string, taskType string, beanIDs []string, complexityBudget int, workID string, taskNumber int) error {
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

	// Insert task_beans
	for _, beanID := range beanIDs {
		err = qtx.CreateTaskBean(ctx, sqlc.CreateTaskBeanParams{
			TaskID: id,
			BeanID: beanID,
		})
		if err != nil {
			return fmt.Errorf("failed to add bean %s to task %s: %w", beanID, id, err)
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

// GetTaskBeans returns the list of bean IDs for a task.
func (db *DB) GetTaskBeans(ctx context.Context, taskID string) ([]string, error) {
	beanIDs, err := db.queries.GetTaskBeans(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}
	return beanIDs, nil
}

// GetTaskForBean returns the task ID that contains a specific bean.
func (db *DB) GetTaskForBean(ctx context.Context, beanID string) (string, error) {
	taskID, err := db.queries.GetTaskForBean(ctx, beanID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get task for bean: %w", err)
	}
	return taskID, nil
}

// CompleteTaskBean marks a specific bean within a task as completed.
func (db *DB) CompleteTaskBean(ctx context.Context, taskID, beanID string) error {
	rows, err := db.queries.CompleteTaskBean(ctx, sqlc.CompleteTaskBeanParams{
		TaskID: taskID,
		BeanID: beanID,
	})
	if err != nil {
		return fmt.Errorf("failed to complete task bean: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bean %s/%s not found", taskID, beanID)
	}
	return nil
}

// FailTaskBean marks a specific bean within a task as failed.
func (db *DB) FailTaskBean(ctx context.Context, taskID, beanID string) error {
	rows, err := db.queries.FailTaskBean(ctx, sqlc.FailTaskBeanParams{
		TaskID: taskID,
		BeanID: beanID,
	})
	if err != nil {
		return fmt.Errorf("failed to fail task bean: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bean %s/%s not found", taskID, beanID)
	}
	return nil
}

// CountTaskBeanStatuses returns the total and completed count of beans in a task.
func (db *DB) CountTaskBeanStatuses(ctx context.Context, taskID string) (total int, completed int, err error) {
	row, err := db.queries.CountTaskBeanStatuses(ctx, taskID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count task bean statuses: %w", err)
	}
	return int(row.Total), int(row.Completed), nil
}

// GetTaskBeanStatus returns the status of a specific bean within a task.
func (db *DB) GetTaskBeanStatus(ctx context.Context, taskID, beanID string) (string, error) {
	status, err := db.queries.GetTaskBeanStatus(ctx, sqlc.GetTaskBeanStatusParams{
		TaskID: taskID,
		BeanID: beanID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("task bean %s/%s not found", taskID, beanID)
		}
		return "", fmt.Errorf("failed to get task bean status: %w", err)
	}
	return status, nil
}

// TaskBeanInfo represents a task bean with its status.
type TaskBeanInfo struct {
	TaskID string
	BeanID string
	Status string
}

// GetTaskBeansForWork returns all task beans for a work in a single query.
func (db *DB) GetTaskBeansForWork(ctx context.Context, workID string) ([]TaskBeanInfo, error) {
	rows, err := db.queries.GetTaskBeansForWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans for work: %w", err)
	}
	result := make([]TaskBeanInfo, len(rows))
	for i, row := range rows {
		result[i] = TaskBeanInfo{
			TaskID: row.TaskID,
			BeanID: row.BeanID,
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

	// Delete task_beans (foreign key constraint)
	_, err = qtx.DeleteTaskBeansByTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task_beans for task %s: %w", taskID, err)
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

// ResetTaskBeanStatuses resets all bean statuses for a task to pending.
func (db *DB) ResetTaskBeanStatuses(ctx context.Context, taskID string) error {
	_, err := db.queries.ResetTaskBeanStatuses(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to reset task bean statuses: %w", err)
	}
	return nil
}

// GetTaskBeansWithStatus returns all beans in a task with their status.
func (db *DB) GetTaskBeansWithStatus(ctx context.Context, taskID string) ([]TaskBeanInfo, error) {
	rows, err := db.queries.GetTaskBeansWithStatus(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans with status: %w", err)
	}

	result := make([]TaskBeanInfo, len(rows))
	for i, row := range rows {
		result[i] = TaskBeanInfo{
			TaskID: row.TaskID,
			BeanID: row.BeanID,
			Status: row.Status,
		}
	}
	return result, nil
}

// ResetTaskBeanStatus resets a single bean's status in a task to pending.
func (db *DB) ResetTaskBeanStatus(ctx context.Context, taskID, beanID string) error {
	rows, err := db.queries.ResetTaskBeanStatus(ctx, sqlc.ResetTaskBeanStatusParams{
		TaskID: taskID,
		BeanID: beanID,
	})
	if err != nil {
		return fmt.Errorf("failed to reset task bean status: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task bean %s/%s not found", taskID, beanID)
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

// CheckAndCompleteTask checks if all beans in a task are completed and marks the task as complete if so.
// Returns true if the task was auto-completed, false if it still has pending beans.
func (db *DB) CheckAndCompleteTask(ctx context.Context, taskID string, prURL string) (bool, error) {
	// Count the bean statuses
	total, completed, err := db.CountTaskBeanStatuses(ctx, taskID)
	if err != nil {
		return false, err
	}

	// If all beans are complete, mark the task as complete
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
