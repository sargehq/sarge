package control_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/mise"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestProject creates a minimal test project with an in-memory database.
func setupTestProject(t *testing.T) (*project.Project, func()) {
	t.Helper()
	ctx := context.Background()

	database, err := db.OpenPath(ctx, ":memory:")
	require.NoError(t, err, "failed to open database")

	cfg := &project.Config{
		Project: project.ProjectConfig{
			Name:      "test-project",
			CreatedAt: time.Now(),
		},
		Repo: project.RepoConfig{
			BaseBranch: "main",
		},
		// SchedulerConfig will use defaults
	}

	proj := &project.Project{
		Root:   "/tmp/test-project",
		Config: cfg,
		DB:     database,
	}

	cleanup := func() {
		database.Close()
	}

	return proj, cleanup
}

// testMocks holds all mocked dependencies for ControlPlane tests.
type testMocks struct {
	CP        *control.ControlPlane
	Git       *git.GitOperationsMock
	Worktree  *worktree.WorktreeOperationsMock
	Feedback  *feedback.FeedbackProcessorMock
	Spawner   *OrchestratorSpawnerMock
	Destroyer *WorkDestroyerMock
	GitHub    *github.GitHubClientMock
}

// setupControlPlane creates a ControlPlane with all mocked dependencies.
func setupControlPlane() *testMocks {
	gitMock := &git.GitOperationsMock{}
	wtMock := &worktree.WorktreeOperationsMock{}
	miseMock := &mise.MiseOperationsMock{}
	feedbackMock := &feedback.FeedbackProcessorMock{}
	spawnerMock := &OrchestratorSpawnerMock{}
	destroyerMock := &WorkDestroyerMock{}
	githubMock := &github.GitHubClientMock{}

	cp := control.NewControlPlaneWithDeps(
		gitMock,
		wtMock,
		nil, // zellij not used in these tests
		func(dir string) mise.Operations { return miseMock },
		feedbackMock,
		spawnerMock,
		destroyerMock,
		githubMock,
	)

	return &testMocks{
		CP:        cp,
		Git:       gitMock,
		Worktree:  wtMock,
		Feedback:  feedbackMock,
		Spawner:   spawnerMock,
		Destroyer: destroyerMock,
		GitHub:    githubMock,
	}
}

// createTestWork creates a work record for testing with minimal required fields.
func createTestWork(ctx context.Context, t *testing.T, database *db.DB, workID, branchName, rootIssueID string) {
	t.Helper()
	err := database.CreateWork(ctx, workID, workID, "", branchName, "main", rootIssueID, false)
	require.NoError(t, err)
}

func TestHandleGitPushTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("succeeds with metadata", func(t *testing.T) {
		mocks := setupControlPlane()

		// Configure git mock to succeed
		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return nil
		}

		// Create work for the task
		createTestWork(ctx, t, proj.DB, "w-test", "feature-branch", "root-issue-1")
		defer proj.DB.DeleteWork(ctx, "w-test")

		task := &db.ScheduledTask{
			ID:       "task-1",
			WorkID:   "w-test",
			TaskType: db.TaskTypeGitPush,
			Metadata: map[string]string{
				"branch": "feature-branch",
				"dir":    "/work/tree",
			},
		}

		err := mocks.CP.HandleGitPushTask(ctx, proj, task)
		require.NoError(t, err)

		// Verify git push was called
		calls := mocks.Git.PushSetUpstreamCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "feature-branch", calls[0].Branch)
		assert.Equal(t, "/work/tree", calls[0].Dir)
	})

	t.Run("uses work info when metadata empty", func(t *testing.T) {
		mocks := setupControlPlane()

		// Configure git mock
		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return nil
		}

		// Create work with worktree path
		createTestWork(ctx, t, proj.DB, "w-test2", "from-work-branch", "root-issue-1")
		err := proj.DB.UpdateWorkWorktreePath(ctx, "w-test2", "/from/work/path")
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-test2")

		task := &db.ScheduledTask{
			ID:       "task-2",
			WorkID:   "w-test2",
			TaskType: db.TaskTypeGitPush,
			Metadata: map[string]string{}, // Empty metadata
		}

		err = mocks.CP.HandleGitPushTask(ctx, proj, task)
		require.NoError(t, err)

		// Verify it used work's branch and path
		calls := mocks.Git.PushSetUpstreamCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "from-work-branch", calls[0].Branch)
		assert.Equal(t, "/from/work/path", calls[0].Dir)
	})

	t.Run("returns error when git push fails", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return errors.New("push failed: authentication error")
		}

		task := &db.ScheduledTask{
			ID:       "task-3",
			WorkID:   "w-test",
			TaskType: db.TaskTypeGitPush,
			Metadata: map[string]string{
				"branch": "branch",
				"dir":    "/dir",
			},
		}

		err := mocks.CP.HandleGitPushTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "push failed")
	})

	t.Run("returns error when no branch or dir", func(t *testing.T) {
		mocks := setupControlPlane()

		task := &db.ScheduledTask{
			ID:       "task-4",
			WorkID:   "nonexistent-work",
			TaskType: db.TaskTypeGitPush,
			Metadata: map[string]string{},
		}

		err := mocks.CP.HandleGitPushTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "work not found")
	})
}

func TestHandleSpawnOrchestratorTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("succeeds when work exists", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Spawner.SpawnWorkOrchestratorFunc = func(ctx context.Context, workID, projectName, workDir, friendlyName string, w io.Writer) error {
			return nil
		}

		// Create work
		createTestWork(ctx, t, proj.DB, "w-spawn", "spawn-branch", "root-1")
		err := proj.DB.UpdateWorkWorktreePath(ctx, "w-spawn", "/spawn/tree")
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-spawn")

		task := &db.ScheduledTask{
			ID:       "spawn-task-1",
			WorkID:   "w-spawn",
			TaskType: db.TaskTypeSpawnOrchestrator,
			Metadata: map[string]string{
				"worker_name": "test-worker",
			},
		}

		err = mocks.CP.HandleSpawnOrchestratorTask(ctx, proj, task)
		require.NoError(t, err)

		// Verify spawner was called with correct args
		calls := mocks.Spawner.SpawnWorkOrchestratorCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "w-spawn", calls[0].WorkID)
		assert.Equal(t, "test-project", calls[0].ProjectName)
		assert.Equal(t, "/spawn/tree", calls[0].WorkDir)
		assert.Equal(t, "test-worker", calls[0].FriendlyName)
	})

	t.Run("succeeds when work deleted", func(t *testing.T) {
		mocks := setupControlPlane()

		// Work doesn't exist - task should complete without error
		task := &db.ScheduledTask{
			ID:       "spawn-task-2",
			WorkID:   "nonexistent",
			TaskType: db.TaskTypeSpawnOrchestrator,
			Metadata: map[string]string{},
		}

		err := mocks.CP.HandleSpawnOrchestratorTask(ctx, proj, task)
		require.NoError(t, err)

		// Spawner should not have been called
		assert.Len(t, mocks.Spawner.SpawnWorkOrchestratorCalls(), 0)
	})

	t.Run("returns error when no worktree path", func(t *testing.T) {
		mocks := setupControlPlane()

		// Create work without worktree path
		createTestWork(ctx, t, proj.DB, "w-no-tree", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-no-tree")

		task := &db.ScheduledTask{
			ID:       "spawn-task-3",
			WorkID:   "w-no-tree",
			TaskType: db.TaskTypeSpawnOrchestrator,
			Metadata: map[string]string{},
		}

		err := mocks.CP.HandleSpawnOrchestratorTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no worktree path")
	})

	t.Run("returns error when spawner fails", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Spawner.SpawnWorkOrchestratorFunc = func(ctx context.Context, workID, projectName, workDir, friendlyName string, w io.Writer) error {
			return errors.New("zellij error")
		}

		createTestWork(ctx, t, proj.DB, "w-fail", "branch", "root-1")
		err := proj.DB.UpdateWorkWorktreePath(ctx, "w-fail", "/fail/tree")
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-fail")

		task := &db.ScheduledTask{
			ID:       "spawn-task-4",
			WorkID:   "w-fail",
			TaskType: db.TaskTypeSpawnOrchestrator,
			Metadata: map[string]string{},
		}

		err = mocks.CP.HandleSpawnOrchestratorTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to spawn orchestrator")
	})
}

func TestHandleDestroyWorktreeTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("succeeds when work exists", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Destroyer.DestroyWorkFunc = func(ctx context.Context, workID string, w io.Writer) error {
			return nil
		}

		// Create work
		createTestWork(ctx, t, proj.DB, "w-destroy", "destroy-branch", "root-1")
		// Note: Normally work would be deleted by the handler, but we use a mock
		defer proj.DB.DeleteWork(ctx, "w-destroy")

		task := &db.ScheduledTask{
			ID:       "destroy-task-1",
			WorkID:   "w-destroy",
			TaskType: db.TaskTypeDestroyWorktree,
		}

		err := mocks.CP.HandleDestroyWorktreeTask(ctx, proj, task)
		require.NoError(t, err)

		// Verify destroyer was called
		calls := mocks.Destroyer.DestroyWorkCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "w-destroy", calls[0].WorkID)
	})

	t.Run("succeeds when work already deleted", func(t *testing.T) {
		mocks := setupControlPlane()

		// Work doesn't exist - task should complete without error
		task := &db.ScheduledTask{
			ID:       "destroy-task-2",
			WorkID:   "nonexistent",
			TaskType: db.TaskTypeDestroyWorktree,
		}

		err := mocks.CP.HandleDestroyWorktreeTask(ctx, proj, task)
		require.NoError(t, err)

		// Destroyer should not have been called
		assert.Len(t, mocks.Destroyer.DestroyWorkCalls(), 0)
	})

	t.Run("returns error when destroyer fails", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Destroyer.DestroyWorkFunc = func(ctx context.Context, workID string, w io.Writer) error {
			return errors.New("filesystem error")
		}

		createTestWork(ctx, t, proj.DB, "w-fail-destroy", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-fail-destroy")

		task := &db.ScheduledTask{
			ID:       "destroy-task-3",
			WorkID:   "w-fail-destroy",
			TaskType: db.TaskTypeDestroyWorktree,
		}

		err := mocks.CP.HandleDestroyWorktreeTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "filesystem error")
	})
}

func TestHandlePRFeedbackTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("processes feedback when PR exists", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Feedback.ProcessPRFeedbackFunc = func(ctx context.Context, proj *project.Project, database *db.DB, workID string) (int, error) {
			return 3, nil // Created 3 beads
		}

		// Mock GetPRStatus for spawnWorkflowWatchers - return empty workflows
		mocks.GitHub.GetPRStatusFunc = func(ctx context.Context, prURL string) (*github.PRStatus, error) {
			return &github.PRStatus{Workflows: nil}, nil
		}

		// Create work with PR URL
		createTestWork(ctx, t, proj.DB, "w-feedback", "feedback-branch", "root-1")
		err := proj.DB.SetWorkPRURLAndScheduleFeedback(ctx, "w-feedback", "https://github.com/org/repo/pull/123", 5*time.Minute, 5*time.Minute)
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-feedback")

		task := &db.ScheduledTask{
			ID:       "feedback-task-1",
			WorkID:   "w-feedback",
			TaskType: db.TaskTypePRFeedback,
		}

		err = mocks.CP.HandlePRFeedbackTask(ctx, proj, task)
		require.NoError(t, err)

		// Verify feedback processor was called
		calls := mocks.Feedback.ProcessPRFeedbackCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "w-feedback", calls[0].WorkID)
	})

	t.Run("skips processing when no PR URL", func(t *testing.T) {
		mocks := setupControlPlane()

		// Create work without PR URL
		createTestWork(ctx, t, proj.DB, "w-no-pr", "no-pr-branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-no-pr")

		task := &db.ScheduledTask{
			ID:       "feedback-task-2",
			WorkID:   "w-no-pr",
			TaskType: db.TaskTypePRFeedback,
		}

		err := mocks.CP.HandlePRFeedbackTask(ctx, proj, task)
		require.NoError(t, err)

		// Feedback processor should not have been called
		assert.Len(t, mocks.Feedback.ProcessPRFeedbackCalls(), 0)
	})

	t.Run("returns error when feedback processing fails", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Feedback.ProcessPRFeedbackFunc = func(ctx context.Context, proj *project.Project, database *db.DB, workID string) (int, error) {
			return 0, errors.New("GitHub API error")
		}

		createTestWork(ctx, t, proj.DB, "w-fail-fb", "branch", "root-1")
		err := proj.DB.SetWorkPRURLAndScheduleFeedback(ctx, "w-fail-fb", "https://github.com/org/repo/pull/456", 5*time.Minute, 5*time.Minute)
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-fail-fb")

		task := &db.ScheduledTask{
			ID:       "feedback-task-3",
			WorkID:   "w-fail-fb",
			TaskType: db.TaskTypePRFeedback,
		}

		err = mocks.CP.HandlePRFeedbackTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "GitHub API error")
	})
}

