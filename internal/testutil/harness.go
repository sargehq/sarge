// Package testutil provides testing utilities for the sarge orchestrator.
package testutil

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/names"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/task"
	"github.com/sargehq/sarge/internal/work"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/stretchr/testify/require"
)

// TestHarness provides a complete test environment with pre-wired mocks
// for integration testing work flows without external dependencies.
type TestHarness struct {
	T                   *testing.T
	DB                  *db.DB
	Git                 *git.GitOperationsMock
	Worktree            *worktree.WorktreeOperationsMock
	Beads               *beads.BeadsCLIMock
	BeadsReader         *beads.BeadsReaderMock
	OrchestratorManager *work.OrchestratorManagerMock
	NameGenerator       *names.GeneratorMock
	TaskPlanner         *task.PlannerMock
	WorkService         *work.WorkService
	Config              *project.Config

	// Internal state for fixtures
	beadStore map[string]*beads.Bead
	beadDeps  map[string][]beads.Dependency
	workCount int
	beadCount int
}

// NewTestHarness creates a new TestHarness with an in-memory database
// and all mocks pre-configured with sensible defaults.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()

	// Create in-memory SQLite database
	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err, "failed to open in-memory database")

	// Create mocks with default no-op/success behavior
	gitMock := &git.GitOperationsMock{}
	worktreeMock := &worktree.WorktreeOperationsMock{}
	beadsMock := &beads.BeadsCLIMock{}
	beadsReaderMock := &beads.BeadsReaderMock{}
	orchestratorMock := &work.OrchestratorManagerMock{}
	nameGenMock := &names.GeneratorMock{}
	taskPlannerMock := &task.PlannerMock{}

	// Create test config
	config := &project.Config{
		Project: project.ProjectConfig{
			Name: "test-project",
		},
		Repo: project.RepoConfig{
			Type:       "local",
			Source:     "/test/repo",
			Path:       "main",
			BaseBranch: "main",
		},
		Beads: project.BeadsConfig{
			Path: "main/.beads",
		},
	}

	h := &TestHarness{
		T:                   t,
		DB:                  testDB,
		Git:                 gitMock,
		Worktree:            worktreeMock,
		Beads:               beadsMock,
		BeadsReader:         beadsReaderMock,
		OrchestratorManager: orchestratorMock,
		NameGenerator:       nameGenMock,
		TaskPlanner:         taskPlannerMock,
		Config:              config,
		beadStore:           make(map[string]*beads.Bead),
		beadDeps:            make(map[string][]beads.Dependency),
	}

	// Wire up WorkService with all mocked dependencies
	h.WorkService = work.NewWorkServiceWithDeps(work.WorkServiceDeps{
		DB:                  testDB,
		Git:                 gitMock,
		Worktree:            worktreeMock,
		BeadsReader:         beadsReaderMock,
		BeadsCLI:            beadsMock,
		OrchestratorManager: orchestratorMock,
		TaskPlanner:         taskPlannerMock,
		NameGenerator:       nameGenMock,
		Config:              config,
		ProjectRoot:         "/test/project",
		MainRepoPath:        "/test/project/main",
	})

	// Configure default mock behaviors
	h.configureDefaultMocks()

	return h
}

// Cleanup releases resources used by the harness.
// Should be called with defer after NewTestHarness.
func (h *TestHarness) Cleanup() {
	if h.DB != nil {
		if err := h.DB.Close(); err != nil {
			h.T.Logf("warning: failed to close database: %v", err)
		}
	}
}

