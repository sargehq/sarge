package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// WorkBead represents a bead assigned to a work.
type WorkBead struct {
	WorkID    string
	BeadID    string
	Position  int64
	CreatedAt time.Time
}

// workBeadToLocal converts an sqlc.WorkBead to local WorkBead.
func workBeadToLocal(wb *sqlc.WorkBead) *WorkBead {
	return &WorkBead{
		WorkID:    wb.WorkID,
		BeadID:    wb.BeadID,
		Position:  wb.Position,
		CreatedAt: wb.CreatedAt,
	}
}

// AddWorkBead adds a bead to a work with the specified position.
func (db *DB) AddWorkBead(ctx context.Context, workID, beadID string, position int64) error {
	err := db.queries.AddWorkBead(ctx, sqlc.AddWorkBeadParams{
		WorkID:   workID,
		BeadID:   beadID,
		Position: position,
	})
	if err != nil {
		return fmt.Errorf("failed to add bead %s to work %s: %w", beadID, workID, err)
	}
	return nil
}

// AddWorkBeads adds multiple beads to a work.
// Beads are positioned sequentially starting from the next available position.
// Returns an error if any bead already exists in the work.
func (db *DB) AddWorkBeads(ctx context.Context, workID string, beadIDs []string) error {
	if len(beadIDs) == 0 {
		return nil
	}

	// Check for existing beads before adding
	existingBeads, err := db.queries.GetWorkBeads(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to check existing beads: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, b := range existingBeads {
		existingSet[b.BeadID] = true
	}

	// Check for duplicates
	var duplicates []string
	for _, beadID := range beadIDs {
		if existingSet[beadID] {
			duplicates = append(duplicates, beadID)
		}
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("beads already exist in work %s: %s", workID, strings.Join(duplicates, ", "))
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Get current max position
	maxPos, err := qtx.GetMaxWorkBeadPosition(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get max position: %w", err)
	}

	position := maxPos + 1
	for _, beadID := range beadIDs {
		err := qtx.AddWorkBead(ctx, sqlc.AddWorkBeadParams{
			WorkID:   workID,
			BeadID:   beadID,
			Position: position,
		})
		if err != nil {
			return fmt.Errorf("failed to add bead %s: %w", beadID, err)
		}
		position++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// RemoveWorkBead removes a bead from a work.
func (db *DB) RemoveWorkBead(ctx context.Context, workID, beadID string) error {
	rows, err := db.queries.RemoveWorkBead(ctx, sqlc.RemoveWorkBeadParams{
		WorkID: workID,
		BeadID: beadID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove bead %s from work %s: %w", beadID, workID, err)
	}
	if rows == 0 {
		return fmt.Errorf("bead %s not found in work %s", beadID, workID)
	}
	return nil
}

// GetWorkBeads returns all beads assigned to a work.
func (db *DB) GetWorkBeads(ctx context.Context, workID string) ([]*WorkBead, error) {
	beads, err := db.queries.GetWorkBeads(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work beads: %w", err)
	}

	result := make([]*WorkBead, len(beads))
	for i, b := range beads {
		result[i] = workBeadToLocal(&b)
	}
	return result, nil
}

// GetUnassignedWorkBeads returns beads in a work that are not yet in any task.
func (db *DB) GetUnassignedWorkBeads(ctx context.Context, workID string) ([]*WorkBead, error) {
	beads, err := db.queries.GetUnassignedWorkBeads(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unassigned work beads: %w", err)
	}

	result := make([]*WorkBead, len(beads))
	for i, b := range beads {
		result[i] = workBeadToLocal(&b)
	}
	return result, nil
}

// IsBeadInTask checks if a bead is already assigned to a task in the work.
func (db *DB) IsBeadInTask(ctx context.Context, workID, beadID string) (bool, error) {
	inTask, err := db.queries.IsBeadInTask(ctx, sqlc.IsBeadInTaskParams{
		WorkID: workID,
		BeadID: beadID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check if bead in task: %w", err)
	}
	return inTask, nil
}

// DeleteWorkBeads removes all beads from a work.
func (db *DB) DeleteWorkBeads(ctx context.Context, workID string) error {
	_, err := db.queries.DeleteWorkBeads(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to delete work beads: %w", err)
	}
	return nil
}

// GetAllAssignedBeads returns a map of bead IDs to work IDs for all beads
// that are assigned to any work. This is used by plan mode to show which
// beads are already assigned.
func (db *DB) GetAllAssignedBeads(ctx context.Context) (map[string]string, error) {
	rows, err := db.queries.GetAllAssignedBeads(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get assigned beads: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.BeadID] = row.WorkID
	}
	return result, nil
}
