package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// nullTime converts a time to sql.NullTime for nullable timestamp fields.
func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// beadToTracked converts an sqlc.Bead to TrackedBead
func beadToTracked(b *sqlc.Bead) *TrackedBead {
	tracked := &TrackedBead{
		ID:            b.ID,
		Status:        b.Status,
		Title:         b.Title,
		PRURL:         b.PrUrl,
		ErrorMessage:  b.ErrorMessage,
		ZellijSession: b.ZellijSession,
		ZellijPane:    b.ZellijPane,
		WorktreePath:  b.WorktreePath,
		CreatedAt:     b.CreatedAt,
		UpdatedAt:     b.UpdatedAt,
	}
	if b.StartedAt.Valid {
		tracked.StartedAt = &b.StartedAt.Time
	}
	if b.CompletedAt.Valid {
		tracked.CompletedAt = &b.CompletedAt.Time
	}
	return tracked
}

// TrackedBead represents a bead tracking record in the database.
type TrackedBead struct {
	ID            string
	Status        string
	Title         string
	PRURL         string
	ErrorMessage  string
	ZellijSession string
	ZellijPane    string
	WorktreePath  string
	StartedAt     *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// StartBead marks a bead as processing with session info.
func (db *DB) StartBead(ctx context.Context, id, title, zellijSession, zellijPane string) error {
	return db.StartBeadWithWorktree(ctx, id, title, zellijSession, zellijPane, "")
}

// StartBeadWithWorktree marks a bead as processing with session and worktree info.
func (db *DB) StartBeadWithWorktree(ctx context.Context, id, title, zellijSession, zellijPane, worktreePath string) error {
	now := time.Now()
	err := db.queries.StartBead(ctx, sqlc.StartBeadParams{
		ID:            id,
		Title:         title,
		ZellijSession: zellijSession,
		ZellijPane:    zellijPane,
		WorktreePath:  worktreePath,
		StartedAt:     nullTime(now),
		UpdatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("failed to start bead %s: %w", id, err)
	}
	return nil
}

// CompleteBead marks a bead as completed with a PR URL.
func (db *DB) CompleteBead(ctx context.Context, id, prURL string) error {
	now := time.Now()
	rows, err := db.queries.CompleteBead(ctx, sqlc.CompleteBeadParams{
		PrUrl:       prURL,
		CompletedAt: nullTime(now),
		UpdatedAt:   now,
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to complete bead %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("bead %s not found", id)
	}
	return nil
}

// FailBead marks a bead as failed with an error message.
func (db *DB) FailBead(ctx context.Context, id, errMsg string) error {
	now := time.Now()
	rows, err := db.queries.FailBead(ctx, sqlc.FailBeadParams{
		ErrorMessage: errMsg,
		CompletedAt:  nullTime(now),
		UpdatedAt:    now,
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to mark bead %s as failed: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("bead %s not found", id)
	}
	return nil
}

// GetBead retrieves a tracking record by ID.
func (db *DB) GetBead(ctx context.Context, id string) (*TrackedBead, error) {
	bead, err := db.queries.GetBead(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bead: %w", err)
	}
	return beadToTracked(&bead), nil
}

// IsCompleted checks if a bead is completed or failed.
func (db *DB) IsCompleted(ctx context.Context, id string) (bool, error) {
	status, err := db.queries.GetBeadStatus(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check bead status: %w", err)
	}
	return status == StatusCompleted || status == StatusFailed, nil
}

// ListBeads returns all beads, optionally filtered by status.
func (db *DB) ListBeads(ctx context.Context, statusFilter string) ([]*TrackedBead, error) {
	var beads []sqlc.Bead
	var err error

	if statusFilter == "" {
		beads, err = db.queries.ListBeads(ctx)
	} else {
		beads, err = db.queries.ListBeadsByStatus(ctx, statusFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list beads: %w", err)
	}

	var trackedBeads []*TrackedBead
	for i := range beads {
		trackedBeads = append(trackedBeads, beadToTracked(&beads[i]))
	}
	return trackedBeads, nil
}
