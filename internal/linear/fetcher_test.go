package linear

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetcherErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid URL format", func(t *testing.T) {
		fetcher := &Fetcher{
			client:     nil, // Won't be called
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		result, err := fetcher.FetchAndImport(ctx, "https://not-linear.com/issue/123", nil)
		require.Error(t, err, "expected error for invalid URL")
		assert.NotNil(t, result.Error, "expected result.Error to be set")
		assert.False(t, result.Success, "expected success to be false")
	})

	t.Run("empty input", func(t *testing.T) {
		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		result, err := fetcher.FetchAndImport(ctx, "", nil)
		require.Error(t, err, "expected error for empty input")
		assert.NotNil(t, result.Error, "expected result.Error to be set")
	})

	t.Run("whitespace only input", func(t *testing.T) {
		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		result, err := fetcher.FetchAndImport(ctx, "   ", nil)
		require.Error(t, err, "expected error for whitespace input")
		assert.NotNil(t, result.Error, "expected result.Error to be set")
	})

	t.Run("invalid issue ID format", func(t *testing.T) {
		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		invalidIDs := []string{
			"ENG123",    // missing dash
			"ENG-",      // missing number
			"123-ENG",   // reversed format
			"ENG_123",   // wrong separator
			"ENG-12-34", // too many parts
		}

		for _, id := range invalidIDs {
			t.Run("invalid_id_"+id, func(t *testing.T) {
				result, err := fetcher.FetchAndImport(ctx, id, nil)
				require.Error(t, err, "expected error for invalid ID %s", id)
				assert.NotNil(t, result.Error, "expected result.Error to be set for %s", id)
			})
		}
	})

	t.Run("cached result returns immediately", func(t *testing.T) {
		fetcher := &Fetcher{
			client:   nil, // Won't be called since it's cached
			beadsDir: ".",
			beadsCache: map[string]string{
				"ENG-123": "beads-abc",
			},
		}

		result, err := fetcher.FetchAndImport(ctx, "ENG-123", nil)
		require.NoError(t, err)
		assert.True(t, result.Success, "expected success for cached result")
		assert.Equal(t, "beads-abc", result.BeanID)
		assert.Equal(t, "already imported (cached)", result.SkipReason)
	})
}

func TestFetcherFiltering(t *testing.T) {
	// Create a mock client that returns a predefined issue
	mockIssue := &Issue{
		Identifier:  "ENG-123",
		Title:       "Test Issue",
		Description: "Test description",
		Priority:    1,
		State:       State{Type: "started"},
		URL:         "https://linear.app/test/issue/ENG-123",
		Assignee:    &User{Email: "john@example.com"},
	}

	t.Run("filter by status - match", func(t *testing.T) {
		// This test demonstrates the filtering logic
		// In a real test, we would need to mock the beads client as well
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by status - no match", func(t *testing.T) {
		// This test demonstrates the filtering logic
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by priority - match", func(t *testing.T) {
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by priority - no match", func(t *testing.T) {
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by assignee - match", func(t *testing.T) {
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by assignee - no match", func(t *testing.T) {
		t.Skip("Requires beads client mocking")
	})

	t.Run("filter by assignee - no assignee", func(t *testing.T) {
		// Issue has no assignee, filter should not match
		t.Skip("Requires beads client mocking")
	})

	// Silence unused variable warning
	_ = mockIssue
}

func TestFetcherDryRun(t *testing.T) {
	t.Run("dry run skips bead creation", func(t *testing.T) {
		// This test would verify that dry run mode doesn't create beads
		t.Skip("Requires full mock setup")
	})
}

func TestFetchBatchErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("empty batch", func(t *testing.T) {
		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		results, err := fetcher.FetchBatch(ctx, []string{}, nil)
		require.NoError(t, err, "unexpected error for empty batch")
		assert.Empty(t, results)
	})

	t.Run("batch with mixed valid and invalid IDs", func(t *testing.T) {
		// This test requires a mock client implementation
		// For now, we only test that invalid IDs fail during parsing
		t.Skip("Requires mock client implementation")

		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		ids := []string{"ENG-123", "INVALID", "ENG-456"}
		results, err := fetcher.FetchBatch(ctx, ids, nil)
		require.NoError(t, err, "batch should continue on individual errors")
		require.Len(t, results, 3)
		// Second result should have an error
		assert.NotNil(t, results[1].Error, "expected error for invalid ID in batch")
	})

	t.Run("context cancellation", func(t *testing.T) {
		// This test requires a mock client implementation
		t.Skip("Requires mock client implementation")

		fetcher := &Fetcher{
			client:     nil,
			beadsDir:   ".",
			beadsCache: make(map[string]string),
		}

		// Create a cancelled context
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		ids := []string{"ENG-123", "ENG-456"}
		results, err := fetcher.FetchBatch(cancelledCtx, ids, nil)
		// Should return error from context cancellation eventually
		// The exact behavior depends on when the cancellation is detected
		_ = results
		_ = err
	})
}

func TestNewFetcher(t *testing.T) {
	t.Run("creates fetcher with valid API key", func(t *testing.T) {
		fetcher, err := NewFetcher("test-api-key", ".")
		require.NoError(t, err)
		require.NotNil(t, fetcher)
		assert.NotNil(t, fetcher.client, "expected client to be initialized")
		assert.NotNil(t, fetcher.beadsCache, "expected beadsCache to be initialized")
	})

	t.Run("fails with empty API key", func(t *testing.T) {
		fetcher, err := NewFetcher("", ".")
		require.Error(t, err, "expected error for empty API key")
		assert.Nil(t, fetcher, "expected nil fetcher on error")
	})
}