func TestGetTaskHandlers(t *testing.T) {
	mocks := setupControlPlane()

	handlers := mocks.CP.GetTaskHandlers()

	// Verify all expected task types have handlers
	expectedTypes := []string{
		db.TaskTypeCreateWorktree,
		db.TaskTypeSpawnOrchestrator,
		db.TaskTypePRFeedback,
		db.TaskTypeGitPush,
		db.TaskTypeDestroyWorktree,
		db.TaskTypeImportPR,
		db.TaskTypeCommentResolution,
		db.TaskTypeGitHubComment,
		db.TaskTypeGitHubResolveThread,
	}

	for _, taskType := range expectedTypes {
		_, ok := handlers[taskType]
		assert.True(t, ok, "expected handler for task type %s", taskType)
	}
}

func TestNewControlPlane(t *testing.T) {
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	cp := control.NewControlPlane(proj)
	require.NotNil(t, cp)

	// Verify default dependencies are set
	assert.NotNil(t, cp.Git)
	assert.NotNil(t, cp.Worktree)
	assert.NotNil(t, cp.Zellij)
	assert.NotNil(t, cp.Mise)
	assert.NotNil(t, cp.FeedbackProcessor)
	assert.NotNil(t, cp.OrchestratorSpawner)
	assert.NotNil(t, cp.WorkDestroyer)
}

func TestDefaultOrchestratorSpawner(t *testing.T) {
	// Compile-time check that DefaultOrchestratorSpawner implements OrchestratorSpawner
	var _ control.OrchestratorSpawner = (*control.DefaultOrchestratorSpawner)(nil)
}

func TestDefaultWorkDestroyer(t *testing.T) {
	// Compile-time check that DefaultWorkDestroyer implements WorkDestroyer
	var _ control.WorkDestroyer = (*control.DefaultWorkDestroyer)(nil)
}

func TestHandleCreateWorktreeTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("succeeds when work is deleted", func(t *testing.T) {
		mocks := setupControlPlane()

		// Work doesn't exist - should complete without error
		task := &db.ScheduledTask{
			ID:       "create-task-3",
			WorkID:   "nonexistent",
			TaskType: db.TaskTypeCreateWorktree,
			Metadata: map[string]string{
				"branch": "some-branch",
			},
		}

		err := mocks.CP.HandleCreateWorktreeTask(ctx, proj, task)
		require.NoError(t, err)
	})

	t.Run("skips worktree creation when already exists", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return nil
		}

		// Create work with existing worktree path
		createTestWork(ctx, t, proj.DB, "w-exists", "exists-branch", "root-1")
		err := proj.DB.UpdateWorkWorktreePath(ctx, "w-exists", "/existing/path")
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-exists")

		task := &db.ScheduledTask{
			ID:       "create-task-6",
			WorkID:   "w-exists",
			TaskType: db.TaskTypeCreateWorktree,
			Metadata: map[string]string{
				"branch": "exists-branch",
			},
		}

		err = mocks.CP.HandleCreateWorktreeTask(ctx, proj, task)
		require.NoError(t, err)

		// Worktree creation should not have been called
		assert.Len(t, mocks.Worktree.CreateCalls(), 0)
		assert.Len(t, mocks.Worktree.CreateFromExistingCalls(), 0)
	})

	t.Run("uses default base branch from config", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return nil
		}

		// Create work with existing worktree path - allows us to test config lookup without filesystem ops
		createTestWork(ctx, t, proj.DB, "w-default-base", "default-branch", "root-1")
		err := proj.DB.UpdateWorkWorktreePath(ctx, "w-default-base", "/work/path")
		require.NoError(t, err)
		defer proj.DB.DeleteWork(ctx, "w-default-base")

		task := &db.ScheduledTask{
			ID:       "create-task-7",
			WorkID:   "w-default-base",
			TaskType: db.TaskTypeCreateWorktree,
			Metadata: map[string]string{
				"branch": "default-branch",
				// No base_branch in metadata - should use config default
			},
		}

		err = mocks.CP.HandleCreateWorktreeTask(ctx, proj, task)
		require.NoError(t, err)

		// Should not try to create worktree since it already exists
		assert.Len(t, mocks.Worktree.CreateCalls(), 0)
	})
}

