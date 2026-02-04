package procmon

import (
	"context"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*db.DB, func()) {
	t.Helper()

	database, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err, "failed to open database")

	cleanup := func() {
		database.Close()
	}

	return database, cleanup
}

func TestNewManager(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()

	// Default heartbeat interval
	m := NewManager(database, 0)
	assert.Equal(t, db.DefaultHeartbeatInterval, m.heartbeat)

	// Custom heartbeat interval
	m2 := NewManager(database, 5*time.Second)
	assert.Equal(t, 5*time.Second, m2.heartbeat)
}

func TestRegisterControlPlane(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	err := m.RegisterControlPlane(ctx)
	require.NoError(t, err)

	// Verify process was registered
	proc, err := database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, db.ProcessTypeControlPlane, proc.ProcessType)
	assert.Nil(t, proc.WorkID)

	// Cannot register again while running
	err = m.RegisterControlPlane(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestRegisterOrchestrator(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	workID := "work-123"
	err := m.RegisterOrchestrator(ctx, workID)
	require.NoError(t, err)

	// Verify process was registered
	proc, err := database.GetOrchestratorProcess(ctx, workID)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, db.ProcessTypeOrchestrator, proc.ProcessType)
	require.NotNil(t, proc.WorkID)
	assert.Equal(t, workID, *proc.WorkID)

	// Cannot register again while running
	err = m.RegisterOrchestrator(ctx, "another-work")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestStop(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)

	err := m.RegisterControlPlane(ctx)
	require.NoError(t, err)

	// Verify process exists before stop
	proc, err := database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)

	m.Stop()

	// Verify process was unregistered
	proc, err = database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	assert.Nil(t, proc, "process should be nil after unregistration")

	// Stop is idempotent - calling again should not panic
	m.Stop()
}

func TestHeartbeatUpdates(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Use a long heartbeat interval since we'll trigger manually
	m := NewManager(database, time.Hour)
	defer m.Stop()

	// Use a mock time function that returns a time far in the future
	// This ensures the updated heartbeat is definitely after the initial
	// (which is set by SQLite CURRENT_TIMESTAMP during registration)
	mockTime := time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)
	m.SetNowFunc(func() time.Time {
		return mockTime
	})

	err := m.RegisterControlPlane(ctx)
	require.NoError(t, err)

	// Get initial heartbeat (set by SQLite CURRENT_TIMESTAMP during registration)
	proc, err := database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	initialHeartbeat := proc.Heartbeat

	// Trigger an immediate heartbeat update (no sleep needed)
	err = m.TriggerHeartbeat()
	require.NoError(t, err)

	// Read updated heartbeat
	proc, err = database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	updatedHeartbeat := proc.Heartbeat

	// The updated heartbeat should be our mock time (2099), which is after
	// the initial heartbeat (set by SQLite CURRENT_TIMESTAMP)
	assert.True(t, updatedHeartbeat.After(initialHeartbeat),
		"heartbeat should have been updated (initial: %v, updated: %v)",
		initialHeartbeat, updatedHeartbeat)
	assert.Equal(t, mockTime, updatedHeartbeat,
		"heartbeat should be set to mock time")
}

func TestIsOrchestratorAlive(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	workID := "work-123"

	// Not alive before registration
	alive, err := m.IsOrchestratorAlive(ctx, workID)
	require.NoError(t, err)
	assert.False(t, alive)

	// Register orchestrator
	err = m.RegisterOrchestrator(ctx, workID)
	require.NoError(t, err)

	// Should be alive now
	alive, err = m.IsOrchestratorAlive(ctx, workID)
	require.NoError(t, err)
	assert.True(t, alive)
}

func TestIsControlPlaneAlive(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	// Not alive before registration
	alive, err := m.IsControlPlaneAlive(ctx)
	require.NoError(t, err)
	assert.False(t, alive)

	// Register control plane
	err = m.RegisterControlPlane(ctx)
	require.NoError(t, err)

	// Should be alive now
	alive, err = m.IsControlPlaneAlive(ctx)
	require.NoError(t, err)
	assert.True(t, alive)
}

func TestCleanupStaleProcessRecords(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a process directly in the database with an old heartbeat
	// to simulate a stale process
	workID := "stale-work"
	err := database.RegisterProcess(ctx, "stale-id", db.ProcessTypeOrchestrator, &workID, 12345)
	require.NoError(t, err)

	// Set heartbeat to 60 seconds ago to simulate a stale process
	oldTime := time.Now().Add(-60 * time.Second)
	err = database.UpdateHeartbeatWithTime(ctx, "stale-id", oldTime)
	require.NoError(t, err)

	// Verify the stale process exists
	proc, err := database.GetOrchestratorProcess(ctx, workID)
	require.NoError(t, err)
	require.NotNil(t, proc)

	// Create a manager and cleanup stale processes
	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	err = m.CleanupStaleProcessRecords(ctx)
	require.NoError(t, err)

	// Stale process should be removed
	proc, err = database.GetOrchestratorProcess(ctx, workID)
	require.NoError(t, err)
	assert.Nil(t, proc, "stale process should have been cleaned up")
}

func TestGetOrchestratorProcess(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	workID := "work-456"

	// Not found before registration
	proc, err := m.GetOrchestratorProcess(ctx, workID)
	require.NoError(t, err)
	assert.Nil(t, proc)

	// Register and verify
	err = m.RegisterOrchestrator(ctx, workID)
	require.NoError(t, err)

	proc, err = m.GetOrchestratorProcess(ctx, workID)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, workID, *proc.WorkID)
}

func TestGetControlPlaneProcess(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	m := NewManager(database, 100*time.Millisecond)
	defer m.Stop()

	// Not found before registration
	proc, err := m.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	assert.Nil(t, proc)

	// Register and verify
	err = m.RegisterControlPlane(ctx)
	require.NoError(t, err)

	proc, err = m.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Equal(t, db.ProcessTypeControlPlane, proc.ProcessType)
}

func TestGetAllProcesses(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// No processes initially
	m := NewManager(database, 100*time.Millisecond)

	procs, err := m.GetAllProcesses(ctx)
	require.NoError(t, err)
	assert.Len(t, procs, 0)

	// Register control plane
	err = m.RegisterControlPlane(ctx)
	require.NoError(t, err)
	defer m.Stop()

	// Register orchestrators using separate managers
	m2 := NewManager(database, 100*time.Millisecond)
	err = m2.RegisterOrchestrator(ctx, "work-1")
	require.NoError(t, err)
	defer m2.Stop()

	m3 := NewManager(database, 100*time.Millisecond)
	err = m3.RegisterOrchestrator(ctx, "work-2")
	require.NoError(t, err)
	defer m3.Stop()

	procs, err = m.GetAllProcesses(ctx)
	require.NoError(t, err)
	assert.Len(t, procs, 3, "expected 3 processes (1 control plane + 2 orchestrators)")
}
