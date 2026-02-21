package work

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/zmx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// filterByPrefix filters a list of strings by prefix, matching real ListSessions behavior.
func filterByPrefix(items []string, prefix string) []string {
	var result []string
	for _, s := range items {
		if strings.HasPrefix(s, prefix) {
			result = append(result, s)
		}
	}
	return result
}

// setupZmxTest creates a DefaultOrchestratorManager configured for zmx with mocked dependencies.
func setupZmxTest(t *testing.T) (*DefaultOrchestratorManager, *zmx.ClientMock, *db.DB) {
	t.Helper()

	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	zmxMock := &zmx.ClientMock{
		SessionExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return false, nil
		},
		RunSessionFunc: func(ctx context.Context, name, command string, args []string, cwd string) error {
			return nil
		},
		AttachSessionFunc: func(ctx context.Context, name string, terminalCmdTemplate string) error {
			return nil
		},
		KillSessionFunc: func(ctx context.Context, name string) error {
			return nil
		},
		ListSessionsFunc: func(ctx context.Context, prefix string) ([]string, error) {
			return nil, nil
		},
	}

	mgr := &DefaultOrchestratorManager{
		database:  testDB,
		zmx:       zmxMock,
		muxConfig: &project.MultiplexerConfig{Type: "zmx"},
	}

	return mgr, zmxMock, testDB
}

func TestSpawnWorkOrchestratorZmx_CreatesSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.SpawnWorkOrchestrator(ctx, "w-abc", "myproj", "/tmp/work", "my feature", &buf)
	require.NoError(t, err)

	// Should have called RunSession
	calls := zmxMock.RunSessionCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "sarge-myproj.orch-w-abc-my-feature", calls[0].Name)
	assert.Equal(t, "sarge", calls[0].Command)
	assert.Equal(t, []string{"orchestrate", "--work", "w-abc"}, calls[0].Args)
	assert.Equal(t, "/tmp/work", calls[0].Cwd)
}

func TestSpawnWorkOrchestratorZmx_KillsExistingSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	expectedName := "sarge-myproj.orch-w-abc"
	zmxMock.SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		if name == expectedName {
			return true, nil
		}
		return false, nil
	}

	var buf bytes.Buffer
	err := mgr.SpawnWorkOrchestrator(ctx, "w-abc", "myproj", "/tmp/work", "", &buf)
	require.NoError(t, err)

	// Should have killed the existing session first
	killCalls := zmxMock.KillSessionCalls()
	require.Len(t, killCalls, 1)
	assert.Equal(t, expectedName, killCalls[0].Name)

	// Then created a new one
	runCalls := zmxMock.RunSessionCalls()
	require.Len(t, runCalls, 1)
}

func TestTerminateWorkTabsZmx_KillsMatchingSessions(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	allSessions := []string{
		"sarge-myproj.orch-w-abc-my-feature",
		"sarge-myproj.task-w-abc.1",
		"sarge-myproj.console-w-abc",
		"sarge-myproj.claude-w-abc",
		"sarge-myproj.pi-w-abc",
		"sarge-myproj.orch-w-other",
		"sarge-myproj.control",
	}
	zmxMock.ListSessionsFunc = func(ctx context.Context, prefix string) ([]string, error) {
		return filterByPrefix(allSessions, prefix), nil
	}

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "myproj", &buf)
	require.NoError(t, err)

	// Should kill 5 sessions (work, task, console, claude, pi) but not w-other or control
	killCalls := zmxMock.KillSessionCalls()
	assert.Len(t, killCalls, 5)

	killedNames := make([]string, 0, len(killCalls))
	for _, call := range killCalls {
		killedNames = append(killedNames, call.Name)
	}
	assert.Contains(t, killedNames, "sarge-myproj.orch-w-abc-my-feature")
	assert.Contains(t, killedNames, "sarge-myproj.task-w-abc.1")
	assert.Contains(t, killedNames, "sarge-myproj.console-w-abc")
	assert.Contains(t, killedNames, "sarge-myproj.claude-w-abc")
	assert.Contains(t, killedNames, "sarge-myproj.pi-w-abc")
}