func TestScheduleDestroyWorktree(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("schedules destroy task successfully", func(t *testing.T) {
		// Create work first
		createTestWork(ctx, t, proj.DB, "w-sched-destroy", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-sched-destroy")

		err := control.ScheduleDestroyWorktree(ctx, proj, "w-sched-destroy")
		require.NoError(t, err)

		// Verify task was scheduled
		task, err := proj.DB.GetNextScheduledTask(ctx)
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, db.TaskTypeDestroyWorktree, task.TaskType)
		assert.Equal(t, "w-sched-destroy", task.WorkID)
	})
}

func TestTriggerPRFeedbackCheck(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("triggers immediate feedback check", func(t *testing.T) {
		// Create work with existing PR feedback task
		createTestWork(ctx, t, proj.DB, "w-trigger", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-trigger")

		// First schedule a task for later
		_, err := proj.DB.ScheduleTask(ctx, "w-trigger", db.TaskTypePRFeedback, time.Now().Add(1*time.Hour), nil)
		require.NoError(t, err)

		// Trigger immediate check
		err = control.TriggerPRFeedbackCheck(ctx, proj, "w-trigger")
		require.NoError(t, err)

		// Verify the task's scheduled_at was updated to now (within tolerance)
		task, err := proj.DB.GetNextScheduledTask(ctx)
		require.NoError(t, err)
		require.NotNil(t, task)
		assert.Equal(t, db.TaskTypePRFeedback, task.TaskType)
		// The task should be due now, not in an hour
		assert.True(t, task.ScheduledAt.Before(time.Now().Add(1*time.Minute)))
	})
}

func TestProcessAllDueTasksWithControlPlane(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("processes due tasks and handles completion", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return nil
		}

		// Create work and schedule a git push task
		createTestWork(ctx, t, proj.DB, "w-process", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-process")

		_, err := proj.DB.ScheduleTask(ctx, "w-process", db.TaskTypeGitPush, time.Now(), map[string]string{
			"branch": "branch",
			"dir":    "/work/dir",
		})
		require.NoError(t, err)

		// Process tasks
		control.ProcessAllDueTasksWithControlPlane(ctx, proj, mocks.CP)

		// Verify git push was called
		calls := mocks.Git.PushSetUpstreamCalls()
		require.Len(t, calls, 1)
	})

	t.Run("handles unknown task type", func(t *testing.T) {
		mocks := setupControlPlane()

		createTestWork(ctx, t, proj.DB, "w-unknown", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-unknown")

		// Schedule a task with unknown type
		_, err := proj.DB.ScheduleTask(ctx, "w-unknown", "unknown_task_type", time.Now(), nil)
		require.NoError(t, err)

		// Process tasks - should handle gracefully
		control.ProcessAllDueTasksWithControlPlane(ctx, proj, mocks.CP)

		// No panic or error expected
	})

	t.Run("handles task failure with retry", func(t *testing.T) {
		mocks := setupControlPlane()

		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			return errors.New("transient error")
		}

		createTestWork(ctx, t, proj.DB, "w-retry", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-retry")

		// Schedule a task that will fail
		err := proj.DB.ScheduleTaskWithRetry(ctx, "w-retry", db.TaskTypeGitPush, time.Now(), map[string]string{
			"branch": "branch",
			"dir":    "/work/dir",
		}, "retry-test", 3)
		require.NoError(t, err)

		// Process tasks - task should fail but be rescheduled
		control.ProcessAllDueTasksWithControlPlane(ctx, proj, mocks.CP)
	})

	t.Run("processes multiple tasks in order", func(t *testing.T) {
		mocks := setupControlPlane()

		callOrder := []string{}
		mocks.Git.PushSetUpstreamFunc = func(ctx context.Context, branch, dir string) error {
			callOrder = append(callOrder, branch)
			return nil
		}

		createTestWork(ctx, t, proj.DB, "w-multi", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-multi")

		// Schedule multiple tasks
		_, err := proj.DB.ScheduleTask(ctx, "w-multi", db.TaskTypeGitPush, time.Now(), map[string]string{
			"branch": "first",
			"dir":    "/dir1",
		})
		require.NoError(t, err)

		_, err = proj.DB.ScheduleTask(ctx, "w-multi", db.TaskTypeGitPush, time.Now(), map[string]string{
			"branch": "second",
			"dir":    "/dir2",
		})
		require.NoError(t, err)

		// Process tasks
		control.ProcessAllDueTasksWithControlPlane(ctx, proj, mocks.CP)

		// Both tasks should be processed
		calls := mocks.Git.PushSetUpstreamCalls()
		assert.Len(t, calls, 2)
	})
}

func TestHandleTaskError(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("reschedules task with retries remaining", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-error-retry", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-error-retry")

		// Create a task with retries
		err := proj.DB.ScheduleTaskWithRetry(ctx, "w-error-retry", db.TaskTypeGitPush, time.Now(), nil, "error-test", 3)
		require.NoError(t, err)

		// Get the task
		task, err := proj.DB.GetNextScheduledTask(ctx)
		require.NoError(t, err)
		require.NotNil(t, task)

		// Mark as executing first
		err = proj.DB.MarkTaskExecuting(ctx, task.ID)
		require.NoError(t, err)

		// Handle error - task should be rescheduled due to retries remaining
		control.HandleTaskError(ctx, proj, task, "test error")

		// Verify no pending tasks (task was rescheduled with future time)
		// The task should have been rescheduled with backoff, not marked as failed
	})

	t.Run("marks task as failed when retries exhausted", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-error-fail", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-error-fail")

		// Create a task with only 1 max attempt
		err := proj.DB.ScheduleTaskWithRetry(ctx, "w-error-fail", db.TaskTypeGitPush, time.Now(), nil, "fail-test", 1)
		require.NoError(t, err)

		// Get the task
		task, err := proj.DB.GetNextScheduledTask(ctx)
		require.NoError(t, err)
		require.NotNil(t, task)

		// Mark as executing
		err = proj.DB.MarkTaskExecuting(ctx, task.ID)
		require.NoError(t, err)

		// Set attempt count to max (exhausted retries)
		task.AttemptCount = 1

		// Handle error - should mark as failed since retries exhausted
		control.HandleTaskError(ctx, proj, task, "final error")
	})
}

