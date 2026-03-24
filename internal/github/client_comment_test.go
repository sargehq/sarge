package github

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPostPRComment(t *testing.T) {
	// This test requires a real PR URL and GitHub authentication
	// It's meant to be run manually for verification

	// Skip if not in manual test mode
	if os.Getenv("MANUAL_GITHUB_TEST") != "1" {
		t.Skip("Skipping manual GitHub test. Set MANUAL_GITHUB_TEST=1 to run")
	}

	// You need to set a real PR URL here when testing manually
	prURL := os.Getenv("TEST_PR_URL")
	require.NotEmpty(t, prURL, "TEST_PR_URL environment variable must be set for manual testing")

	client := NewClient()
	ctx := context.Background()

	// Test posting a simple comment
	err := client.PostPRComment(ctx, prURL, "Test comment from sarge integration test")
	require.NoError(t, err, "Failed to post PR comment")

	t.Log("Successfully posted PR comment")
}

func TestPostReplyToComment(t *testing.T) {
	// This test requires a real PR URL and comment ID
	// It's meant to be run manually for verification

	// Skip if not in manual test mode
	if os.Getenv("MANUAL_GITHUB_TEST") != "1" {
		t.Skip("Skipping manual GitHub test. Set MANUAL_GITHUB_TEST=1 to run")
	}

	prURL := os.Getenv("TEST_PR_URL")
	require.NotEmpty(t, prURL, "TEST_PR_URL environment variable must be set for manual testing")

	// You need to set a real comment ID here when testing manually
	commentID := 123456789 // Replace with actual comment ID

	client := NewClient()
	ctx := context.Background()

	// Test posting a reply to a comment
	err := client.PostReplyToComment(ctx, prURL, commentID, "Test reply from sarge integration test")
	require.NoError(t, err, "Failed to post reply to comment")

	t.Log("Successfully posted reply to comment")
}

func TestCommentIntegration(t *testing.T) {
	// This test demonstrates the full flow of creating a bean from a comment
	// and posting back an acknowledgment

	// Skip if not in manual test mode
	if os.Getenv("MANUAL_GITHUB_TEST") != "1" {
		t.Skip("Skipping manual GitHub test. Set MANUAL_GITHUB_TEST=1 to run")
	}

	prURL := os.Getenv("TEST_PR_URL")
	require.NotEmpty(t, prURL, "TEST_PR_URL environment variable must be set for manual testing")

	client := NewClient()
	ctx := context.Background()

	// Simulate creating a bean from feedback
	beanID := "beans-test-123"
	feedbackTitle := "Fix test failure in authentication module"
	priority := 2

	// Post acknowledgment message
	ackMessage := fmt.Sprintf("✅ Created tracking issue **%s** for this feedback.\n\nTitle: %s\nPriority: P%d",
		beanID, feedbackTitle, priority)

	err := client.PostPRComment(ctx, prURL, ackMessage)
	require.NoError(t, err, "Failed to post acknowledgment")

	t.Log("Successfully posted bean acknowledgment to PR")
}
