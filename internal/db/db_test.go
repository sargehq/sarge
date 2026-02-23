package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*DB, func()) {
	t.Helper()

	db, err := OpenPath(context.Background(), ":memory:")
	require.NoError(t, err, "failed to open database")

	cleanup := func() {
		db.Close()
	}

	return db, cleanup
}

func TestOpen(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	require.NotNil(t, db, "expected non-nil database")

	// Verify schema was created by querying the table
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM beans").Scan(&count)
	require.NoError(t, err, "failed to query beans table")
	assert.Equal(t, 0, count, "expected 0 beans")
}

func TestStartBean(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := db.StartBean(ctx, "test-1", "Test Bean", "session-1", "pane-1")
	require.NoError(t, err, "StartBean failed")

	// Verify bean was created
	bean, err := db.GetBean(ctx, "test-1")
	require.NoError(t, err, "GetBean failed")
	require.NotNil(t, bean, "expected bean, got nil")
	assert.Equal(t, "test-1", bean.ID)
	assert.Equal(t, "Test Bean", bean.Title)
	assert.Equal(t, StatusProcessing, bean.Status)
	assert.Equal(t, "session-1", bean.ZellijSession)
	assert.Equal(t, "pane-1", bean.ZellijPane)
	assert.NotNil(t, bean.StartedAt, "expected StartedAt to be set")
}

func TestStartBeanUpsert(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create initial bean
	err := db.StartBean(ctx, "test-1", "Original Title", "session-1", "pane-1")
	require.NoError(t, err, "first StartBean failed")

	// Update with new values (upsert)
	err = db.StartBean(ctx, "test-1", "Updated Title", "session-2", "pane-2")
	require.NoError(t, err, "second StartBean failed")

	// Verify bean was updated
	bean, err := db.GetBean(ctx, "test-1")
	require.NoError(t, err, "GetBean failed")
	assert.Equal(t, "Updated Title", bean.Title)
	assert.Equal(t, "session-2", bean.ZellijSession)
}

func TestCompleteBean(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create bean first
	err := db.StartBean(ctx, "test-1", "Test Bean", "session-1", "pane-1")
	require.NoError(t, err, "StartBean failed")

	// Complete it
	err = db.CompleteBean(ctx, "test-1", "https://github.com/example/pr/1")
	require.NoError(t, err, "CompleteBean failed")

	// Verify status and PR URL
	bean, err := db.GetBean(ctx, "test-1")
	require.NoError(t, err, "GetBean failed")
	assert.Equal(t, StatusCompleted, bean.Status)
	assert.Equal(t, "https://github.com/example/pr/1", bean.PRURL)
	assert.NotNil(t, bean.CompletedAt, "expected CompletedAt to be set")
}

func TestCompleteBeanNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := db.CompleteBean(ctx, "nonexistent", "")
	assert.Error(t, err, "expected error for nonexistent bean")
}

func TestFailBean(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create bean first
	err := db.StartBean(ctx, "test-1", "Test Bean", "session-1", "pane-1")
	require.NoError(t, err, "StartBean failed")

	// Fail it
	err = db.FailBean(ctx, "test-1", "something went wrong")
	require.NoError(t, err, "FailBean failed")

	// Verify status and error message
	bean, err := db.GetBean(ctx, "test-1")
	require.NoError(t, err, "GetBean failed")
	assert.Equal(t, StatusFailed, bean.Status)
	assert.Equal(t, "something went wrong", bean.ErrorMessage)
	assert.NotNil(t, bean.CompletedAt, "expected CompletedAt to be set")
}

func TestFailBeanNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	err := db.FailBean(ctx, "nonexistent", "error")
	assert.Error(t, err, "expected error for nonexistent bean")
}

func TestGetBeanNotFound(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	bean, err := db.GetBean(ctx, "nonexistent")
	require.NoError(t, err, "GetBean failed")
	assert.Nil(t, bean, "expected nil for nonexistent bean")
}

func TestIsCompleted(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Nonexistent bean
	completed, err := db.IsCompleted(ctx, "nonexistent")
	require.NoError(t, err, "IsCompleted failed")
	assert.False(t, completed, "expected false for nonexistent bean")

	// Processing bean
	db.StartBean(ctx, "test-1", "Test", "s", "p")
	completed, err = db.IsCompleted(ctx, "test-1")
	require.NoError(t, err, "IsCompleted failed")
	assert.False(t, completed, "expected false for processing bean")

	// Completed bean
	db.CompleteBean(ctx, "test-1", "")
	completed, err = db.IsCompleted(ctx, "test-1")
	require.NoError(t, err, "IsCompleted failed")
	assert.True(t, completed, "expected true for completed bean")

	// Failed bean also counts as completed
	db.StartBean(ctx, "test-2", "Test 2", "s", "p")
	db.FailBean(ctx, "test-2", "error")
	completed, err = db.IsCompleted(ctx, "test-2")
	require.NoError(t, err, "IsCompleted failed")
	assert.True(t, completed, "expected true for failed bean")
}

