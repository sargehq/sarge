package taskseq

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/bridge"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a minimal in-memory test database.
func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err, "failed to open in-memory database")
	t.Cleanup(func() { database.Close() })
	return database
}

// newTestProject creates a minimal Project with an in-memory DB for testing.
func newTestProject(t *testing.T) *project.Project {
	t.Helper()
	database := setupTestDB(t)
	return &project.Project{
		Root: t.TempDir(),
		Config: &project.Config{
			Beans: project.BeansConfig{Path: ".beans"},
			Repo:  project.RepoConfig{BaseBranch: "main"},
		},
		DB: database,
	}
}

// newTestSequencer creates a sequencer with a test project and bridge.
func newTestSequencer(t *testing.T) (*Sequencer, *project.Project) {
	t.Helper()
	proj := newTestProject(t)
	b := bridge.NewBridge()
	t.Cleanup(func() { b.KillAll() })
	s := New(proj, b)
	return s, proj
}

// createTestWork creates a work record for testing.
func createTestWork(ctx context.Context, t *testing.T, database *db.DB, id, branch string, auto bool) {
	t.Helper()
	err := database.CreateWork(ctx, id, id, "/tmp/"+id, branch, "main", "root-1", auto)
	require.NoError(t, err)
}

// --- New / Notify / ActiveTasks tests ---

func TestNew(t *testing.T) {
	s, _ := newTestSequencer(t)
	require.NotNil(t, s)
	assert.NotNil(t, s.activeTasks)
	assert.NotNil(t, s.done)
	assert.NotNil(t, s.notify)
}

func TestNotify_NonBlocking(t *testing.T) {
	s, _ := newTestSequencer(t)

	// Multiple Notify calls should not block
	for i := 0; i < 100; i++ {
		s.Notify()
	}
	// Channel is buffered with size 1, should have exactly 1 item
	assert.Len(t, s.notify, 1)
}

func TestActiveTasks_Empty(t *testing.T) {
	s, _ := newTestSequencer(t)
	tasks := s.ActiveTasks()
	assert.Empty(t, tasks)
}

func TestActiveTasks_ReturnsSnapshot(t *testing.T) {
	s, _ := newTestSequencer(t)

	// Manually inject active tasks
	s.mu.Lock()
	s.activeTasks["w-1"] = "w-1.1"
	s.activeTasks["w-2"] = "w-2.3"
	s.mu.Unlock()

	tasks := s.ActiveTasks()
	assert.Len(t, tasks, 2)
	assert.Equal(t, "w-1.1", tasks["w-1"])
	assert.Equal(t, "w-2.3", tasks["w-2"])

	// Verify it's a copy, not a reference
	tasks["w-3"] = "w-3.1"
	assert.Len(t, s.ActiveTasks(), 2)
}

// --- poll tests ---

func TestPoll_SkipsCompletedWorks(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	// Create a completed work
	createTestWork(ctx, t, proj.DB, "w-done", "branch-done", false)
	err := proj.DB.CompleteWork(ctx, "w-done", "")
	require.NoError(t, err)

	// Create a pending task for it (should not be picked up)
	err = proj.DB.CreateTask(ctx, "w-done.1", "implement", []string{"b1"}, 10, "w-done", 1)
	require.NoError(t, err)

	s.poll(ctx)

	// No active tasks should be started
	assert.Empty(t, s.ActiveTasks())
}

func TestPoll_SkipsWorksWithActiveTasks(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-active", "branch-active", false)

	// Simulate an active task already running
	s.mu.Lock()
	s.activeTasks["w-active"] = "w-active.1"
	s.mu.Unlock()

	s.poll(ctx)

	// Still only the one manually set active task
	tasks := s.ActiveTasks()
	assert.Len(t, tasks, 1)
	assert.Equal(t, "w-active.1", tasks["w-active"])
}

