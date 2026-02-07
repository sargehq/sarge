package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// Process types
const (
	ProcessTypeControlPlane = "control_plane"
	ProcessTypeOrchestrator = "orchestrator"
)

// Default intervals for heartbeat monitoring
const (
	DefaultHeartbeatInterval   = 10 * time.Second
	DefaultStalenessThreshold  = 30 * time.Second
)

// Process represents a running process (orchestrator or control plane).
type Process struct {
	ID          string
	ProcessType string
	WorkID      *string
	PID         int
	Hostname    string
	Heartbeat   time.Time
	StartedAt   time.Time
	PprofPort   *int
}

// RegisterProcess registers or updates a process in the database.
func (db *DB) RegisterProcess(ctx context.Context, id, processType string, workID *string, pid int) error {
	hostname, _ := os.Hostname()

	var workIDParam sql.NullString
	if workID != nil {
		workIDParam = sql.NullString{String: *workID, Valid: true}
	}

	err := db.queries.RegisterProcess(ctx, sqlc.RegisterProcessParams{
		ID:          id,
		ProcessType: processType,
		WorkID:      workIDParam,
		Pid:         int64(pid),
		Hostname:    hostname,
	})
	if err != nil {
		return fmt.Errorf("failed to register process: %w", err)
	}
	return nil
}

// UpdateHeartbeat updates the heartbeat timestamp for a process.
// Returns the number of rows affected (0 if the process record was deleted).
func (db *DB) UpdateHeartbeat(ctx context.Context, id string) (int64, error) {
	result, err := db.queries.UpdateHeartbeat(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("failed to update heartbeat: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rows, nil
}

// UpdateHeartbeatWithTime updates the heartbeat timestamp for a process with an explicit time.
// Returns the number of rows affected (0 if the process record was deleted).
// This is useful for testing where time needs to be controlled.
func (db *DB) UpdateHeartbeatWithTime(ctx context.Context, id string, t time.Time) (int64, error) {
	result, err := db.queries.UpdateHeartbeatWithTime(ctx, sqlc.UpdateHeartbeatWithTimeParams{
		Heartbeat: t,
		ID:        id,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to update heartbeat: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rows, nil
}

// IsOrchestratorAlive checks if an orchestrator for the given work ID has a recent heartbeat.
func (db *DB) IsOrchestratorAlive(ctx context.Context, workID string, threshold time.Duration) (bool, error) {
	// Convert threshold to negative seconds for SQL datetime comparison
	thresholdSeconds := fmt.Sprintf("-%d", int(threshold.Seconds()))

	alive, err := db.queries.IsOrchestratorAlive(ctx, sqlc.IsOrchestratorAliveParams{
		WorkID:  sql.NullString{String: workID, Valid: true},
		Column2: sql.NullString{String: thresholdSeconds, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("failed to check orchestrator status: %w", err)
	}
	return alive == 1, nil
}

// IsControlPlaneAlive checks if the control plane has a recent heartbeat.
func (db *DB) IsControlPlaneAlive(ctx context.Context, threshold time.Duration) (bool, error) {
	// Convert threshold to negative seconds for SQL datetime comparison
	thresholdSeconds := fmt.Sprintf("-%d", int(threshold.Seconds()))

	alive, err := db.queries.IsControlPlaneAlive(ctx, sql.NullString{String: thresholdSeconds, Valid: true})
	if err != nil {
		return false, fmt.Errorf("failed to check control plane status: %w", err)
	}
	return alive == 1, nil
}

// GetStaleProcesses returns all processes with heartbeats older than the threshold.
func (db *DB) GetStaleProcesses(ctx context.Context, threshold time.Duration) ([]*Process, error) {
	// Convert threshold to negative seconds for SQL datetime comparison
	thresholdSeconds := fmt.Sprintf("-%d", int(threshold.Seconds()))

	rows, err := db.queries.GetStaleProcesses(ctx, sql.NullString{String: thresholdSeconds, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to get stale processes: %w", err)
	}

	result := make([]*Process, len(rows))
	for i, r := range rows {
		result[i] = processFromSqlc(&r)
	}
	return result, nil
}

// CleanupStaleProcesses removes processes with heartbeats older than the threshold.
func (db *DB) CleanupStaleProcesses(ctx context.Context, threshold time.Duration) error {
	// Convert threshold to negative seconds for SQL datetime comparison
	thresholdSeconds := fmt.Sprintf("-%d", int(threshold.Seconds()))

	err := db.queries.DeleteStaleProcesses(ctx, sql.NullString{String: thresholdSeconds, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to cleanup stale processes: %w", err)
	}
	return nil
}

// UnregisterProcess removes a process from the database.
func (db *DB) UnregisterProcess(ctx context.Context, id string) error {
	err := db.queries.DeleteProcess(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to unregister process: %w", err)
	}
	return nil
}

// DeleteControlPlaneProcess unconditionally deletes any existing control plane record.
// This is used before registering a new control plane to force-take the registration.
func (db *DB) DeleteControlPlaneProcess(ctx context.Context) error {
	return db.queries.DeleteControlPlaneProcess(ctx)
}

// DeleteOrchestratorByWorkID unconditionally deletes any existing orchestrator record for a work ID.
// This is used before registering a new orchestrator to force-take the registration.
func (db *DB) DeleteOrchestratorByWorkID(ctx context.Context, workID string) error {
	return db.queries.DeleteOrchestratorByWorkID(ctx, sql.NullString{String: workID, Valid: true})
}

// GetOrchestratorProcess retrieves the orchestrator process for a work ID.
func (db *DB) GetOrchestratorProcess(ctx context.Context, workID string) (*Process, error) {
	row, err := db.queries.GetOrchestratorProcess(ctx, sql.NullString{String: workID, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get orchestrator process: %w", err)
	}
	return processFromSqlc(&row), nil
}

// GetControlPlaneProcess retrieves the control plane process.
func (db *DB) GetControlPlaneProcess(ctx context.Context) (*Process, error) {
	row, err := db.queries.GetControlPlaneProcess(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get control plane process: %w", err)
	}
	return processFromSqlc(&row), nil
}

// GetAllProcesses retrieves all registered processes.
func (db *DB) GetAllProcesses(ctx context.Context) ([]*Process, error) {
	rows, err := db.queries.GetAllProcesses(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all processes: %w", err)
	}

	result := make([]*Process, len(rows))
	for i, r := range rows {
		result[i] = processFromSqlc(&r)
	}
	return result, nil
}

// UpdatePprofPort sets the pprof port for a process. Pass nil to clear.
func (db *DB) UpdatePprofPort(ctx context.Context, id string, port *int) error {
	var portParam sql.NullInt64
	if port != nil {
		portParam = sql.NullInt64{Int64: int64(*port), Valid: true}
	}

	err := db.queries.UpdatePprofPort(ctx, sqlc.UpdatePprofPortParams{
		ID:        id,
		PprofPort: portParam,
	})
	if err != nil {
		return fmt.Errorf("failed to update pprof port: %w", err)
	}
	return nil
}

// processFromSqlc converts a SQLC Process to a local Process.
func processFromSqlc(p *sqlc.Process) *Process {
	proc := &Process{
		ID:          p.ID,
		ProcessType: p.ProcessType,
		PID:         int(p.Pid),
		Hostname:    p.Hostname,
		Heartbeat:   p.Heartbeat,
		StartedAt:   p.StartedAt,
	}
	if p.WorkID.Valid {
		proc.WorkID = &p.WorkID.String
	}
	if p.PprofPort.Valid {
		port := int(p.PprofPort.Int64)
		proc.PprofPort = &port
	}
	return proc
}