func TestListBeans(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Create several beans with different statuses
	db.StartBean(ctx, "test-1", "Processing 1", "s", "p")
	db.StartBean(ctx, "test-2", "Processing 2", "s", "p")
	db.StartBean(ctx, "test-3", "Will Complete", "s", "p")
	db.CompleteBean(ctx, "test-3", "")
	db.StartBean(ctx, "test-4", "Will Fail", "s", "p")
	db.FailBean(ctx, "test-4", "error")

	// List all
	beans, err := db.ListBeans(ctx, "")
	require.NoError(t, err, "ListBeans failed")
	assert.Len(t, beans, 4, "expected 4 beans")

	// List processing only
	beans, err = db.ListBeans(ctx, StatusProcessing)
	require.NoError(t, err, "ListBeans failed")
	assert.Len(t, beans, 2, "expected 2 processing beans")

	// List completed only
	beans, err = db.ListBeans(ctx, StatusCompleted)
	require.NoError(t, err, "ListBeans failed")
	assert.Len(t, beans, 1, "expected 1 completed bean")

	// List failed only
	beans, err = db.ListBeans(ctx, StatusFailed)
	require.NoError(t, err, "ListBeans failed")
	assert.Len(t, beans, 1, "expected 1 failed bean")
}

func TestTimestamps(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	db.StartBean(ctx, "test-1", "Test", "s", "p")
	after := time.Now().Add(time.Second)

	bean, _ := db.GetBean(ctx, "test-1")

	assert.True(t, bean.CreatedAt.After(before) && bean.CreatedAt.Before(after), "CreatedAt not within expected range")
	assert.True(t, bean.UpdatedAt.After(before) && bean.UpdatedAt.Before(after), "UpdatedAt not within expected range")
	assert.True(t, bean.StartedAt.After(before) && bean.StartedAt.Before(after), "StartedAt not within expected range")
}

func TestTasksTableExists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Verify tasks table was created
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
	require.NoError(t, err, "failed to query tasks table")
	assert.Equal(t, 0, count, "expected 0 tasks")

	// Create a work first (tasks have FK to works)
	_, err = db.Exec(`INSERT INTO works (id, status) VALUES ('work-1', 'pending')`)
	require.NoError(t, err, "failed to insert work")

	// Insert a task to verify schema
	_, err = db.Exec(`
		INSERT INTO tasks (id, status, complexity_budget, actual_complexity, work_id)
		VALUES ('task-1', 'pending', 100, 50, 'work-1')
	`)
	require.NoError(t, err, "failed to insert task")

	// Verify insertion
	var id, status string
	var budget, actual int
	err = db.QueryRow("SELECT id, status, complexity_budget, actual_complexity FROM tasks WHERE id = 'task-1'").
		Scan(&id, &status, &budget, &actual)
	require.NoError(t, err, "failed to query task")
	assert.Equal(t, "task-1", id)
	assert.Equal(t, "pending", status)
	assert.Equal(t, 100, budget)
	assert.Equal(t, 50, actual)
}

func TestTaskBeansTableExists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a work first (tasks have FK to works)
	_, err := db.Exec(`INSERT INTO works (id, status) VALUES ('work-1', 'pending')`)
	require.NoError(t, err, "failed to insert work")

	// Create a task first (foreign key reference)
	_, err = db.Exec(`INSERT INTO tasks (id, status, work_id) VALUES ('task-1', 'pending', 'work-1')`)
	require.NoError(t, err, "failed to insert task")

	// Insert task_beans entries
	_, err = db.Exec(`
		INSERT INTO task_beans (task_id, bean_id, status)
		VALUES ('task-1', 'bean-1', 'pending'), ('task-1', 'bean-2', 'completed')
	`)
	require.NoError(t, err, "failed to insert task_beans")

	// Verify count
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM task_beans WHERE task_id = 'task-1'").Scan(&count)
	require.NoError(t, err, "failed to query task_beans")
	assert.Equal(t, 2, count, "expected 2 task_beans")
}

func TestComplexityCacheTableExists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert complexity cache entry
	_, err := db.Exec(`
		INSERT INTO complexity_cache (bean_id, description_hash, complexity_score, estimated_tokens)
		VALUES ('bean-1', 'abc123hash', 5, 1000)
	`)
	require.NoError(t, err, "failed to insert complexity_cache")

	// Verify insertion
	var beanID, hash string
	var score, tokens int
	err = db.QueryRow("SELECT bean_id, description_hash, complexity_score, estimated_tokens FROM complexity_cache WHERE bean_id = 'bean-1'").
		Scan(&beanID, &hash, &score, &tokens)
	require.NoError(t, err, "failed to query complexity_cache")
	assert.Equal(t, "bean-1", beanID)
	assert.Equal(t, "abc123hash", hash)
	assert.Equal(t, 5, score)
	assert.Equal(t, 1000, tokens)
}