// configureDefaultMocks sets up sensible default behaviors for all mocks.
// Tests can override specific behaviors as needed.
func (h *TestHarness) configureDefaultMocks() {
	// Git defaults: branch doesn't exist, operations succeed
	h.Git.BranchExistsFunc = func(ctx context.Context, repoPath string, branchName string) bool {
		return false
	}
	h.Git.ValidateExistingBranchFunc = func(ctx context.Context, repoPath string, branchName string) (bool, bool, error) {
		return false, false, nil // not local, not remote, no error
	}
	h.Git.PushSetUpstreamFunc = func(ctx context.Context, branch string, dir string) error {
		return nil
	}

	// Worktree defaults: worktree doesn't exist, creation succeeds
	h.Worktree.ExistsPathFunc = func(worktreePath string) bool {
		return false
	}
	h.Worktree.CreateFunc = func(ctx context.Context, repoPath string, worktreePath string, branch string, baseBranch string) error {
		return nil
	}

	// Beads CLI defaults: operations succeed
	h.Beads.CloseFunc = func(ctx context.Context, beadID string) error {
		return nil
	}
	h.Beads.UpdateFunc = func(ctx context.Context, beadID string, opts beads.UpdateOptions) error {
		return nil
	}

	// BeadsReader defaults: delegate to internal store
	h.BeadsReader.GetBeadFunc = func(ctx context.Context, id string) (*beads.BeadWithDeps, error) {
		return h.getBeadWithDeps(id), nil
	}
	h.BeadsReader.GetBeadsWithDepsFunc = func(ctx context.Context, beadIDs []string) (*beads.BeadsWithDepsResult, error) {
		return h.getBeadsWithDepsResult(beadIDs), nil
	}
	h.BeadsReader.GetBeadWithChildrenFunc = func(ctx context.Context, id string) ([]beads.Bead, error) {
		return h.getBeadWithChildren(id), nil
	}
	h.BeadsReader.GetTransitiveDependenciesFunc = func(ctx context.Context, id string) ([]beads.Bead, error) {
		return h.getTransitiveDependencies(id), nil
	}

	// Orchestrator defaults: spawning succeeds
	h.OrchestratorManager.EnsureWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
		return true, nil
	}
	h.OrchestratorManager.SpawnWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) error {
		return nil
	}

	// Name generator: return sequential work IDs
	h.NameGenerator.GetNextAvailableNameFunc = func(ctx context.Context, db names.DB) (string, error) {
		h.workCount++
		return "w-test" + string(rune('a'+h.workCount-1)), nil
	}
}

// getBeadWithDeps returns a BeadWithDeps from the internal store.
func (h *TestHarness) getBeadWithDeps(id string) *beads.BeadWithDeps {
	bead, ok := h.beadStore[id]
	if !ok {
		return nil
	}
	return &beads.BeadWithDeps{
		Bead:         bead,
		Dependencies: h.beadDeps[id],
		Dependents:   h.getDependents(id),
	}
}

// getBeadsWithDepsResult builds a BeadsWithDepsResult from the internal store.
func (h *TestHarness) getBeadsWithDepsResult(beadIDs []string) *beads.BeadsWithDepsResult {
	result := &beads.BeadsWithDepsResult{
		Beads:        make(map[string]beads.Bead),
		Dependencies: make(map[string][]beads.Dependency),
		Dependents:   make(map[string][]beads.Dependent),
	}
	for _, id := range beadIDs {
		if bead, ok := h.beadStore[id]; ok {
			result.Beads[id] = *bead
			result.Dependencies[id] = h.beadDeps[id]
			result.Dependents[id] = h.getDependents(id)
		}
	}
	return result
}

// getDependents returns beads that depend on the given bead.
func (h *TestHarness) getDependents(id string) []beads.Dependent {
	var dependents []beads.Dependent
	for beadID, deps := range h.beadDeps {
		for _, dep := range deps {
			if dep.DependsOnID == id {
				bead := h.beadStore[beadID]
				dependents = append(dependents, beads.Dependent{
					IssueID:     beadID,
					DependsOnID: id,
					Type:        dep.Type,
					Status:      bead.Status,
					Title:       bead.Title,
				})
			}
		}
	}
	return dependents
}

