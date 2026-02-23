package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestHarness(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Verify harness was created with all mocks
	assert.NotNil(t, h.DB)
	assert.NotNil(t, h.Git)
	assert.NotNil(t, h.Worktree)
	assert.NotNil(t, h.Beads)
	assert.NotNil(t, h.BeadsReader)
	assert.NotNil(t, h.OrchestratorManager)
	assert.NotNil(t, h.NameGenerator)
	assert.NotNil(t, h.TaskPlanner)
	assert.NotNil(t, h.WorkService)
	assert.NotNil(t, h.Config)

	// Verify config defaults
	assert.Equal(t, "test-project", h.Config.Project.Name)
	assert.Equal(t, "main", h.Config.Repo.GetBaseBranch())
}

func TestCreateBead(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	bead := h.CreateBead("bead-1", "Test Bead")

	assert.Equal(t, "bead-1", bead.ID)
	assert.Equal(t, "Test Bead", bead.Title)
	assert.Equal(t, "open", bead.Status)
	assert.Equal(t, "task", bead.Type)

	// Verify it's accessible through BeadsReader
	ctx := context.Background()
	retrieved, err := h.BeadsReader.GetBead(ctx, "bead-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "bead-1", retrieved.ID)
}

func TestCreateEpicWithChildren(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	epic := h.CreateEpicWithChildren("epic-1", "child-1", "child-2")

	assert.Equal(t, "epic-1", epic.ID)
	assert.True(t, epic.IsEpic)
	assert.Equal(t, "epic", epic.Type)

	// Verify children were created
	ctx := context.Background()
	children, err := h.BeadsReader.GetBeadWithChildren(ctx, "epic-1")
	require.NoError(t, err)
	assert.Len(t, children, 3) // epic + 2 children
}

func TestSetBeadDependency(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	h.CreateBead("bead-1", "First")
	h.CreateBead("bead-2", "Second")
	h.SetBeadDependency("bead-2", "bead-1") // bead-2 blocked by bead-1

	ctx := context.Background()
	bead2, err := h.BeadsReader.GetBead(ctx, "bead-2")
	require.NoError(t, err)
	require.Len(t, bead2.Dependencies, 1)
	assert.Equal(t, "bead-1", bead2.Dependencies[0].DependsOnID)
}

func TestCreateWork(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	work := h.CreateWork("w-test", "feat/test-branch")

	assert.Equal(t, "w-test", work.ID)
	assert.Equal(t, "feat/test-branch", work.BranchName)
	assert.Equal(t, "main", work.BaseBranch)
}

func TestCreateTask(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// Create work first
	h.CreateWork("w-test", "feat/test")

	// Create task with beads
	task := h.CreateTask("w-test.1", "w-test", []string{"bead-1", "bead-2"})

	assert.Equal(t, "w-test.1", task.ID)
	assert.Equal(t, "w-test", task.WorkID)
	assert.Equal(t, "pending", task.Status)

	// Verify beads are associated
	ctx := context.Background()
	beanIDs, err := h.DB.GetTaskBeans(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Len(t, beanIDs, 2)
}

func TestCompleteAndFailTask(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	h.CreateWork("w-test", "feat/test")
	h.CreateTask("w-test.1", "w-test", []string{"bead-1"})

	// Complete the task
	h.CompleteTask("w-test.1")

	ctx := context.Background()
	task, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, "completed", task.Status)

	// Create and fail another task
	h.CreateTask("w-test.2", "w-test", []string{"bead-2"})
	h.FailTask("w-test.2", "test failure")

	task2, err := h.DB.GetTask(ctx, "w-test.2")
	require.NoError(t, err)
	assert.Equal(t, "failed", task2.Status)
	assert.Equal(t, "test failure", task2.ErrorMessage)
}

func TestMockGitPushFails(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	testErr := errors.New("push failed")
	h.MockGitPushFails(testErr)

	ctx := context.Background()
	err := h.Git.PushSetUpstream(ctx, "test-branch", "/test/dir")
	assert.Error(t, err)
	assert.Equal(t, testErr, err)
}

func TestMockBranchExists(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	// By default, branch doesn't exist
	ctx := context.Background()
	assert.False(t, h.Git.BranchExists(ctx, "/repo", "feature"))

	// Configure branch to exist
	h.MockBranchExists("feature", true, true)

	assert.True(t, h.Git.BranchExists(ctx, "/repo", "feature"))
	local, remote, err := h.Git.ValidateExistingBranch(ctx, "/repo", "feature")
	require.NoError(t, err)
	assert.True(t, local)
	assert.True(t, remote)
}