func TestPoll_NoReadyTasks_ChecksWorkStatus(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-nort", "branch-nort", false)

	// Create and complete a task so checkWorkStatus has something to transition
	err := proj.DB.CreateTask(ctx, "w-nort.1", "implement", []string{"b1"}, 10, "w-nort", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-nort.1", "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, "w-nort.1", "")
	require.NoError(t, err)

	// Poll should trigger checkWorkStatus since there are no ready tasks
	s.poll(ctx)

	w, err := proj.DB.GetWork(ctx, "w-nort")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, w.Status)
}

// --- checkWorkStatus tests ---

func TestCheckWorkStatus_AllCompleted_TransitionsToIdle(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-idle", "branch-idle", false)

	// Create and complete two tasks
	for i, id := range []string{"w-idle.1", "w-idle.2"} {
		err := proj.DB.CreateTask(ctx, id, "implement", []string{"b" + string(rune('1'+i))}, 10, "w-idle", i+1)
		require.NoError(t, err)
		err = proj.DB.StartTask(ctx, id, "/tmp")
		require.NoError(t, err)
		err = proj.DB.CompleteTask(ctx, id, "")
		require.NoError(t, err)
	}

	w, err := proj.DB.GetWork(ctx, "w-idle")
	require.NoError(t, err)

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-idle")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, w.Status)
}

func TestCheckWorkStatus_WithFailures_TransitionsToFailed(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-fail", "branch-fail", false)

	// One completed, one failed
	err := proj.DB.CreateTask(ctx, "w-fail.1", "implement", []string{"b1"}, 10, "w-fail", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-fail.1", "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, "w-fail.1", "")
	require.NoError(t, err)

	err = proj.DB.CreateTask(ctx, "w-fail.2", "implement", []string{"b2"}, 10, "w-fail", 2)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-fail.2", "/tmp")
	require.NoError(t, err)
	err = proj.DB.FailTask(ctx, "w-fail.2", "some error")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-fail")
	require.NoError(t, err)

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-fail")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, w.Status)
}

func TestCheckWorkStatus_WithPendingTasks_NoTransition(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-pend", "branch-pend", false)

	err := proj.DB.CreateTask(ctx, "w-pend.1", "implement", []string{"b1"}, 10, "w-pend", 1)
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-pend")
	require.NoError(t, err)
	origStatus := w.Status

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-pend")
	require.NoError(t, err)
	assert.Equal(t, origStatus, w.Status)
}

func TestCheckWorkStatus_WithProcessingTasks_NoTransition(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-proc", "branch-proc", false)

	err := proj.DB.CreateTask(ctx, "w-proc.1", "implement", []string{"b1"}, 10, "w-proc", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-proc.1", "/tmp")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-proc")
	require.NoError(t, err)

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-proc")
	require.NoError(t, err)
	assert.NotEqual(t, db.StatusIdle, w.Status)
	assert.NotEqual(t, db.StatusFailed, w.Status)
}

func TestCheckWorkStatus_PRURLFromPRTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-pr", "branch-pr", false)

	err := proj.DB.CreateTask(ctx, "w-pr.1", "pr", nil, 0, "w-pr", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-pr.1", "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, "w-pr.1", "https://github.com/org/repo/pull/42")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-pr")
	require.NoError(t, err)

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-pr")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, w.Status)
	assert.Equal(t, "https://github.com/org/repo/pull/42", w.PRURL)
}

func TestCheckWorkStatus_NoTasks_NoTransition(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-empty", "branch-empty", false)

	w, err := proj.DB.GetWork(ctx, "w-empty")
	require.NoError(t, err)
	origStatus := w.Status

	s.checkWorkStatus(ctx, w)

	w, err = proj.DB.GetWork(ctx, "w-empty")
	require.NoError(t, err)
	assert.Equal(t, origStatus, w.Status)
}

// --- Run lifecycle tests ---

