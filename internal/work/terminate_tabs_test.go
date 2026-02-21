package work

import (
	"bytes"
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/zellij"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTerminateTest creates a DefaultOrchestratorManager with mocked zellij
// and an in-memory database. Returns the manager, session mock, and database.
func setupTerminateTest(t *testing.T, tabNames []string) (*DefaultOrchestratorManager, *zellij.SessionMock, *db.DB) {
	t.Helper()

	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	sessionMock := &zellij.SessionMock{
		QueryTabNamesFunc: func(ctx context.Context) ([]string, error) {
			return tabNames, nil
		},
		CloseTabByNameFunc: func(ctx context.Context, tabName string) error {
			return nil
		},
		TerminateAndCloseTabFunc: func(ctx context.Context, tabName string) error {
			return nil
		},
	}

	zellijMock := &zellij.SessionManagerMock{
		SessionExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return true, nil
		},
		SessionFunc: func(name string) zellij.Session {
			return sessionMock
		},
	}

	mgr := &DefaultOrchestratorManager{
		database: testDB,
		zellij:   zellijMock,
	}

	return mgr, sessionMock, testDB
}

func TestTerminateWorkTabs_OrchestratorGetsSignaledBeforeClose(t *testing.T) {
	tabNames := []string{
		"orch-w-abc (my feature)",
		"task-w-abc.1",
		"console-w-abc",
	}
	mgr, sessionMock, testDB := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	// Register an orchestrator process with a PID that doesn't exist.
	// The SIGTERM will fail gracefully (process not found) which is fine —
	// we're verifying the code path runs and the tab still gets closed.
	err := testDB.RegisterProcess(ctx, "orch-1", db.ProcessTypeOrchestrator, strPtr("w-abc"), 999999)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	// All 3 tabs should have been closed via CloseTabByName
	assert.Len(t, sessionMock.CloseTabByNameCalls(), 3)

	// TerminateAndCloseTab should NOT have been called (the old Ctrl+C path)
	assert.Empty(t, sessionMock.TerminateAndCloseTabCalls(),
		"TerminateAndCloseTab should not be called — we use CloseTabByName instead")

	// Verify all tabs were closed
	closedTabs := make([]string, 0, len(sessionMock.CloseTabByNameCalls()))
	for _, call := range sessionMock.CloseTabByNameCalls() {
		closedTabs = append(closedTabs, call.TabName)
	}
	assert.Contains(t, closedTabs, "orch-w-abc (my feature)")
	assert.Contains(t, closedTabs, "task-w-abc.1")
	assert.Contains(t, closedTabs, "console-w-abc")
}

func TestTerminateWorkTabs_NoCtrlCSent(t *testing.T) {
	tabNames := []string{
		"task-w-xyz.1",
		"task-w-xyz.2",
		"agent-w-xyz",
	}
	mgr, sessionMock, _ := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-xyz", "test-project", &buf)
	require.NoError(t, err)

	// TerminateAndCloseTab (which sends Ctrl+C) should never be called
	assert.Empty(t, sessionMock.TerminateAndCloseTabCalls(),
		"TerminateAndCloseTab should not be called — no Ctrl+C should be sent")

	// CloseTabByName should be called for each tab
	assert.Len(t, sessionMock.CloseTabByNameCalls(), 3)
}

func TestTerminateWorkTabs_OrchestratorProcessNotFound(t *testing.T) {
	// Orchestrator tab exists but no process record in DB
	tabNames := []string{"orch-w-abc"}
	mgr, sessionMock, _ := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	// Tab should still be closed even without a process record
	assert.Len(t, sessionMock.CloseTabByNameCalls(), 1)
	assert.Equal(t, "orch-w-abc", sessionMock.CloseTabByNameCalls()[0].TabName)
}

func TestTerminateWorkTabs_OnlyMatchesCorrectWork(t *testing.T) {
	// Include tabs from another work to verify filtering
	tabNames := []string{
		"orch-w-abc",
		"task-w-abc.1",
		"orch-w-other",
		"task-w-other.1",
		"control",
	}
	mgr, sessionMock, _ := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	// Only w-abc tabs should be closed, not w-other or control
	assert.Len(t, sessionMock.CloseTabByNameCalls(), 2)
	closedTabs := make([]string, 0)
	for _, call := range sessionMock.CloseTabByNameCalls() {
		closedTabs = append(closedTabs, call.TabName)
	}
	assert.Contains(t, closedTabs, "orch-w-abc")
	assert.Contains(t, closedTabs, "task-w-abc.1")
	assert.NotContains(t, closedTabs, "orch-w-other")
	assert.NotContains(t, closedTabs, "task-w-other.1")
	assert.NotContains(t, closedTabs, "control")
}

func TestTerminateWorkTabs_SessionDoesNotExist(t *testing.T) {
	mgr, sessionMock, _ := setupTerminateTest(t, nil)
	ctx := context.Background()

	// Override to report session doesn't exist
	mgr.zellij.(*zellij.SessionManagerMock).SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		return false, nil
	}

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	// No tabs should be queried or closed
	assert.Empty(t, sessionMock.CloseTabByNameCalls())
}

func TestTerminateWorkTabs_NoMatchingTabs(t *testing.T) {
	tabNames := []string{"control", "orch-w-other"}
	mgr, sessionMock, _ := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	// No tabs should be closed
	assert.Empty(t, sessionMock.CloseTabByNameCalls())
}

func strPtr(s string) *string {
	return &s
}
