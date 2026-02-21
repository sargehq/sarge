package control

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/zmx"
)

func newTestProject(t *testing.T, ctx context.Context) *project.Project {
	t.Helper()
	database, err := db.OpenPath(ctx, ":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &project.Project{
		Root: t.TempDir(),
		Config: &project.Config{
			Project: project.ProjectConfig{Name: "testproj"},
			Multiplexer: project.MultiplexerConfig{
				Type: "zmx",
			},
		},
		DB: database,
	}
}

func TestEnsureControlPlaneZmx_SessionNotExisting(t *testing.T) {
	ctx := context.Background()
	proj := newTestProject(t, ctx)

	mock := &zmx.ClientMock{
		SessionExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return false, nil
		},
		RunSessionFunc: func(ctx context.Context, name, command string, args []string, cwd string) error {
			return nil
		},
	}

	result, err := ensureControlPlaneZmxWith(ctx, proj, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.SessionCreated {
		t.Error("expected SessionCreated to be true")
	}
	expectedName := zmx.SessionName("testproj", ControlPlaneTabName)
	if result.SessionName != expectedName {
		t.Errorf("expected session name %q, got %q", expectedName, result.SessionName)
	}

	// Verify RunSession was called
	calls := mock.RunSessionCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 RunSession call, got %d", len(calls))
	}
	if calls[0].Name != expectedName {
		t.Errorf("RunSession called with name %q, expected %q", calls[0].Name, expectedName)
	}
}

func TestEnsureControlPlaneZmx_SessionExistingAndAlive(t *testing.T) {
	ctx := context.Background()
	proj := newTestProject(t, ctx)

	// Register a process and update heartbeat so IsControlPlaneAlive returns true
	if err := proj.DB.RegisterProcess(ctx, "control-plane", "control_plane", nil, 1); err != nil {
		t.Fatalf("failed to register process: %v", err)
	}
	if _, err := proj.DB.UpdateHeartbeat(ctx, "control-plane"); err != nil {
		t.Fatalf("failed to update heartbeat: %v", err)
	}

	mock := &zmx.ClientMock{
		SessionExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return true, nil
		},
	}

	result, err := ensureControlPlaneZmxWith(ctx, proj, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionCreated {
		t.Error("expected SessionCreated to be false for existing alive session")
	}

	// Verify RunSession was NOT called
	if len(mock.RunSessionCalls()) != 0 {
		t.Error("expected no RunSession calls for alive session")
	}
}

func TestEnsureControlPlaneZmx_SessionExistingButDead(t *testing.T) {
	ctx := context.Background()
	proj := newTestProject(t, ctx)

	// No heartbeat registered, so IsControlPlaneAlive returns false

	mock := &zmx.ClientMock{
		SessionExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return true, nil
		},
		KillSessionFunc: func(ctx context.Context, name string) error {
			return nil
		},
		RunSessionFunc: func(ctx context.Context, name, command string, args []string, cwd string) error {
			return nil
		},
	}

	result, err := ensureControlPlaneZmxWith(ctx, proj, mock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.SessionCreated {
		t.Error("expected SessionCreated to be true for restarted session")
	}

	// Verify KillSession was called to kill dead session
	if len(mock.KillSessionCalls()) != 1 {
		t.Fatalf("expected 1 KillSession call, got %d", len(mock.KillSessionCalls()))
	}

	// Verify RunSession was called to restart
	if len(mock.RunSessionCalls()) != 1 {
		t.Fatalf("expected 1 RunSession call, got %d", len(mock.RunSessionCalls()))
	}
}