func TestRun_StopsOnContextCancel(t *testing.T) {
	s, _ := newTestSequencer(t)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	var runErr error
	go func() {
		defer wg.Done()
		runErr = s.Run(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	cancel()
	wg.Wait()

	assert.NoError(t, runErr)
}

func TestRun_NotifyWakesSequencer(t *testing.T) {
	s, _ := newTestSequencer(t)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.Run(ctx)
	}()

	// Notify should not panic or block
	s.Notify()
	time.Sleep(50 * time.Millisecond)

	cancel()
	wg.Wait()
}

func TestWait_BlocksUntilRunExits(t *testing.T) {
	s, _ := newTestSequencer(t)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		_ = s.Run(ctx)
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Wait()
		close(done)
	}()

	// Wait should be blocking
	select {
	case <-done:
		t.Fatal("Wait returned before Run exited")
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	cancel()

	select {
	case <-done:
		// expected - Wait returned after Run exited
	case <-time.After(2 * time.Second):
		t.Fatal("Wait did not return after Run exited")
	}
}

// --- buildTaskInput tests ---

func TestBuildTaskInput_ImplementTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-impl", "feat/impl", false)

	err := proj.DB.CreateTask(ctx, "w-impl.1", "implement", []string{"bean-1", "bean-2"}, 10, "w-impl", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-impl.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-impl")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	assert.Equal(t, "w-impl.1", input.Params.TaskID)
	assert.Equal(t, "w-impl", input.Params.WorkID)
	assert.Equal(t, "feat/impl", input.Params.BranchName)
	assert.Equal(t, "main", input.Params.BaseBranch)
	assert.ElementsMatch(t, []string{"bean-1", "bean-2"}, input.Params.BeanIDs)
	assert.Empty(t, input.TempFilePath)
}

func TestBuildTaskInput_EstimateTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-est", "feat/est", false)

	err := proj.DB.CreateTask(ctx, "w-est.1", "estimate", []string{"bean-a"}, 0, "w-est", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-est.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-est")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	assert.Equal(t, "estimate", string(input.Params.Type))
	assert.Equal(t, []string{"bean-a"}, input.Params.BeanIDs)
}

func TestBuildTaskInput_ReviewTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-rev", "feat/rev", false)

	err := proj.DB.CreateTask(ctx, "w-rev.1", "review", nil, 0, "w-rev", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-rev.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-rev")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	assert.Equal(t, "review", string(input.Params.Type))
	assert.Equal(t, "root-1", input.Params.RootIssueID)
}

func TestBuildTaskInput_PRTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-prt", "feat/prt", false)

	err := proj.DB.CreateTask(ctx, "w-prt.1", "pr", nil, 0, "w-prt", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-prt.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-prt")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	assert.Equal(t, "pr", string(input.Params.Type))
}

func TestBuildTaskInput_UpdatePRDescription_NoPRURL(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-upd", "feat/upd", false)

	err := proj.DB.CreateTask(ctx, "w-upd.1", "update-pr-description", nil, 0, "w-upd", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-upd.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-upd")
	require.NoError(t, err)

	_, err = s.buildTaskInput(ctx, task, w)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "has no PR URL")
}

func TestBuildTaskInput_UnknownType(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-unk", "feat/unk", false)

	err := proj.DB.CreateTask(ctx, "w-unk.1", "nonexistent_type", nil, 0, "w-unk", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-unk.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-unk")
	require.NoError(t, err)

	_, err = s.buildTaskInput(ctx, task, w)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown task type")
}

func TestBuildTaskInput_UsesWorkBaseBranch(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	// Create work with custom base branch
	err := proj.DB.CreateWork(ctx, "w-bb", "w-bb", "/tmp/w-bb", "feat/bb", "develop", "root-1", false)
	require.NoError(t, err)

	err = proj.DB.CreateTask(ctx, "w-bb.1", "implement", []string{"b1"}, 10, "w-bb", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-bb.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-bb")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	assert.Equal(t, "develop", input.Params.BaseBranch)
}

func TestBuildTaskInput_FallsBackToConfigBaseBranch(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	// Create work with empty base branch
	err := proj.DB.CreateWork(ctx, "w-fb", "w-fb", "/tmp/w-fb", "feat/fb", "", "root-1", false)
	require.NoError(t, err)

	err = proj.DB.CreateTask(ctx, "w-fb.1", "implement", []string{"b1"}, 10, "w-fb", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-fb.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-fb")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)
	// Should fall back to config's base branch
	assert.Equal(t, "main", input.Params.BaseBranch)
}

// --- logAnalysisInput tests ---

func TestBuildTaskInput_LogAnalysis_MissingLogContent(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-la", "feat/la", false)

	err := proj.DB.CreateTask(ctx, "w-la.1", "log_analysis", nil, 0, "w-la", 1)
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-la.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-la")
	require.NoError(t, err)

	_, err = s.buildTaskInput(ctx, task, w)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "log_content metadata is missing")
}

func TestBuildTaskInput_LogAnalysis_WithMetadata(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-la2", "feat/la2", false)

	err := proj.DB.CreateTask(ctx, "w-la2.1", "log_analysis", nil, 0, "w-la2", 1)
	require.NoError(t, err)

	// Set metadata
	err = proj.DB.SetTaskMetadata(ctx, "w-la2.1", "log_content", "some log output here")
	require.NoError(t, err)
	err = proj.DB.SetTaskMetadata(ctx, "w-la2.1", "workflow_name", "CI")
	require.NoError(t, err)
	err = proj.DB.SetTaskMetadata(ctx, "w-la2.1", "job_name", "test")
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-la2.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-la2")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)

	assert.Equal(t, "log_analysis", string(input.Params.Type))
	assert.Equal(t, "CI", input.Params.WorkflowName)
	assert.Equal(t, "test", input.Params.JobName)
	assert.NotEmpty(t, input.TempFilePath)
	assert.NotEmpty(t, input.Params.LogFilePath)
	assert.Equal(t, input.TempFilePath, input.Params.LogFilePath)

	// Verify temp file exists and has content
	content, err := os.ReadFile(input.TempFilePath)
	require.NoError(t, err)
	assert.Equal(t, "some log output here", string(content))

	// Clean up
	os.Remove(input.TempFilePath)
}

func TestBuildTaskInput_LogAnalysis_UsesMetadataBranch(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-la3", "feat/la3", false)

	err := proj.DB.CreateTask(ctx, "w-la3.1", "log_analysis", nil, 0, "w-la3", 1)
	require.NoError(t, err)

	err = proj.DB.SetTaskMetadata(ctx, "w-la3.1", "log_content", "log data")
	require.NoError(t, err)
	err = proj.DB.SetTaskMetadata(ctx, "w-la3.1", "branch_name", "custom-branch")
	require.NoError(t, err)
	err = proj.DB.SetTaskMetadata(ctx, "w-la3.1", "root_issue_id", "custom-root")
	require.NoError(t, err)

	task, err := proj.DB.GetTask(ctx, "w-la3.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-la3")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)

	assert.Equal(t, "custom-branch", input.Params.BranchName)
	assert.Equal(t, "custom-root", input.Params.RootIssueID)

	os.Remove(input.TempFilePath)
}

func TestBuildTaskInput_LogAnalysis_FallsBackToWorkFields(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-la4", "feat/la4", false)

	err := proj.DB.CreateTask(ctx, "w-la4.1", "log_analysis", nil, 0, "w-la4", 1)
	require.NoError(t, err)

	err = proj.DB.SetTaskMetadata(ctx, "w-la4.1", "log_content", "log data")
	require.NoError(t, err)
	// No branch_name or root_issue_id metadata - should fall back to work fields

	task, err := proj.DB.GetTask(ctx, "w-la4.1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-la4")
	require.NoError(t, err)

	input, err := s.buildTaskInput(ctx, task, w)
	require.NoError(t, err)

	assert.Equal(t, "feat/la4", input.Params.BranchName)
	assert.Equal(t, "root-1", input.Params.RootIssueID)

	os.Remove(input.TempFilePath)
}

// --- createPRTask tests ---

func TestCreatePRTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-cpr", "feat/cpr", false)

	// Use GetNextTaskNumber to keep the counter in sync
	num, err := proj.DB.GetNextTaskNumber(ctx, "w-cpr")
	require.NoError(t, err)
	reviewID := fmt.Sprintf("w-cpr.%d", num)
	err = proj.DB.CreateTask(ctx, reviewID, "review", nil, 0, "w-cpr", num)
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-cpr")
	require.NoError(t, err)

	err = s.createPRTask(ctx, w, reviewID)
	require.NoError(t, err)

	// Verify PR task was created
	tasks, err := proj.DB.GetWorkTasks(ctx, "w-cpr")
	require.NoError(t, err)
	assert.Len(t, tasks, 2) // review + pr

	var prTask *db.Task
	for _, task := range tasks {
		if task.TaskType == "pr" {
			prTask = task
			break
		}
	}
	require.NotNil(t, prTask)
	assert.Equal(t, "pr", prTask.TaskType)
	assert.Equal(t, db.StatusPending, prTask.Status)
}

func TestCreatePRTask_AlreadyExistsPending_Noop(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-cpr2", "feat/cpr2", false)

	num1, err := proj.DB.GetNextTaskNumber(ctx, "w-cpr2")
	require.NoError(t, err)
	reviewID := fmt.Sprintf("w-cpr2.%d", num1)
	err = proj.DB.CreateTask(ctx, reviewID, "review", nil, 0, "w-cpr2", num1)
	require.NoError(t, err)

	num2, err := proj.DB.GetNextTaskNumber(ctx, "w-cpr2")
	require.NoError(t, err)
	prID := fmt.Sprintf("w-cpr2.%d", num2)
	err = proj.DB.CreateTask(ctx, prID, "pr", nil, 0, "w-cpr2", num2)
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-cpr2")
	require.NoError(t, err)

	err = s.createPRTask(ctx, w, reviewID)
	require.NoError(t, err)

	tasks, err := proj.DB.GetWorkTasks(ctx, "w-cpr2")
	require.NoError(t, err)

	prCount := 0
	for _, task := range tasks {
		if task.TaskType == "pr" {
			prCount++
		}
	}
	assert.Equal(t, 1, prCount, "should not create duplicate PR task")
}

func TestCreatePRTask_CompletedPR_CreatesUpdateTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-cpr3", "feat/cpr3", false)

	num1, err := proj.DB.GetNextTaskNumber(ctx, "w-cpr3")
	require.NoError(t, err)
	reviewID := fmt.Sprintf("w-cpr3.%d", num1)
	err = proj.DB.CreateTask(ctx, reviewID, "review", nil, 0, "w-cpr3", num1)
	require.NoError(t, err)

	num2, err := proj.DB.GetNextTaskNumber(ctx, "w-cpr3")
	require.NoError(t, err)
	prID := fmt.Sprintf("w-cpr3.%d", num2)
	err = proj.DB.CreateTask(ctx, prID, "pr", nil, 0, "w-cpr3", num2)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, prID, "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, prID, "https://github.com/org/repo/pull/1")
	require.NoError(t, err)

	// Set PR URL on work
	err = proj.DB.IdleWorkWithPR(ctx, "w-cpr3", "https://github.com/org/repo/pull/1")
	require.NoError(t, err)

	w, err := proj.DB.GetWork(ctx, "w-cpr3")
	require.NoError(t, err)

	err = s.createPRTask(ctx, w, reviewID)
	require.NoError(t, err)

	tasks, err := proj.DB.GetWorkTasks(ctx, "w-cpr3")
	require.NoError(t, err)

	hasUpdatePR := false
	for _, task := range tasks {
		if task.TaskType == "update-pr-description" {
			hasUpdatePR = true
		}
	}
	assert.True(t, hasUpdatePR, "should create update-pr-description task when PR already completed")
}

