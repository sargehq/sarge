package orchestration

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a minimal test database for orchestration tests.
func setupTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()
	ctx := context.Background()

	database, err := db.OpenPath(ctx, ":memory:")
	require.NoError(t, err, "failed to open database")

	cleanup := func() {
		database.Close()
	}

	return database, cleanup
}

// createTestWork creates a work record for testing.
func createTestWork(ctx context.Context, t *testing.T, database *db.DB, workID, branchName string) {
	t.Helper()
	err := database.CreateWork(ctx, workID, workID, "", branchName, "main", "root-1", false)
	require.NoError(t, err)
}

func TestCountReviewIterations(t *testing.T) {
	ctx := context.Background()
	database, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("counts review tasks correctly", func(t *testing.T) {
		// Create work
		createTestWork(ctx, t, database, "w-review-count", "review-branch")
		defer database.DeleteWork(ctx, "w-review-count")

		// Create some implement tasks and review tasks
		err := database.CreateTask(ctx, "w-review-count.1", "implement", []string{"bead-1"}, 10, "w-review-count")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-review-count.2", "review", []string{}, 10, "w-review-count")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-review-count.3", "review", []string{}, 10, "w-review-count")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-review-count.4", "implement", []string{"bead-2"}, 10, "w-review-count")
		require.NoError(t, err)

		// Count review iterations
		count := CountReviewIterations(ctx, database, "w-review-count")
		assert.Equal(t, 2, count)
	})

	t.Run("returns zero when no review tasks", func(t *testing.T) {
		// Create work
		createTestWork(ctx, t, database, "w-no-review", "no-review-branch")
		defer database.DeleteWork(ctx, "w-no-review")

		// Create only implement tasks
		err := database.CreateTask(ctx, "w-no-review.1", "implement", []string{"bead-1"}, 10, "w-no-review")
		require.NoError(t, err)

		// Count review iterations
		count := CountReviewIterations(ctx, database, "w-no-review")
		assert.Equal(t, 0, count)
	})

	t.Run("returns zero for work with no tasks", func(t *testing.T) {
		// Create work with no tasks
		createTestWork(ctx, t, database, "w-empty-review", "empty-review-branch")
		defer database.DeleteWork(ctx, "w-empty-review")

		// Count review iterations
		count := CountReviewIterations(ctx, database, "w-empty-review")
		assert.Equal(t, 0, count)
	})

	t.Run("returns zero for nonexistent work", func(t *testing.T) {
		// Count review iterations for nonexistent work
		count := CountReviewIterations(ctx, database, "nonexistent")
		assert.Equal(t, 0, count)
	})

	t.Run("counts only review tasks mixed with other types", func(t *testing.T) {
		// Create work
		createTestWork(ctx, t, database, "w-mixed", "mixed-branch")
		defer database.DeleteWork(ctx, "w-mixed")

		// Create mixed task types
		err := database.CreateTask(ctx, "w-mixed.1", "estimate", []string{}, 10, "w-mixed")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-mixed.2", "review", []string{}, 10, "w-mixed")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-mixed.3", "pr", []string{}, 10, "w-mixed")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-mixed.4", "review", []string{}, 10, "w-mixed")
		require.NoError(t, err)

		err = database.CreateTask(ctx, "w-mixed.5", "review", []string{}, 10, "w-mixed")
		require.NoError(t, err)

		// Count review iterations
		count := CountReviewIterations(ctx, database, "w-mixed")
		assert.Equal(t, 3, count)
	})
}

func TestSpinnerFrames(t *testing.T) {
	// Verify SpinnerFrames is correctly defined
	assert.NotEmpty(t, SpinnerFrames)
	assert.Len(t, SpinnerFrames, 10) // Expected number of frames

	// Verify all frames are non-empty strings
	for i, frame := range SpinnerFrames {
		assert.NotEmpty(t, frame, "SpinnerFrame[%d] should not be empty", i)
	}
}