func TestTerminateWorkTabsZmx_NoMatchingSessions(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	allSessions := []string{
		"sarge-myproj.orch-w-other",
		"sarge-myproj.control",
	}
	zmxMock.ListSessionsFunc = func(ctx context.Context, prefix string) ([]string, error) {
		return filterByPrefix(allSessions, prefix), nil
	}

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "myproj", &buf)
	require.NoError(t, err)

	assert.Empty(t, zmxMock.KillSessionCalls())
}

func TestTerminateWorkTabsZmx_OrchestratorGetsSignaled(t *testing.T) {
	mgr, zmxMock, testDB := setupZmxTest(t)
	ctx := context.Background()

	allSessions := []string{"sarge-myproj.orch-w-abc"}
	zmxMock.ListSessionsFunc = func(ctx context.Context, prefix string) ([]string, error) {
		return filterByPrefix(allSessions, prefix), nil
	}

	// Register an orchestrator process with a non-existent PID
	err := testDB.RegisterProcess(ctx, "orch-1", db.ProcessTypeOrchestrator, strPtr("w-abc"), 999999)
	require.NoError(t, err)

	var buf bytes.Buffer
	err = mgr.TerminateWorkTabs(ctx, "w-abc", "myproj", &buf)
	require.NoError(t, err)

	// Session should still be killed even if SIGTERM fails
	assert.Len(t, zmxMock.KillSessionCalls(), 1)
}

func TestOpenConsoleZmx_CreatesSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.OpenConsole(ctx, "w-abc", "myproj", "/tmp/work", "my feature", nil, &buf)
	require.NoError(t, err)

	// Should create session with "" command (zmx default shell) and correct cwd
	runCalls := zmxMock.RunSessionCalls()
	require.Len(t, runCalls, 1)
	assert.Equal(t, "sarge-myproj.console-w-abc-my-feature", runCalls[0].Name)
	assert.Equal(t, "", runCalls[0].Command)
	assert.Equal(t, "/tmp/work", runCalls[0].Cwd)

	// Then attach
	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
	assert.Equal(t, "sarge-myproj.console-w-abc-my-feature", attachCalls[0].Name)
}

func TestOpenConsoleZmx_ExistingSessionAttaches(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	zmxMock.SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		return true, nil
	}

	var buf bytes.Buffer
	err := mgr.OpenConsole(ctx, "w-abc", "myproj", "/tmp/work", "", nil, &buf)
	require.NoError(t, err)

	// Should NOT create session
	assert.Empty(t, zmxMock.RunSessionCalls())

	// Should attach
	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
}

func TestOpenAgentSessionZmx_CreatesClaudeSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	cfg := &project.Config{}

	var buf bytes.Buffer
	err := mgr.OpenAgentSession(ctx, "w-abc", "myproj", "/tmp/work", "", nil, cfg, &buf)
	require.NoError(t, err)

	// Should create session with agent command in one call
	runCalls := zmxMock.RunSessionCalls()
	require.Len(t, runCalls, 1)
	assert.Equal(t, "sarge-myproj.claude-w-abc", runCalls[0].Name)
	assert.Equal(t, "claude --dangerously-skip-permissions", runCalls[0].Command)
	assert.Equal(t, "/tmp/work", runCalls[0].Cwd)

	// Then attach
	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
}

func TestOpenAgentSessionZmx_CreatesPiSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	cfg := &project.Config{}
	cfg.Agent.Type = "pi"

	var buf bytes.Buffer
	err := mgr.OpenAgentSession(ctx, "w-abc", "myproj", "/tmp/work", "feat", nil, cfg, &buf)
	require.NoError(t, err)

	runCalls := zmxMock.RunSessionCalls()
	require.Len(t, runCalls, 1)
	assert.Equal(t, "sarge-myproj.pi-w-abc-feat", runCalls[0].Name)
	assert.Equal(t, "pi", runCalls[0].Command)

	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
}