// --- killAllActiveSessions tests ---

func TestKillAllActiveSessions_Empty(t *testing.T) {
	s, _ := newTestSequencer(t)
	// Should not panic with empty active tasks
	s.killAllActiveSessions()
}

func TestKillAllActiveSessions_WithTasks(t *testing.T) {
	s, _ := newTestSequencer(t)

	// Add some active tasks
	s.mu.Lock()
	s.activeTasks["w-1"] = "t-1"
	s.activeTasks["w-2"] = "t-2"
	s.mu.Unlock()

	// Should not panic even though sessions don't exist in bridge
	s.killAllActiveSessions()
}

// --- resetAllStuckTasks tests ---

func TestResetAllStuckTasks_NoWorks(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestSequencer(t)

	// Should not panic with no works
	s.resetAllStuckTasks(ctx)
}

func TestResetAllStuckTasks_NoStuckTasks(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-nostuck", "feat/nostuck", false)

	// Create a pending task (not stuck)
	err := proj.DB.CreateTask(ctx, "w-nostuck.1", "implement", []string{"b1"}, 10, "w-nostuck", 1)
	require.NoError(t, err)

	// Should not panic - no stuck processing tasks to reset
	s.resetAllStuckTasks(ctx)
}

// --- beanIDsForTask tests ---