// getBeadWithChildren returns a bead and all its children (for epics).
func (h *TestHarness) getBeadWithChildren(id string) []beads.Bead {
	var result []beads.Bead
	if bead, ok := h.beadStore[id]; ok {
		result = append(result, *bead)
	}

	// Find children (beads with parent-child dependency to this bead)
	for beadID, deps := range h.beadDeps {
		for _, dep := range deps {
			if dep.DependsOnID == id && dep.Type == "parent-child" {
				if child, ok := h.beadStore[beadID]; ok {
					result = append(result, *child)
				}
			}
		}
	}
	return result
}

// getTransitiveDependencies returns all transitive dependencies for a bead.
func (h *TestHarness) getTransitiveDependencies(id string) []beads.Bead {
	visited := make(map[string]bool)
	var result []beads.Bead

	var collect func(beadID string)
	collect = func(beadID string) {
		if visited[beadID] {
			return
		}
		visited[beadID] = true

		// First collect dependencies
		for _, dep := range h.beadDeps[beadID] {
			if dep.Type == "blocked_by" || dep.Type == "blocks" {
				collect(dep.DependsOnID)
			}
		}

		// Then add this bead
		if bead, ok := h.beadStore[beadID]; ok {
			result = append(result, *bead)
		}
	}

	collect(id)
	return result
}

// =============================================================================
// Bead Fixtures
// =============================================================================

// CreateBead creates a test bead and stores it in the harness.
// The bead is created with status "open" and type "task" by default.
func (h *TestHarness) CreateBead(id, title string) *beads.Bead {
	h.beadCount++
	bead := &beads.Bead{
		ID:       id,
		Title:    title,
		Status:   beads.StatusOpen,
		Type:     "task",
		Priority: 2, // medium priority
	}
	h.beadStore[id] = bead
	return bead
}

// CreateEpicWithChildren creates an epic bead with child beads.
// The epic is created with the given ID, and children are created with
// the parent-child dependency relationship.
func (h *TestHarness) CreateEpicWithChildren(epicID string, childIDs ...string) *beads.Bead {
	// Create the epic
	epic := &beads.Bead{
		ID:       epicID,
		Title:    "Epic: " + epicID,
		Status:   beads.StatusOpen,
		Type:     "epic",
		IsEpic:   true,
		Priority: 1,
	}
	h.beadStore[epicID] = epic

	// Create children and set up parent-child relationships
	for _, childID := range childIDs {
		child := h.CreateBead(childID, "Task: "+childID)
		// Add parent-child dependency (child depends on parent)
		h.beadDeps[childID] = append(h.beadDeps[childID], beads.Dependency{
			IssueID:     childID,
			DependsOnID: epicID,
			Type:        "parent-child",
			Status:      epic.Status,
			Title:       epic.Title,
		})
		_ = child // silence unused variable warning
	}

	return epic
}

// SetBeadDependency creates a blocking dependency between two beads.
// The bead identified by beadID will be blocked by dependsOnID.
func (h *TestHarness) SetBeadDependency(beadID, dependsOnID string) {
	depBead := h.beadStore[dependsOnID]
	var status, title string
	if depBead != nil {
		status = depBead.Status
		title = depBead.Title
	}

	h.beadDeps[beadID] = append(h.beadDeps[beadID], beads.Dependency{
		IssueID:     beadID,
		DependsOnID: dependsOnID,
		Type:        "blocks",
		Status:      status,
		Title:       title,
	})
}

// =============================================================================
// Work Fixtures
// =============================================================================

// CreateWork creates a work record in the database with the given ID and branch.
// Returns the created work.
func (h *TestHarness) CreateWork(workID, branch string) *db.Work {
	return h.CreateWorkWithRootIssue(workID, branch, "")
}