func TestOpenAgentSessionZmx_ExistingSessionAttaches(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	zmxMock.SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		return true, nil
	}

	var buf bytes.Buffer
	err := mgr.OpenAgentSession(ctx, "w-abc", "myproj", "/tmp/work", "", nil, &project.Config{}, &buf)
	require.NoError(t, err)

	// Should NOT create session
	assert.Empty(t, zmxMock.RunSessionCalls())

	// Should attach
	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
}

func TestSpawnPlanSessionZmx_CreatesSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.SpawnPlanSession(ctx, "ac-cdo.5", "myproj", "/tmp/repo", &buf)
	require.NoError(t, err)

	// Should create via RunSession with correct cwd
	runCalls := zmxMock.RunSessionCalls()
	require.Len(t, runCalls, 1)
	assert.Equal(t, "sarge-myproj.plan-ac-cdo.5", runCalls[0].Name)
	assert.Equal(t, "sarge", runCalls[0].Command)
	assert.Equal(t, []string{"plan", "ac-cdo.5"}, runCalls[0].Args)
	assert.Equal(t, "/tmp/repo", runCalls[0].Cwd)

	// Then attach
	attachCalls := zmxMock.AttachSessionCalls()
	require.Len(t, attachCalls, 1)
}

func TestSpawnPlanSessionZmx_KillsExistingSession(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	expectedName := "sarge-myproj.plan-ac-cdo.5"
	zmxMock.SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		if name == expectedName {
			return true, nil
		}
		return false, nil
	}

	var buf bytes.Buffer
	err := mgr.SpawnPlanSession(ctx, "ac-cdo.5", "myproj", "/tmp/repo", &buf)
	require.NoError(t, err)

	killCalls := zmxMock.KillSessionCalls()
	require.Len(t, killCalls, 1)
	assert.Equal(t, expectedName, killCalls[0].Name)

	// Should still create and attach
	require.Len(t, zmxMock.RunSessionCalls(), 1)
	require.Len(t, zmxMock.AttachSessionCalls(), 1)
}

func TestEnsureWorkOrchestratorZmx_SpawnsWhenNotRunning(t *testing.T) {
	mgr, zmxMock, _ := setupZmxTest(t)
	ctx := context.Background()

	spawned, err := mgr.EnsureWorkOrchestrator(ctx, "w-abc", "myproj", "/tmp/work", "", io.Discard)
	require.NoError(t, err)
	assert.True(t, spawned)

	assert.Len(t, zmxMock.RunSessionCalls(), 1)
}

func TestEnsureWorkOrchestratorZmx_SkipsWhenAlive(t *testing.T) {
	mgr, zmxMock, testDB := setupZmxTest(t)
	ctx := context.Background()

	// Session exists
	zmxMock.SessionExistsFunc = func(ctx context.Context, name string) (bool, error) {
		return true, nil
	}

	// Register a live orchestrator heartbeat
	err := testDB.RegisterProcess(ctx, "orch-1", db.ProcessTypeOrchestrator, strPtr("w-abc"), 999999)
	require.NoError(t, err)
	_, err = testDB.UpdateHeartbeat(ctx, "orch-1")
	require.NoError(t, err)

	spawned, err := mgr.EnsureWorkOrchestrator(ctx, "w-abc", "myproj", "/tmp/work", "", io.Discard)
	require.NoError(t, err)
	assert.False(t, spawned)

	// Should NOT have called RunSession
	assert.Empty(t, zmxMock.RunSessionCalls())
}

func TestTerminateWorkTabsZellij_IncludesPiTabs(t *testing.T) {
	// Verify the zellij path also cleans up pi- tabs after our fix
	tabNames := []string{
		"orch-w-abc",
		"pi-w-abc (feat)",
		"console-w-abc",
	}
	mgr, sessionMock, _ := setupTerminateTest(t, tabNames)
	ctx := context.Background()

	var buf bytes.Buffer
	err := mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)

	assert.Len(t, sessionMock.CloseTabByNameCalls(), 3)

	closedTabs := make([]string, 0)
	for _, call := range sessionMock.CloseTabByNameCalls() {
		closedTabs = append(closedTabs, call.TabName)
	}
	assert.Contains(t, closedTabs, "pi-w-abc (feat)")
}
