package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAndGetTaskMetadata(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Create a task first
	err := db.CreateTask(ctx, "task-1", "review", nil, 0, workID, 0)
	require.NoError(t, err, "CreateTask failed")

	// Set metadata
	err = db.SetTaskMetadata(ctx, "task-1", "auto_workflow", "false")
	require.NoError(t, err, "SetTaskMetadata failed")

	// Get metadata
	value, err := db.GetTaskMetadata(ctx, "task-1", "auto_workflow")
	require.NoError(t, err, "GetTaskMetadata failed")
	assert.Equal(t, "false", value)
}

func TestGetTaskMetadata_NotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Create a task first
	err := db.CreateTask(ctx, "task-1", "review", nil, 0, workID, 0)
	require.NoError(t, err, "CreateTask failed")

	// Get non-existent metadata - should return empty string and nil error
	value, err := db.GetTaskMetadata(ctx, "task-1", "nonexistent_key")
	require.NoError(t, err, "GetTaskMetadata should not error for non-existent key")
	assert.Empty(t, value, "expected empty string for non-existent metadata key")
}

func TestSetTaskMetadata_UpdateExisting(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Create a task first
	err := db.CreateTask(ctx, "task-1", "review", nil, 0, workID, 0)
	require.NoError(t, err, "CreateTask failed")

	// Set metadata initially
	err = db.SetTaskMetadata(ctx, "task-1", "auto_workflow", "true")
	require.NoError(t, err, "SetTaskMetadata failed")

	// Update metadata
	err = db.SetTaskMetadata(ctx, "task-1", "auto_workflow", "false")
	require.NoError(t, err, "SetTaskMetadata update failed")

	// Verify updated value
	value, err := db.GetTaskMetadata(ctx, "task-1", "auto_workflow")
	require.NoError(t, err, "GetTaskMetadata failed")
	assert.Equal(t, "false", value)
}

func TestSetTaskMetadata_MultipleKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Create a task first
	err := db.CreateTask(ctx, "task-1", "review", nil, 0, workID, 0)
	require.NoError(t, err, "CreateTask failed")

	// Set multiple metadata keys
	err = db.SetTaskMetadata(ctx, "task-1", "auto_workflow", "false")
	require.NoError(t, err, "SetTaskMetadata auto_workflow failed")

	err = db.SetTaskMetadata(ctx, "task-1", "review_epic_id", "epic-123")
	require.NoError(t, err, "SetTaskMetadata review_epic_id failed")

	// Verify both values
	autoWorkflow, err := db.GetTaskMetadata(ctx, "task-1", "auto_workflow")
	require.NoError(t, err, "GetTaskMetadata auto_workflow failed")
	assert.Equal(t, "false", autoWorkflow)

	reviewEpic, err := db.GetTaskMetadata(ctx, "task-1", "review_epic_id")
	require.NoError(t, err, "GetTaskMetadata review_epic_id failed")
	assert.Equal(t, "epic-123", reviewEpic)
}

func TestTaskMetadata_IndependentPerTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Create two tasks
	err := db.CreateTask(ctx, "task-1", "review", nil, 0, workID, 0)
	require.NoError(t, err)
	err = db.CreateTask(ctx, "task-2", "review", nil, 0, workID, 0)
	require.NoError(t, err)

	// Set different values for the same key on different tasks
	err = db.SetTaskMetadata(ctx, "task-1", "auto_workflow", "false")
	require.NoError(t, err)
	err = db.SetTaskMetadata(ctx, "task-2", "auto_workflow", "true")
	require.NoError(t, err)

	// Verify values are independent
	val1, err := db.GetTaskMetadata(ctx, "task-1", "auto_workflow")
	require.NoError(t, err)
	assert.Equal(t, "false", val1)

	val2, err := db.GetTaskMetadata(ctx, "task-2", "auto_workflow")
	require.NoError(t, err)
	assert.Equal(t, "true", val2)
}

func TestAutoWorkflowMetadata_ManualReviewTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Simulate creating a manual review task (like TUI does)
	reviewTaskID := "work-1.1"
	err := db.CreateTask(ctx, reviewTaskID, "review", nil, 0, workID, 0)
	require.NoError(t, err)

	// Set auto_workflow=false as TUI does for manual reviews
	err = db.SetTaskMetadata(ctx, reviewTaskID, "auto_workflow", "false")
	require.NoError(t, err)

	// Verify the metadata indicates this is a manual review
	autoWorkflow, err := db.GetTaskMetadata(ctx, reviewTaskID, "auto_workflow")
	require.NoError(t, err)
	assert.Equal(t, "false", autoWorkflow, "manual review tasks should have auto_workflow=false")
}

func TestAutoWorkflowMetadata_AutomatedReviewTask(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()
	workID := createTestWork(t, db)

	// Simulate creating an automated review task (no metadata set)
	reviewTaskID := "work-1.1"
	err := db.CreateTask(ctx, reviewTaskID, "review", nil, 0, workID, 0)
	require.NoError(t, err)

	// Automated review tasks don't have auto_workflow metadata
	// GetTaskMetadata returns empty string when key doesn't exist
	value, err := db.GetTaskMetadata(ctx, reviewTaskID, "auto_workflow")
	require.NoError(t, err)
	assert.Empty(t, value, "automated review tasks should not have auto_workflow metadata set")

	// The workflow check: if err == nil && autoWorkflow == "false" -> skip
	// For automated reviews: value is empty, so the check fails and workflow continues
	shouldSkipWorkflow := (err == nil && value == "false")
	assert.False(t, shouldSkipWorkflow, "automated reviews should not skip workflow")
}