// CreateWorkWithRootIssue creates a work record with an optional root issue ID.
// Returns the created work.
func (h *TestHarness) CreateWorkWithRootIssue(workID, branch, rootIssueID string) *db.Work {
	h.T.Helper()
	ctx := context.Background()

	err := h.DB.CreateWork(ctx, workID, "Test Work: "+workID,
		"/test/project/"+workID+"/tree", branch, "main", rootIssueID, false)
	require.NoError(h.T, err, "failed to create work")

	work, err := h.DB.GetWork(ctx, workID)
	require.NoError(h.T, err, "failed to get created work")
	return work
}

// AddBeadToWork associates a bead with a work in the database.
func (h *TestHarness) AddBeadToWork(workID, beadID string) {
	h.T.Helper()
	ctx := context.Background()

	err := h.DB.AddBeadToWork(ctx, workID, beadID)
	require.NoError(h.T, err, "failed to add bead to work")
}

// =============================================================================
// Task Fixtures
// =============================================================================

// CreateTask creates a task in the database with the given beads.
// Returns the created task.
func (h *TestHarness) CreateTask(taskID, workID string, beadIDs []string) *db.Task {
	h.T.Helper()
	ctx := context.Background()

	err := h.DB.CreateTask(ctx, taskID, "implement", beadIDs, 10, workID, extractTaskNumber(taskID))
	require.NoError(h.T, err, "failed to create task")

	task, err := h.DB.GetTask(ctx, taskID)
	require.NoError(h.T, err, "failed to get created task")
	return task
}

// CompleteTask marks a task as completed in the database.
func (h *TestHarness) CompleteTask(taskID string) {
	h.T.Helper()
	ctx := context.Background()

	err := h.DB.CompleteTask(ctx, taskID, "")
	require.NoError(h.T, err, "failed to complete task")
}

// FailTask marks a task as failed with an error message in the database.
func (h *TestHarness) FailTask(taskID, errorMsg string) {
	h.T.Helper()
	ctx := context.Background()

	err := h.DB.FailTask(ctx, taskID, errorMsg)
	require.NoError(h.T, err, "failed to fail task")
}

// =============================================================================
// Mock Behavior Configuration
// =============================================================================

// MockGitPushFails configures the Git mock to return an error on push.
func (h *TestHarness) MockGitPushFails(err error) {
	h.Git.PushSetUpstreamFunc = func(ctx context.Context, branch string, dir string) error {
		return err
	}
}

// MockGitPushSucceeds configures the Git mock to succeed on push.
func (h *TestHarness) MockGitPushSucceeds() {
	h.Git.PushSetUpstreamFunc = func(ctx context.Context, branch string, dir string) error {
		return nil
	}
}

// MockClaudeCompletesSuccessfully configures the orchestrator mock to
// indicate successful completion.
func (h *TestHarness) MockClaudeCompletesSuccessfully() {
	h.OrchestratorManager.SpawnWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) error {
		return nil
	}
	h.OrchestratorManager.EnsureWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) (bool, error) {
		return true, nil
	}
}

// MockClaudeFails configures the orchestrator mock to return an error.
func (h *TestHarness) MockClaudeFails(err error) {
	h.OrchestratorManager.SpawnWorkOrchestratorFunc = func(ctx context.Context, workID string, projName string, workDir string, friendlyName string, w io.Writer) error {
		return err
	}
}

// MockWorktreeCreationFails configures the worktree mock to return an error on creation.
func (h *TestHarness) MockWorktreeCreationFails(err error) {
	h.Worktree.CreateFunc = func(ctx context.Context, repoPath string, worktreePath string, branch string, baseBranch string) error {
		return err
	}
}

// MockBranchExists configures the Git mock to indicate a branch exists.
func (h *TestHarness) MockBranchExists(branchName string, local, remote bool) {
	h.Git.BranchExistsFunc = func(ctx context.Context, repoPath string, name string) bool {
		return name == branchName && local
	}
	h.Git.ValidateExistingBranchFunc = func(ctx context.Context, repoPath string, name string) (bool, bool, error) {
		if name == branchName {
			return local, remote, nil
		}
		return false, false, nil
	}
}

