package linear_test

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/linear"
	"github.com/stretchr/testify/require"
)

// TestLinearImportIntegration demonstrates the complete Linear import workflow
// This test serves as documentation for how to use the Linear import API
func TestLinearImportIntegration(t *testing.T) {
	// Skip in CI or if Linear API key is not set
	t.Skip("Integration test - requires Linear API key and beads setup")

	ctx := context.Background()

	// Initialize the fetcher with API key and beads directory
	apiKey := "lin_api_test_key" // In production, get from env or config
	beadsDir := "/path/to/beads" // In production, auto-detect or get from config

	fetcher, err := linear.NewFetcher(apiKey, beadsDir)
	require.NoError(t, err, "Failed to create fetcher")

	// Example 1: Simple import of a single issue
	t.Run("ImportSingleIssue", func(t *testing.T) {
		result, err := fetcher.FetchAndImport(ctx, "ENG-123", nil)
		require.NoError(t, err, "Failed to import issue")
		require.True(t, result.Success, "Import failed: %s", result.SkipReason)

		t.Logf("Imported Linear issue %s as bead %s", result.LinearID, result.BeanID)
	})

	// Example 2: Import with options
	t.Run("ImportWithOptions", func(t *testing.T) {
		opts := &linear.ImportOptions{
			CreateDeps:     true,  // Import blocking issues as dependencies
			UpdateExisting: false, // Skip if already imported
			DryRun:         false, // Actually create the beads
			MaxDepDepth:    2,     // Import up to 2 levels of dependencies
		}

		result, err := fetcher.FetchAndImport(ctx, "ENG-456", opts)
		require.NoError(t, err, "Failed to import issue")

		if result.SkipReason == "already imported" {
			t.Logf("Issue already imported as bead %s", result.BeanID)
		} else if result.Success {
			t.Logf("Imported Linear issue %s as bead %s", result.LinearID, result.BeanID)
		}
	})

	// Example 3: Import by URL
	t.Run("ImportByURL", func(t *testing.T) {
		url := "https://linear.app/company/issue/ENG-789/feature-title"
		result, err := fetcher.FetchAndImport(ctx, url, nil)
		require.NoError(t, err, "Failed to import issue")

		if result.Success {
			t.Logf("Imported Linear issue from URL: %s -> bead %s", result.LinearURL, result.BeanID)
		}
	})

	// Example 4: Batch import
	t.Run("BatchImport", func(t *testing.T) {
		issues := []string{"ENG-100", "ENG-101", "ENG-102"}

		opts := &linear.ImportOptions{
			CreateDeps: false,
			DryRun:     false,
		}

		results, err := fetcher.FetchBatch(ctx, issues, opts)
		require.NoError(t, err, "Batch import failed")

		successCount := 0
		for _, result := range results {
			if result.Success {
				successCount++
				t.Logf("✓ %s -> %s", result.LinearID, result.BeanID)
			} else if result.Error != nil {
				t.Logf("✗ %s: %v", result.LinearID, result.Error)
			} else {
				t.Logf("○ %s: %s", result.LinearID, result.SkipReason)
			}
		}

		t.Logf("Imported %d/%d issues successfully", successCount, len(issues))
	})

	// Example 5: Update existing bead from Linear
	t.Run("UpdateExisting", func(t *testing.T) {
		opts := &linear.ImportOptions{
			UpdateExisting: true, // Update if already imported
		}

		result, err := fetcher.FetchAndImport(ctx, "ENG-123", opts)
		require.NoError(t, err, "Failed to update issue")

		if result.SkipReason == "updated existing bead" {
			t.Logf("Updated existing bead %s with latest data from Linear", result.BeanID)
		} else if result.Success {
			t.Logf("Created new bead %s", result.BeanID)
		}
	})

	// Example 6: Import with filters
	t.Run("ImportWithFilters", func(t *testing.T) {
		opts := &linear.ImportOptions{
			StatusFilter:   "in_progress", // Only import if status matches
			PriorityFilter: "P1",          // Only import if priority matches
			AssigneeFilter: "john@company.com",
		}

		result, err := fetcher.FetchAndImport(ctx, "ENG-999", opts)
		require.NoError(t, err, "Failed to import")

		if result.SkipReason != "" {
			t.Logf("Skipped: %s", result.SkipReason)
		} else if result.Success {
			t.Logf("Imported bead %s (matched filters)", result.BeanID)
		}
	})

	// Example 7: Dry run (preview without creating)
	t.Run("DryRun", func(t *testing.T) {
		opts := &linear.ImportOptions{
			DryRun: true,
		}

		result, err := fetcher.FetchAndImport(ctx, "ENG-777", opts)
		require.NoError(t, err, "Failed dry run")

		if result.SkipReason == "dry run" {
			t.Logf("Dry run successful - would import Linear issue %s", result.LinearID)
		}
	})
}

// TestLinearImportErrorHandling demonstrates error handling patterns
func TestLinearImportErrorHandling(t *testing.T) {
	t.Skip("Integration test - requires Linear API key and beads setup")

	ctx := context.Background()

	// Example: Invalid API key
	t.Run("InvalidAPIKey", func(t *testing.T) {
		_, err := linear.NewFetcher("invalid_key", "/path/to/beads")
		require.Error(t, err, "Expected error for invalid API key")
		t.Logf("Got expected error: %v", err)
	})

	// Example: Invalid issue ID
	t.Run("InvalidIssueID", func(t *testing.T) {
		fetcher, _ := linear.NewFetcher("valid_key", "/path/to/beads")
		result, err := fetcher.FetchAndImport(ctx, "INVALID-999", nil)
		require.True(t, err != nil || !result.Success, "Expected error for invalid issue ID")
		if result.Error != nil {
			t.Logf("Got expected error: %v", result.Error)
		}
	})

	// Example: Network errors
	t.Run("NetworkError", func(t *testing.T) {
		// Simulate network error by using invalid endpoint
		// In production, this would be a real network failure
		fetcher, _ := linear.NewFetcher("valid_key", "/path/to/beads")
		result, err := fetcher.FetchAndImport(ctx, "ENG-123", nil)
		if err != nil {
			t.Logf("Network error (expected in test): %v", err)
		}
		if result.Error != nil {
			t.Logf("Result error: %v", result.Error)
		}
	})
}
