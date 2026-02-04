package task

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestHashDescription(t *testing.T) {
	// Same inputs should produce same hash
	hash1 := db.HashDescription("Title\nDescription")
	hash2 := db.HashDescription("Title\nDescription")
	assert.Equal(t, hash1, hash2, "same inputs should produce same hash")

	// Different inputs should produce different hash
	hash3 := db.HashDescription("Different Title\nDescription")
	assert.NotEqual(t, hash1, hash3, "different inputs should produce different hash")

	// Hash should be hex encoded
	assert.Len(t, hash1, 64, "expected 64 char hex hash (SHA256 produces 32 bytes)")
}

func TestCacheComplexity(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Cache a complexity estimate
	err := database.CacheComplexity(ctx, "bead-1", "hash123", 5, 10000)
	require.NoError(t, err, "CacheComplexity failed")

	// Retrieve it
	score, tokens, found, err := database.GetCachedComplexity(ctx, "bead-1", "hash123")
	require.NoError(t, err, "GetCachedComplexity failed")

	assert.True(t, found, "expected to find cached complexity")
	assert.Equal(t, 5, score, "expected score 5")
	assert.Equal(t, 10000, tokens, "expected tokens 10000")

	// Different hash should not match
	_, _, found, err = database.GetCachedComplexity(ctx, "bead-1", "differenthash")
	require.NoError(t, err, "GetCachedComplexity failed")
	assert.False(t, found, "expected no result for different hash")
}

func TestCacheUpdate(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Cache initial estimate
	database.CacheComplexity(ctx, "bead-1", "hash1", 3, 5000)

	// Update with new hash (description changed)
	err := database.CacheComplexity(ctx, "bead-1", "hash2", 7, 15000)
	require.NoError(t, err, "CacheComplexity update failed")

	// Old hash should not match
	_, _, found, _ := database.GetCachedComplexity(ctx, "bead-1", "hash1")
	assert.False(t, found, "old hash should not match after update")

	// New hash should match
	score, tokens, found, err := database.GetCachedComplexity(ctx, "bead-1", "hash2")
	require.NoError(t, err, "GetCachedComplexity failed")
	assert.True(t, found, "expected to find updated complexity")
	assert.Equal(t, 7, score, "expected score 7")
	assert.Equal(t, 15000, tokens, "expected tokens 15000")
}

func TestNewLLMEstimator(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Test creation with database
	estimator := NewLLMEstimator(database, "/tmp/test", "test-project", "work-test")
	require.NotNil(t, estimator, "expected non-nil estimator")
	assert.Equal(t, database, estimator.database, "database not set correctly")

	// Test creation with nil database (should still work for non-cached usage)
	estimator = NewLLMEstimator(nil, "/tmp/test", "test-project", "work-test")
	require.NotNil(t, estimator, "expected non-nil estimator even with nil database")
}

// Note: Testing the actual Estimate and EstimateBatch functions would require
// a running Claude Code instance, which is beyond the scope of unit tests.
// The caching behavior is tested via the database methods above.