// =============================================================================
// Review Loop Fixtures
// =============================================================================

// CreateReviewTask creates a review task in the database using GetNextTaskNumber.
// Review tasks have no beads associated with them directly.
// If taskID is empty, generates a new task ID using the atomic counter.
// Returns the created task with its actual ID.
func (h *TestHarness) CreateReviewTask(taskID, workID string) *db.Task {
	h.T.Helper()
	ctx := context.Background()

	// If no taskID provided, generate one using the atomic counter
	if taskID == "" {
		taskNum, err := h.DB.GetNextTaskNumber(ctx, workID)
		require.NoError(h.T, err, "failed to get next task number")
		taskID = workID + "." + itoaHarness(taskNum)
	} else {
		// If explicit taskID provided, we still need to advance the counter
		// to avoid conflicts with later GetNextTaskNumber calls
		_, _ = h.DB.GetNextTaskNumber(ctx, workID)
	}

	err := h.DB.CreateTask(ctx, taskID, "review", nil, 0, workID, extractTaskNumber(taskID))
	require.NoError(h.T, err, "failed to create review task")

	task, err := h.DB.GetTask(ctx, taskID)
	require.NoError(h.T, err, "failed to get created review task")
	return task
}

// itoaHarness converts an int to a string for task IDs
func itoaHarness(n int) string {
	return strconv.Itoa(n)
}

// extractTaskNumber extracts the numeric suffix from a task ID like "work-1.14" -> 14.
// Returns 0 if the ID doesn't have a numeric suffix.
func extractTaskNumber(id string) int {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		if n, err := strconv.Atoi(id[idx+1:]); err == nil {
			return n
		}
	}
	return 0
}

// AddReviewIssues adds beads that simulate issues created by a review task.
// These beads are added as children of the specified parent bead.
func (h *TestHarness) AddReviewIssues(parentID string, issues []beads.Bead) {
	for _, issue := range issues {
		// Add the issue to the bead store
		h.beadStore[issue.ID] = &beads.Bead{
			ID:          issue.ID,
			Title:       issue.Title,
			Status:      issue.Status,
			Type:        "task",
			Priority:    2,
			ExternalRef: issue.ExternalRef,
		}

		// Add parent-child dependency
		h.beadDeps[issue.ID] = append(h.beadDeps[issue.ID], beads.Dependency{
			IssueID:     issue.ID,
			DependsOnID: parentID,
			Type:        "parent-child",
			Status:      h.beadStore[parentID].Status,
			Title:       h.beadStore[parentID].Title,
		})
	}
}

// SimulateReviewCompletion simulates completing a review task and checking for issues.
// Returns true if there are beads to fix (issues created by the review).
func (h *TestHarness) SimulateReviewCompletion(reviewTaskID, workID string, reviewIssues []beads.Bead) bool {
	h.T.Helper()
	ctx := context.Background()

	// Complete the review task
	err := h.DB.CompleteTask(ctx, reviewTaskID, "")
	require.NoError(h.T, err, "failed to complete review task")

	// Check if there are issues to fix
	// In the real code, this filters beads by ExternalRef matching "review-{taskID}"
	expectedExternalRef := "review-" + reviewTaskID
	var beadsToFix []beads.Bead

	for _, issue := range reviewIssues {
		if issue.ExternalRef == expectedExternalRef && beads.IsWorkableStatus(issue.Status) {
			beadsToFix = append(beadsToFix, issue)
		}
	}

	return len(beadsToFix) > 0
}

// CountReviewIterations counts the number of completed review tasks for a work.
func (h *TestHarness) CountReviewIterations(workID string) int {
	h.T.Helper()
	ctx := context.Background()

	tasks, err := h.DB.GetWorkTasks(ctx, workID)
	require.NoError(h.T, err, "failed to get work tasks")

	count := 0
	for _, task := range tasks {
		if task.TaskType == "review" && task.Status == db.StatusCompleted {
			count++
		}
	}
	return count
}
