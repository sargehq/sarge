package beads

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIReader_ImplementsReader(t *testing.T) {
	// Compile-time check is in cli_reader.go, but let's verify at runtime too
	var _ Reader = (*CLIReader)(nil)
}

func TestBeadFromBDShow(t *testing.T) {
	issue := bdShowIssue{
		ID:        "test-1",
		Title:     "Test Issue",
		Status:    "open",
		Priority:  1,
		IssueType: "task",
		Owner:     "alice",
	}

	bead := beadFromBDShow(issue)
	require.Equal(t, "test-1", bead.ID)
	require.Equal(t, "Test Issue", bead.Title)
	require.Equal(t, "open", bead.Status)
	require.Equal(t, 1, bead.Priority)
	require.Equal(t, "task", bead.Type)
	require.Equal(t, "alice", bead.Owner)
	require.False(t, bead.IsEpic)
}

func TestBeadFromBDShow_Epic(t *testing.T) {
	issue := bdShowIssue{
		ID:        "test-2",
		Title:     "Epic Issue",
		IssueType: "epic",
	}

	bead := beadFromBDShow(issue)
	require.True(t, bead.IsEpic)
}

func TestBeadFromBDList(t *testing.T) {
	issue := bdListIssue{
		ID:        "test-3",
		Title:     "List Issue",
		Status:    "closed",
		Priority:  2,
		IssueType: "bug",
	}

	bead := beadFromBDList(issue)
	require.Equal(t, "test-3", bead.ID)
	require.Equal(t, "List Issue", bead.Title)
	require.Equal(t, "closed", bead.Status)
	require.Equal(t, "bug", bead.Type)
}

func TestCLIReader_GetBeadsWithDeps_Empty(t *testing.T) {
	ctx := context.Background()
	reader, err := NewCLIReader(ctx, CLIReaderConfig{
		BeadsDir: "/nonexistent",
	})
	require.NoError(t, err)
	defer reader.Close()

	result, err := reader.GetBeadsWithDeps(ctx, []string{})
	require.NoError(t, err)
	require.Empty(t, result.Beads)
	require.Empty(t, result.Dependencies)
	require.Empty(t, result.Dependents)
}
