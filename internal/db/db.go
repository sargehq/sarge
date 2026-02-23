package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/sargehq/sarge/internal/db/sqlc"
)

// Status constants for bean tracking.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusIdle       = "idle"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
	StatusMerged     = "merged"
)

// PR state constants
const (
	PRStateOpen   = "open"
	PRStateClosed = "closed"
	PRStateMerged = "merged"
)

// CI status constants
const (
	CIStatusPending = "pending"
	CIStatusSuccess = "success"
	CIStatusFailure = "failure"
)

// Approval status constants
const (
	ApprovalStatusPending          = "pending"
	ApprovalStatusApproved         = "approved"
	ApprovalStatusChangesRequested = "changes_requested"
)

// Mergeable state constants (from GitHub API mergeStateStatus)
const (
	MergeableStateClean    = "CLEAN"    // Ready to merge
	MergeableStateDirty    = "DIRTY"    // Has conflicts
	MergeableStateBlocked  = "BLOCKED"  // Blocked by checks
	MergeableStateBehind   = "BEHIND"   // Behind base branch
	MergeableStateDraft    = "DRAFT"    // Draft PR
	MergeableStateUnstable = "UNSTABLE" // CI unstable
	MergeableStateUnknown  = "UNKNOWN"  // Unknown state
)

// DB wraps the SQLite database connection and sqlc queries.
type DB struct {
	*sql.DB
	queries *sqlc.Queries
}

// OpenPath initializes the database at the specified path and runs migrations.
func OpenPath(ctx context.Context, dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations instead of creating schema directly
	if err := RunMigrations(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{
		DB:      db,
		queries: sqlc.New(db),
	}, nil
}
