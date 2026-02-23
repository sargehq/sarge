package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// PlanSession represents a running plan mode Claude session for a specific bean.
type PlanSession struct {
	BeanID        string
	ZellijSession string
	TabName       string
	PID           int
	StartedAt     time.Time
}

// TabNameForBean returns the zellij tab name for a bean's planning session.
func TabNameForBean(beanID string) string {
	return fmt.Sprintf("plan-%s", beanID)
}

// RegisterPlanSession registers a plan session for a specific bean.
// It also cleans up any stale sessions (where the process is no longer running).
func (db *DB) RegisterPlanSession(ctx context.Context, beanID, zellijSession, tabName string, pid int) error {
	// First, clean up stale sessions
	if err := db.CleanupStalePlanSessions(ctx); err != nil {
		return err
	}

	_, err := db.ExecContext(ctx, `
		INSERT OR REPLACE INTO plan_sessions (bean_id, zellij_session, tab_name, pid, started_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, beanID, zellijSession, tabName, pid)
	return err
}

// UnregisterPlanSession removes a plan session by bean ID.
func (db *DB) UnregisterPlanSession(ctx context.Context, beanID string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM plan_sessions WHERE bean_id = ?
	`, beanID)
	return err
}

// GetPlanSession gets the plan session for a specific bean.
// Returns nil if no session is registered.
func (db *DB) GetPlanSession(ctx context.Context, beanID string) (*PlanSession, error) {
	row := db.QueryRowContext(ctx, `
		SELECT bean_id, zellij_session, tab_name, pid, started_at
		FROM plan_sessions
		WHERE bean_id = ?
	`, beanID)

	var ps PlanSession
	err := row.Scan(&ps.BeanID, &ps.ZellijSession, &ps.TabName, &ps.PID, &ps.StartedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ps, nil
}

// IsPlanSessionRunning checks if a plan session is running for the given bean.
// It also validates that the registered process is still alive.
func (db *DB) IsPlanSessionRunning(ctx context.Context, beanID string) (bool, error) {
	ps, err := db.GetPlanSession(ctx, beanID)
	if err != nil {
		return false, err
	}
	if ps == nil {
		return false, nil
	}

	// Check if the process is still alive
	if !isProcessAlive(ps.PID) {
		// Process is dead, clean up the stale registration
		_ = db.UnregisterPlanSession(ctx, beanID)
		return false, nil
	}

	return true, nil
}

// ListPlanSessions returns all plan sessions for a zellij session.
func (db *DB) ListPlanSessions(ctx context.Context, zellijSession string) ([]*PlanSession, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT bean_id, zellij_session, tab_name, pid, started_at
		FROM plan_sessions
		WHERE zellij_session = ?
	`, zellijSession)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*PlanSession
	for rows.Next() {
		var ps PlanSession
		if err := rows.Scan(&ps.BeanID, &ps.ZellijSession, &ps.TabName, &ps.PID, &ps.StartedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, &ps)
	}

	return sessions, rows.Err()
}

// GetBeansWithActiveSessions returns a map of bean IDs that have active planning sessions.
// It validates that processes are still alive and cleans up stale sessions.
func (db *DB) GetBeansWithActiveSessions(ctx context.Context, zellijSession string) (map[string]bool, error) {
	sessions, err := db.ListPlanSessions(ctx, zellijSession)
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, ps := range sessions {
		if isProcessAlive(ps.PID) {
			result[ps.BeanID] = true
		} else {
			// Clean up stale session
			_ = db.UnregisterPlanSession(ctx, ps.BeanID)
		}
	}

	return result, nil
}

// CleanupStalePlanSessions removes registrations for processes that are no longer running.
func (db *DB) CleanupStalePlanSessions(ctx context.Context) error {
	rows, err := db.QueryContext(ctx, `SELECT bean_id, pid FROM plan_sessions`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var stale []string
	for rows.Next() {
		var beanID string
		var pid int
		if err := rows.Scan(&beanID, &pid); err != nil {
			return err
		}
		if !isProcessAlive(pid) {
			stale = append(stale, beanID)
		}
	}

	for _, beanID := range stale {
		if err := db.UnregisterPlanSession(ctx, beanID); err != nil {
			return err
		}
	}

	return nil
}

// isProcessAlive checks if a process with the given PID is still running.
func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. We need to send signal 0 to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}
