// Package procmon provides process monitoring via database-backed heartbeats.
// It enables detection of hung processes (alive but not making progress) via stale heartbeats.
package procmon

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
)

// Manager handles process registration, heartbeat updates, and cleanup.
type Manager struct {
	db        *db.DB
	id        string
	procType  string
	workID    *string
	heartbeat time.Duration
	nowFunc   func() time.Time // For testing; defaults to time.Now

	mu        sync.Mutex
	running   bool
	stopCh    chan struct{}
	stoppedCh chan struct{}
	triggerCh chan chan error // For testing: trigger immediate heartbeat
}

// NewManager creates a new process manager.
func NewManager(database *db.DB, heartbeatInterval time.Duration) *Manager {
	if heartbeatInterval <= 0 {
		heartbeatInterval = db.DefaultHeartbeatInterval
	}
	return &Manager{
		db:        database,
		heartbeat: heartbeatInterval,
		nowFunc:   time.Now,
	}
}

// SetNowFunc sets the time function used for heartbeat updates.
// This is primarily for testing purposes.
func (m *Manager) SetNowFunc(f func() time.Time) {
	m.nowFunc = f
}

// RegisterControlPlane registers this process as the control plane.
// Any existing stale control plane record is cleaned up first.
// Returns an error if registration fails.
func (m *Manager) RegisterControlPlane(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("manager already running")
	}

	// Clean up any stale control plane record before registering
	// This handles cases where a previous control plane was killed without cleanup
	if err := m.db.CleanupStaleControlPlane(ctx); err != nil {
		logging.Warn("failed to cleanup stale control plane", "error", err)
		// Continue anyway - the registration will fail if there's a real conflict
	}

	m.id = uuid.New().String()
	m.procType = db.ProcessTypeControlPlane
	m.workID = nil

	if err := m.db.RegisterProcess(ctx, m.id, m.procType, nil, os.Getpid()); err != nil {
		return fmt.Errorf("failed to register control plane: %w", err)
	}

	m.startHeartbeat()
	logging.Info("registered control plane", "id", m.id, "pid", os.Getpid())
	return nil
}

// RegisterOrchestrator registers this process as an orchestrator for the given work ID.
// Any existing stale orchestrator record for this work ID is cleaned up first.
// Returns an error if registration fails.
func (m *Manager) RegisterOrchestrator(ctx context.Context, workID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("manager already running")
	}

	// Clean up any stale orchestrator record before registering
	// This handles cases where a previous orchestrator was killed without cleanup
	if err := m.db.CleanupStaleOrchestrator(ctx, workID); err != nil {
		logging.Warn("failed to cleanup stale orchestrator", "workID", workID, "error", err)
		// Continue anyway - the registration will fail if there's a real conflict
	}

	m.id = uuid.New().String()
	m.procType = db.ProcessTypeOrchestrator
	m.workID = &workID

	if err := m.db.RegisterProcess(ctx, m.id, m.procType, &workID, os.Getpid()); err != nil {
		return fmt.Errorf("failed to register orchestrator: %w", err)
	}

	m.startHeartbeat()
	logging.Info("registered orchestrator", "id", m.id, "pid", os.Getpid(), "workID", workID)
	return nil
}

// startHeartbeat starts the background heartbeat goroutine.
func (m *Manager) startHeartbeat() {
	m.running = true
	m.stopCh = make(chan struct{})
	m.stoppedCh = make(chan struct{})
	m.triggerCh = make(chan chan error)

	go func() {
		defer close(m.stoppedCh)
		ticker := time.NewTicker(m.heartbeat)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopCh:
				return
			case <-ticker.C:
				if err := m.db.UpdateHeartbeatWithTime(context.Background(), m.id, m.nowFunc()); err != nil {
					logging.Warn("failed to update heartbeat", "id", m.id, "error", err)
				}
			case resultCh := <-m.triggerCh:
				err := m.db.UpdateHeartbeatWithTime(context.Background(), m.id, m.nowFunc())
				resultCh <- err
			}
		}
	}()
}

// TriggerHeartbeat forces an immediate heartbeat update and waits for it to complete.
// This is primarily for testing purposes to avoid time-based sleeps.
func (m *Manager) TriggerHeartbeat() error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return fmt.Errorf("manager not running")
	}
	triggerCh := m.triggerCh
	m.mu.Unlock()

	resultCh := make(chan error, 1)
	triggerCh <- resultCh
	return <-resultCh
}

// Stop gracefully shuts down the manager and unregisters the process.
func (m *Manager) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	// Wait for the heartbeat goroutine to finish
	<-m.stoppedCh

	// Unregister the process
	if err := m.db.UnregisterProcess(context.Background(), m.id); err != nil {
		logging.Warn("failed to unregister process", "id", m.id, "error", err)
	} else {
		logging.Info("unregistered process", "id", m.id)
	}
}

// IsOrchestratorAlive checks if an orchestrator for the given work ID has a recent heartbeat.
func (m *Manager) IsOrchestratorAlive(ctx context.Context, workID string) (bool, error) {
	return m.db.IsOrchestratorAlive(ctx, workID, db.DefaultStalenessThreshold)
}

// IsControlPlaneAlive checks if the control plane has a recent heartbeat.
func (m *Manager) IsControlPlaneAlive(ctx context.Context) (bool, error) {
	return m.db.IsControlPlaneAlive(ctx, db.DefaultStalenessThreshold)
}

// CleanupStaleProcessRecords removes database records for processes with stale heartbeats.
// This should be called periodically by a cleanup routine.
func (m *Manager) CleanupStaleProcessRecords(ctx context.Context) error {
	staleProcs, err := m.db.GetStaleProcesses(ctx, db.DefaultStalenessThreshold)
	if err != nil {
		return fmt.Errorf("failed to get stale processes: %w", err)
	}

	for _, p := range staleProcs {
		logging.Info("cleaning up stale process",
			"id", p.ID,
			"type", p.ProcessType,
			"workID", p.WorkID,
			"pid", p.PID,
			"lastHeartbeat", p.Heartbeat)
	}

	return m.db.CleanupStaleProcesses(ctx, db.DefaultStalenessThreshold)
}

// GetOrchestratorProcess retrieves the orchestrator process for a work ID.
func (m *Manager) GetOrchestratorProcess(ctx context.Context, workID string) (*db.Process, error) {
	return m.db.GetOrchestratorProcess(ctx, workID)
}

// GetControlPlaneProcess retrieves the control plane process.
func (m *Manager) GetControlPlaneProcess(ctx context.Context) (*db.Process, error) {
	return m.db.GetControlPlaneProcess(ctx)
}

// GetAllProcesses retrieves all registered processes.
func (m *Manager) GetAllProcesses(ctx context.Context) ([]*db.Process, error) {
	return m.db.GetAllProcesses(ctx)
}
