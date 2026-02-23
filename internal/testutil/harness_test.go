package testutil

import (
	"context"
	"errors"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
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
	assert.NotNil(t, h.Beans)
	assert.NotNil(t, h.BeansReader)
	assert.NotNil(t, h.OrchestratorManager)
	assert.NotNil(t, h.NameGenerator)
	assert.NotNil(t, h.TaskPlanner)
	assert.NotNil(t, h.WorkService)
	assert.NotNil(t, h.Config)

	// Verify config defaults
	assert.Equal(t, "test-project", h.Config.Project.Name)
	assert.Equal(t, "main", h.Config.Repo.GetBaseBranch())
}

func TestCreateBean(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	bean := h.CreateBean("bean-1", "Test Bean")

	assert.Equal(t, "bean-1", bean.ID)
	assert.Equal(t, "Test Bean", bean.Title)
	assert.Equal(t, beans.StatusTodo, bean.Status)
	assert.Equal(t, "task", bean.Type)

	// Verify it's accessible through BeansReader
	ctx := context.Background()
	retrieved, err := h.BeansReader.GetBean(ctx, "bean-1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.Equal(t, "bean-1", retrieved.ID)
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
	children, err := h.BeansReader.GetBeanWithChildren(ctx, "epic-1")
	require.NoError(t, err)
	assert.Len(t, children, 3) // epic + 2 children
}

func TestSetBeanDependency(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	h.CreateBean("bean-1", "First")
	h.CreateBean("bean-2", "Second")
	h.SetBeanDependency("bean-2", "bean-1") // bean-2 blocked by bean-1

	ctx := context.Background()
	bean2, err := h.BeansReader.GetBean(ctx, "bean-2")
	require.NoError(t, err)
	require.Len(t, bean2.Dependencies, 1)
	assert.Equal(t, "bean-1", bean2.Dependencies[0].BlockedByID)
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

	// Create task with beans
	task := h.CreateTask("w-test.1", "w-test", []string{"bean-1", "bean-2"})

	assert.Equal(t, "w-test.1", task.ID)
	assert.Equal(t, "w-test", task.WorkID)
	assert.Equal(t, "pending", task.Status)

	// Verify beans are associated
	ctx := context.Background()
	beanIDs, err := h.DB.GetTaskBeans(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Len(t, beanIDs, 2)
}

func TestCompleteAndFailTask(t *testing.T) {
	h := NewTestHarness(t)
	defer h.Cleanup()

	h.CreateWork("w-test", "feat/test")
	h.CreateTask("w-test.1", "w-test", []string{"bean-1"})

	// Complete the task
	h.CompleteTask("w-test.1")

	ctx := context.Background()
	task, err := h.DB.GetTask(ctx, "w-test.1")
	require.NoError(t, err)
	assert.Equal(t, "completed", task.Status)

	// Create and fail another task
	h.CreateTask("w-test.2", "w-test", []string{"bean-2"})
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