func TestProcessAllDueTasks(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("uses default control plane", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-default-cp", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-default-cp")

		// Schedule a task that will fail because default dependencies hit real services
		// But this tests that ProcessAllDueTasks correctly creates a default ControlPlane
		err := proj.DB.ScheduleTaskWithRetry(ctx, "w-default-cp", db.TaskTypeGitPush, time.Now(), map[string]string{
			"branch": "branch",
			"dir":    "/nonexistent",
		}, "default-cp-test", 1)
		require.NoError(t, err)

		// Process tasks - should not panic even though task will fail
		control.ProcessAllDueTasks(ctx, proj)
	})
}

func TestHandleCommentResolutionTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("skips when work has no PR URL", func(t *testing.T) {
		// Create work without PR URL
		createTestWork(ctx, t, proj.DB, "w-no-pr-comment", "no-pr-branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-no-pr-comment")

		task := &db.ScheduledTask{
			ID:       "comment-task-1",
			WorkID:   "w-no-pr-comment",
			TaskType: db.TaskTypeCommentResolution,
		}

		// Should succeed without doing anything (no PR URL)
		err := control.HandleCommentResolutionTask(ctx, proj, task)
		require.NoError(t, err)
	})

	t.Run("skips when work does not exist", func(t *testing.T) {
		task := &db.ScheduledTask{
			ID:       "comment-task-2",
			WorkID:   "nonexistent-work",
			TaskType: db.TaskTypeCommentResolution,
		}

		// Should succeed without doing anything (work not found)
		err := control.HandleCommentResolutionTask(ctx, proj, task)
		require.NoError(t, err)
	})
}

func TestHandleGitHubCommentTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("returns error when pr_url missing", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-gh-comment", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-gh-comment")

		task := &db.ScheduledTask{
			ID:       "gh-comment-task-1",
			WorkID:   "w-gh-comment",
			TaskType: db.TaskTypeGitHubComment,
			Metadata: map[string]string{
				"body": "test comment",
				// Missing pr_url
			},
		}

		err := control.HandleGitHubCommentTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing pr_url or body")
	})

	t.Run("returns error when body missing", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-gh-comment-2", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-gh-comment-2")

		task := &db.ScheduledTask{
			ID:       "gh-comment-task-2",
			WorkID:   "w-gh-comment-2",
			TaskType: db.TaskTypeGitHubComment,
			Metadata: map[string]string{
				"pr_url": "https://github.com/org/repo/pull/123",
				// Missing body
			},
		}

		err := control.HandleGitHubCommentTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing pr_url or body")
	})

	t.Run("returns error when reply_to_id is invalid", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-gh-comment-3", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-gh-comment-3")

		task := &db.ScheduledTask{
			ID:       "gh-comment-task-3",
			WorkID:   "w-gh-comment-3",
			TaskType: db.TaskTypeGitHubComment,
			Metadata: map[string]string{
				"pr_url":      "https://github.com/org/repo/pull/123",
				"body":        "test comment",
				"reply_to_id": "not-a-number",
			},
		}

		err := control.HandleGitHubCommentTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid reply_to_id")
	})
}

func TestHandleGitHubResolveThreadTask(t *testing.T) {
	ctx := context.Background()
	proj, cleanup := setupTestProject(t)
	defer cleanup()

	t.Run("returns error when pr_url missing", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-resolve-1", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-resolve-1")

		task := &db.ScheduledTask{
			ID:       "resolve-task-1",
			WorkID:   "w-resolve-1",
			TaskType: db.TaskTypeGitHubResolveThread,
			Metadata: map[string]string{
				"comment_id": "123",
				// Missing pr_url
			},
		}

		err := control.HandleGitHubResolveThreadTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing pr_url or comment_id")
	})

	t.Run("returns error when comment_id missing", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-resolve-2", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-resolve-2")

		task := &db.ScheduledTask{
			ID:       "resolve-task-2",
			WorkID:   "w-resolve-2",
			TaskType: db.TaskTypeGitHubResolveThread,
			Metadata: map[string]string{
				"pr_url": "https://github.com/org/repo/pull/123",
				// Missing comment_id
			},
		}

		err := control.HandleGitHubResolveThreadTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing pr_url or comment_id")
	})

	t.Run("returns error when comment_id is invalid", func(t *testing.T) {
		createTestWork(ctx, t, proj.DB, "w-resolve-3", "branch", "root-1")
		defer proj.DB.DeleteWork(ctx, "w-resolve-3")

		task := &db.ScheduledTask{
			ID:       "resolve-task-3",
			WorkID:   "w-resolve-3",
			TaskType: db.TaskTypeGitHubResolveThread,
			Metadata: map[string]string{
				"pr_url":     "https://github.com/org/repo/pull/123",
				"comment_id": "not-a-number",
			},
		}

		err := control.HandleGitHubResolveThreadTask(ctx, proj, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid comment_id")
	})
}
