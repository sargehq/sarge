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

// beanToTracked converts an sqlc.Bean to TrackedBean
func beanToTracked(b *sqlc.Bean) *TrackedBean {
	tracked := &TrackedBean{
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

// TrackedBean represents a bean tracking record in the database.
type TrackedBean struct {
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

// StartBean marks a bean as processing with session info.
func (db *DB) StartBean(ctx context.Context, id, title, zellijSession, zellijPane string) error {
	return db.StartBeanWithWorktree(ctx, id, title, zellijSession, zellijPane, "")
}

// StartBeanWithWorktree marks a bean as processing with session and worktree info.
func (db *DB) StartBeanWithWorktree(ctx context.Context, id, title, zellijSession, zellijPane, worktreePath string) error {
	now := time.Now()
	err := db.queries.StartBean(ctx, sqlc.StartBeanParams{
		ID:            id,
		Title:         title,
		ZellijSession: zellijSession,
		ZellijPane:    zellijPane,
		WorktreePath:  worktreePath,
		StartedAt:     nullTime(now),
		UpdatedAt:     now,
	})
	if err != nil {
		return fmt.Errorf("failed to start bean %s: %w", id, err)
	}
	return nil
}

// CompleteBean marks a bean as completed with a PR URL.
func (db *DB) CompleteBean(ctx context.Context, id, prURL string) error {
	now := time.Now()
	rows, err := db.queries.CompleteBean(ctx, sqlc.CompleteBeanParams{
		PrUrl:       prURL,
		CompletedAt: nullTime(now),
		UpdatedAt:   now,
		ID:          id,
	})
	if err != nil {
		return fmt.Errorf("failed to complete bean %s: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("bean %s not found", id)
	}
	return nil
}

// FailBean marks a bean as failed with an error message.
func (db *DB) FailBean(ctx context.Context, id, errMsg string) error {
	now := time.Now()
	rows, err := db.queries.FailBean(ctx, sqlc.FailBeanParams{
		ErrorMessage: errMsg,
		CompletedAt:  nullTime(now),
		UpdatedAt:    now,
		ID:           id,
	})
	if err != nil {
		return fmt.Errorf("failed to mark bean %s as failed: %w", id, err)
	}
	if rows == 0 {
		return fmt.Errorf("bean %s not found", id)
	}
	return nil
}

// GetBean retrieves a tracking record by ID.
func (db *DB) GetBean(ctx context.Context, id string) (*TrackedBean, error) {
	bean, err := db.queries.GetBean(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get bean: %w", err)
	}
	return beanToTracked(&bean), nil
}

// IsCompleted checks if a bean is completed or failed.
func (db *DB) IsCompleted(ctx context.Context, id string) (bool, error) {
	status, err := db.queries.GetBeanStatus(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check bean status: %w", err)
	}
	return status == StatusCompleted || status == StatusFailed, nil
}

// ListBeans returns all beans, optionally filtered by status.
func (db *DB) ListBeans(ctx context.Context, statusFilter string) ([]*TrackedBean, error) {
	var beans []sqlc.Bean
	var err error

	if statusFilter == "" {
		beans, err = db.queries.ListBeans(ctx)
	} else {
		beans, err = db.queries.ListBeansByStatus(ctx, statusFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list beans: %w", err)
	}

	var trackedBeans []*TrackedBean
	for i := range beans {
		trackedBeans = append(trackedBeans, beanToTracked(&beans[i]))
	}
	return trackedBeans, nil
}