func TestBeanIDsForTask(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-beans", "feat/beans", false)

	err := proj.DB.CreateTask(ctx, "w-beans.1", "implement", []string{"bean-x", "bean-y", "bean-z"}, 10, "w-beans", 1)
	require.NoError(t, err)

	beanIDs, err := s.beanIDsForTask(ctx, "w-beans.1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"bean-x", "bean-y", "bean-z"}, beanIDs)
}

func TestBeanIDsForTask_NoBeans(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-nb", "feat/nb", false)

	err := proj.DB.CreateTask(ctx, "w-nb.1", "review", nil, 0, "w-nb", 1)
	require.NoError(t, err)

	beanIDs, err := s.beanIDsForTask(ctx, "w-nb.1")
	require.NoError(t, err)
	assert.Empty(t, beanIDs)
}

// --- Integration: poll triggers checkWorkStatus transitions ---

func TestPoll_TransitionsIdleWhenAllTasksComplete(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-poll-idle", "feat/poll-idle", false)

	// Create two tasks, complete both with dependencies satisfied
	err := proj.DB.CreateTask(ctx, "w-poll-idle.1", "implement", []string{"b1"}, 10, "w-poll-idle", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-poll-idle.1", "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, "w-poll-idle.1", "")
	require.NoError(t, err)

	err = proj.DB.CreateTask(ctx, "w-poll-idle.2", "review", nil, 0, "w-poll-idle", 2)
	require.NoError(t, err)
	err = proj.DB.AddTaskDependency(ctx, "w-poll-idle.2", "w-poll-idle.1")
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-poll-idle.2", "/tmp")
	require.NoError(t, err)
	err = proj.DB.CompleteTask(ctx, "w-poll-idle.2", "")
	require.NoError(t, err)

	// Poll - no ready tasks, all complete → should idle the work
	s.poll(ctx)

	w, err := proj.DB.GetWork(ctx, "w-poll-idle")
	require.NoError(t, err)
	assert.Equal(t, db.StatusIdle, w.Status)
}

func TestPoll_TransitionsFailedWhenTasksFail(t *testing.T) {
	ctx := context.Background()
	s, proj := newTestSequencer(t)

	createTestWork(ctx, t, proj.DB, "w-poll-fail", "feat/poll-fail", false)

	err := proj.DB.CreateTask(ctx, "w-poll-fail.1", "implement", []string{"b1"}, 10, "w-poll-fail", 1)
	require.NoError(t, err)
	err = proj.DB.StartTask(ctx, "w-poll-fail.1", "/tmp")
	require.NoError(t, err)
	err = proj.DB.FailTask(ctx, "w-poll-fail.1", "build failed")
	require.NoError(t, err)

	s.poll(ctx)

	w, err := proj.DB.GetWork(ctx, "w-poll-fail")
	require.NoError(t, err)
	assert.Equal(t, db.StatusFailed, w.Status)
}

// --- Concurrent safety tests ---

func TestActiveTasks_ConcurrentAccess(t *testing.T) {
	s, _ := newTestSequencer(t)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.mu.Lock()
			s.activeTasks["w-"+string(rune('a'+i%26))] = "task"
			s.mu.Unlock()
		}(i)
		go func() {
			defer wg.Done()
			_ = s.ActiveTasks()
		}()
	}
	wg.Wait()
}

func TestNotify_ConcurrentCalls(t *testing.T) {
	s, _ := newTestSequencer(t)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Notify()
		}()
	}
	wg.Wait()
}
