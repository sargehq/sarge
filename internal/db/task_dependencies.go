package db

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// AddTaskDependency adds a dependency between two tasks.
// The task with taskID will depend on the task with dependsOnTaskID,
// meaning dependsOnTaskID must complete before taskID can run.
func (db *DB) AddTaskDependency(ctx context.Context, taskID, dependsOnTaskID string) error {
	err := db.queries.AddTaskDependency(ctx, sqlc.AddTaskDependencyParams{
		TaskID:          taskID,
		DependsOnTaskID: dependsOnTaskID,
	})
	if err != nil {
		return fmt.Errorf("failed to add task dependency %s -> %s: %w", taskID, dependsOnTaskID, err)
	}
	return nil
}

// GetTaskDependencies returns the IDs of tasks that the given task depends on.
func (db *DB) GetTaskDependencies(ctx context.Context, taskID string) ([]string, error) {
	deps, err := db.queries.GetTaskDependencies(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task dependencies for %s: %w", taskID, err)
	}
	return deps, nil
}

// GetTaskDependents returns the IDs of tasks that depend on the given task.
func (db *DB) GetTaskDependents(ctx context.Context, taskID string) ([]string, error) {
	deps, err := db.queries.GetTaskDependents(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task dependents for %s: %w", taskID, err)
	}
	return deps, nil
}

// GetReadyTasksForWork returns tasks that are pending and have all dependencies satisfied.
// Tasks are returned in position order.
func (db *DB) GetReadyTasksForWork(ctx context.Context, workID string) ([]*Task, error) {
	tasks, err := db.queries.GetReadyTasksForWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get ready tasks for work %s: %w", workID, err)
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

// HasPendingDependencies checks if a task has any dependencies that haven't completed.
func (db *DB) HasPendingDependencies(ctx context.Context, taskID string) (bool, error) {
	hasPending, err := db.queries.HasPendingDependencies(ctx, taskID)
	if err != nil {
		return false, fmt.Errorf("failed to check pending dependencies for %s: %w", taskID, err)
	}
	return hasPending, nil
}

// DeleteTaskDependencies removes all dependencies for a task.
func (db *DB) DeleteTaskDependencies(ctx context.Context, taskID string) error {
	_, err := db.queries.DeleteTaskDependencies(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task dependencies for %s: %w", taskID, err)
	}
	return nil
}

// DeleteTaskDependency removes a single dependency between two tasks.
func (db *DB) DeleteTaskDependency(ctx context.Context, taskID, dependsOnTaskID string) error {
	_, err := db.queries.DeleteTaskDependency(ctx, sqlc.DeleteTaskDependencyParams{
		TaskID:          taskID,
		DependsOnTaskID: dependsOnTaskID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete task dependency %s -> %s: %w", taskID, dependsOnTaskID, err)
	}
	return nil
}

// DeleteTaskDependenciesForWork removes all task dependencies for tasks in a work.
func (db *DB) DeleteTaskDependenciesForWork(ctx context.Context, workID string) error {
	_, err := db.queries.DeleteTaskDependenciesForWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to delete task dependencies for work %s: %w", workID, err)
	}
	return nil
}
