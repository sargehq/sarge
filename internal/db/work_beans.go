package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// WorkBean represents a bean assigned to a work.
type WorkBean struct {
	WorkID    string
	BeanID    string
	Position  int64
	CreatedAt time.Time
}

// workBeanToLocal converts an sqlc.WorkBean to local WorkBean.
func workBeanToLocal(wb *sqlc.WorkBean) *WorkBean {
	return &WorkBean{
		WorkID:    wb.WorkID,
		BeanID:    wb.BeanID,
		Position:  wb.Position,
		CreatedAt: wb.CreatedAt,
	}
}

// AddWorkBean adds a bean to a work with the specified position.
func (db *DB) AddWorkBean(ctx context.Context, workID, beanID string, position int64) error {
	err := db.queries.AddWorkBean(ctx, sqlc.AddWorkBeanParams{
		WorkID:   workID,
		BeanID:   beanID,
		Position: position,
	})
	if err != nil {
		return fmt.Errorf("failed to add bean %s to work %s: %w", beanID, workID, err)
	}
	return nil
}

// AddWorkBeans adds multiple beans to a work.
// Beans are positioned sequentially starting from the next available position.
// Returns an error if any bean already exists in the work.
func (db *DB) AddWorkBeans(ctx context.Context, workID string, beanIDs []string) error {
	if len(beanIDs) == 0 {
		return nil
	}

	// Check for existing beans before adding
	existingBeans, err := db.queries.GetWorkBeans(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to check existing beans: %w", err)
	}

	existingSet := make(map[string]bool)
	for _, b := range existingBeans {
		existingSet[b.BeanID] = true
	}

	// Check for duplicates
	var duplicates []string
	for _, beanID := range beanIDs {
		if existingSet[beanID] {
			duplicates = append(duplicates, beanID)
		}
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("beans already exist in work %s: %s", workID, strings.Join(duplicates, ", "))
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Get current max position
	maxPos, err := qtx.GetMaxWorkBeanPosition(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get max position: %w", err)
	}

	position := maxPos + 1
	for _, beanID := range beanIDs {
		err := qtx.AddWorkBean(ctx, sqlc.AddWorkBeanParams{
			WorkID:   workID,
			BeanID:   beanID,
			Position: position,
		})
		if err != nil {
			return fmt.Errorf("failed to add bean %s: %w", beanID, err)
		}
		position++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// RemoveWorkBean removes a bean from a work.
func (db *DB) RemoveWorkBean(ctx context.Context, workID, beanID string) error {
	rows, err := db.queries.RemoveWorkBean(ctx, sqlc.RemoveWorkBeanParams{
		WorkID: workID,
		BeanID: beanID,
	})
	if err != nil {
		return fmt.Errorf("failed to remove bean %s from work %s: %w", beanID, workID, err)
	}
	if rows == 0 {
		return fmt.Errorf("bean %s not found in work %s", beanID, workID)
	}
	return nil
}

// GetWorkBeans returns all beans assigned to a work.
func (db *DB) GetWorkBeans(ctx context.Context, workID string) ([]*WorkBean, error) {
	beans, err := db.queries.GetWorkBeans(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work beans: %w", err)
	}

	result := make([]*WorkBean, len(beans))
	for i, b := range beans {
		result[i] = workBeanToLocal(&b)
	}
	return result, nil
}

// GetUnassignedWorkBeans returns beans in a work that are not yet in any task.
func (db *DB) GetUnassignedWorkBeans(ctx context.Context, workID string) ([]*WorkBean, error) {
	beans, err := db.queries.GetUnassignedWorkBeans(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unassigned work beans: %w", err)
	}

	result := make([]*WorkBean, len(beans))
	for i, b := range beans {
		result[i] = workBeanToLocal(&b)
	}
	return result, nil
}

// IsBeanInTask checks if a bean is already assigned to a task in the work.
func (db *DB) IsBeanInTask(ctx context.Context, workID, beanID string) (bool, error) {
	inTask, err := db.queries.IsBeanInTask(ctx, sqlc.IsBeanInTaskParams{
		WorkID: workID,
		BeanID: beanID,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check if bean in task: %w", err)
	}
	return inTask, nil
}

// DeleteWorkBeans removes all beans from a work.
func (db *DB) DeleteWorkBeans(ctx context.Context, workID string) error {
	_, err := db.queries.DeleteWorkBeans(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to delete work beans: %w", err)
	}
	return nil
}

// GetAllAssignedBeans returns a map of bean IDs to work IDs for all beans
// that are assigned to any work. This is used by plan mode to show which
// beans are already assigned.
func (db *DB) GetAllAssignedBeans(ctx context.Context) (map[string]string, error) {
	rows, err := db.queries.GetAllAssignedBeans(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get assigned beans: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		result[row.BeanID] = row.WorkID
	}
	return result, nil
}
