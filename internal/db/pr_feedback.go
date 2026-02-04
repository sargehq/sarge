package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sargehq/sarge/internal/db/sqlc"
	"github.com/sargehq/sarge/internal/github"
)

// convertPrFeedback converts a sqlc.PrFeedback to our PRFeedback struct.
// It handles: nullable fields, metadata JSON parsing, and field name mapping.
func convertPrFeedback(f sqlc.PrFeedback) PRFeedback {
	result := PRFeedback{
		ID:           f.ID,
		WorkID:       f.WorkID,
		PRURL:        f.PrUrl,
		FeedbackType: f.FeedbackType,
		Title:        f.Title,
		Description:  f.Description,
		SourceURL:    f.SourceUrl.String,
		Priority:     int(f.Priority),
		CreatedAt:    f.CreatedAt,
	}

	if f.SourceID.Valid {
		result.SourceID = &f.SourceID.String
	}
	if f.BeadID.Valid {
		result.BeadID = &f.BeadID.String
	}
	if f.ProcessedAt.Valid {
		result.ProcessedAt = &f.ProcessedAt.Time
	}
	if f.ResolvedAt.Valid {
		result.ResolvedAt = &f.ResolvedAt.Time
	}

	// Parse new structured source fields
	if f.SourceType.Valid {
		result.SourceType = github.SourceType(f.SourceType.String)
	}
	if f.SourceName.Valid {
		result.SourceName = f.SourceName.String
	}

	// Parse context JSON
	if f.Context.Valid && f.Context.String != "" && f.Context.String != "{}" {
		var ctx github.FeedbackContext
		if err := json.Unmarshal([]byte(f.Context.String), &ctx); err == nil {
			result.Context = &ctx
		}
	}

	return result
}

// PRFeedback represents a piece of feedback from a PR.
type PRFeedback struct {
	ID           string
	WorkID       string
	PRURL        string
	FeedbackType string
	Title        string
	Description  string
	SourceURL    string                  // URL to the source item
	SourceID     *string                 // GitHub comment/check ID for resolution tracking
	SourceType   github.SourceType       // Structured type: ci, workflow, review_comment, issue_comment
	SourceName   string                  // Human-readable name (check name, workflow name, reviewer)
	Context      *github.FeedbackContext // Structured context data
	Priority     int
	BeadID       *string
	CreatedAt    time.Time
	ProcessedAt  *time.Time
	ResolvedAt   *time.Time // When the GitHub comment was resolved
}

// IsReviewComment returns true if this feedback is from a GitHub review comment
// that can be replied to and resolved.
func (f *PRFeedback) IsReviewComment() bool {
	return f.SourceType == github.SourceTypeReviewComment && f.SourceID != nil && *f.SourceID != ""
}

// CreatePRFeedbackParams holds parameters for creating a PR feedback record.
type CreatePRFeedbackParams struct {
	WorkID       string
	PRURL        string
	FeedbackType string
	Title        string
	Description  string
	Source       github.SourceInfo       // Structured source info
	Context      *github.FeedbackContext // Structured context (optional)
	Priority     int
}

// CreatePRFeedback creates a new PR feedback record with structured source info.
func (db *DB) CreatePRFeedbackFromParams(ctx context.Context, params CreatePRFeedbackParams) (*PRFeedback, error) {
	id := uuid.New().String()

	// Convert context to JSON
	var contextJSON sql.NullString
	if params.Context != nil {
		data, err := json.Marshal(params.Context)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal context: %w", err)
		}
		contextJSON = sql.NullString{String: string(data), Valid: true}
	}

	err := db.queries.CreatePRFeedback(ctx, sqlc.CreatePRFeedbackParams{
		ID:           id,
		WorkID:       params.WorkID,
		PrUrl:        params.PRURL,
		FeedbackType: params.FeedbackType,
		Title:        params.Title,
		Description:  params.Description,
		SourceUrl:    sql.NullString{String: params.Source.URL, Valid: params.Source.URL != ""},
		SourceID:     sql.NullString{String: params.Source.ID, Valid: params.Source.ID != ""},
		Priority:     int64(params.Priority),
		SourceType:   sql.NullString{String: string(params.Source.Type), Valid: params.Source.Type != ""},
		SourceName:   sql.NullString{String: params.Source.Name, Valid: params.Source.Name != ""},
		Context:      contextJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PR feedback: %w", err)
	}

	sourceID := params.Source.ID
	result := &PRFeedback{
		ID:           id,
		WorkID:       params.WorkID,
		PRURL:        params.PRURL,
		FeedbackType: params.FeedbackType,
		Title:        params.Title,
		Description:  params.Description,
		SourceURL:    params.Source.URL,
		SourceID:     &sourceID,
		SourceType:   params.Source.Type,
		SourceName:   params.Source.Name,
		Context:      params.Context,
		Priority:     params.Priority,
		CreatedAt:    time.Now(),
	}
	return result, nil
}

// GetUnprocessedFeedback returns all unprocessed feedback for a work.
func (db *DB) GetUnprocessedFeedback(ctx context.Context, workID string) ([]PRFeedback, error) {
	feedbacks, err := db.queries.ListUnprocessedPRFeedback(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to query unprocessed feedback: %w", err)
	}

	result := make([]PRFeedback, len(feedbacks))
	for i, f := range feedbacks {
		result[i] = convertPrFeedback(f)
	}

	return result, nil
}

// MarkFeedbackProcessed marks feedback as processed and associates it with a bead.
func (db *DB) MarkFeedbackProcessed(ctx context.Context, feedbackID, beadID string) error {
	err := db.queries.MarkPRFeedbackProcessed(ctx, sqlc.MarkPRFeedbackProcessedParams{
		BeadID: sql.NullString{String: beadID, Valid: beadID != ""},
		ID:     feedbackID,
	})
	if err != nil {
		return fmt.Errorf("failed to mark feedback as processed: %w", err)
	}
	return nil
}

// CountUnresolvedFeedbackForWork returns the count of PR feedback items that have beads
// which are not yet assigned to any task and not resolved/closed.
func (db *DB) CountUnresolvedFeedbackForWork(ctx context.Context, workID string) (int, error) {
	count, err := db.queries.CountUnassignedFeedbackForWork(ctx, workID)
	if err != nil {
		return 0, fmt.Errorf("failed to count unresolved feedback: %w", err)
	}
	return int(count), nil
}

// GetUnassignedFeedbackBeadIDs returns bead IDs from PR feedback items that are not yet
// assigned to any task and not resolved/closed.
func (db *DB) GetUnassignedFeedbackBeadIDs(ctx context.Context, workID string) ([]string, error) {
	nullStrings, err := db.queries.GetUnassignedFeedbackBeadIDs(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unassigned feedback bead IDs: %w", err)
	}
	result := make([]string, 0, len(nullStrings))
	for _, ns := range nullStrings {
		if ns.Valid {
			result = append(result, ns.String)
		}
	}
	return result, nil
}

// GetFeedbackByBeadID returns the feedback associated with a bead.
func (db *DB) GetFeedbackByBeadID(ctx context.Context, beadID string) (*PRFeedback, error) {
	f, err := db.queries.GetPRFeedbackByBead(ctx, sql.NullString{String: beadID, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query feedback by bead ID: %w", err)
	}

	result := convertPrFeedback(f)
	return &result, nil
}

// HasExistingFeedback checks if feedback already exists for a specific source.
// Falls back to checking by title, source_type, and source_name.
func (db *DB) HasExistingFeedback(ctx context.Context, workID, title string, sourceType github.SourceType, sourceName string) (bool, error) {
	count, err := db.queries.HasExistingFeedback(ctx, sqlc.HasExistingFeedbackParams{
		WorkID:     workID,
		Title:      title,
		SourceType: sql.NullString{String: string(sourceType), Valid: sourceType != ""},
		SourceName: sql.NullString{String: sourceName, Valid: sourceName != ""},
	})
	if err != nil {
		return false, fmt.Errorf("failed to check existing feedback: %w", err)
	}

	return count > 0, nil
}

// HasExistingFeedbackBySourceID checks if feedback already exists for a specific source ID.
// This is the preferred method for checking duplicates when a unique source ID is available.
func (db *DB) HasExistingFeedbackBySourceID(ctx context.Context, workID, sourceID string) (bool, error) {
	count, err := db.queries.HasExistingFeedbackBySourceID(ctx, sqlc.HasExistingFeedbackBySourceIDParams{
		WorkID:   workID,
		SourceID: sql.NullString{String: sourceID, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("failed to check existing feedback by source ID: %w", err)
	}

	return count > 0, nil
}

// GetFeedbackBySourceID returns the feedback associated with a source ID (e.g., GitHub comment ID).
// Returns nil if no feedback exists for this source ID.
func (db *DB) GetFeedbackBySourceID(ctx context.Context, workID, sourceID string) (*PRFeedback, error) {
	f, err := db.queries.GetPRFeedbackBySourceID(ctx, sqlc.GetPRFeedbackBySourceIDParams{
		WorkID:   workID,
		SourceID: sql.NullString{String: sourceID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query feedback by source ID: %w", err)
	}

	result := convertPrFeedback(f)
	return &result, nil
}

// GetUnresolvedFeedbackForWork returns feedback items for the work where the associated issue is closed but not resolved on GitHub.
func (db *DB) GetUnresolvedFeedbackForWork(ctx context.Context, workID string) ([]PRFeedback, error) {
	feedbacks, err := db.queries.GetUnresolvedFeedbackForWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved feedback: %w", err)
	}

	result := make([]PRFeedback, len(feedbacks))
	for i, f := range feedbacks {
		result[i] = convertPrFeedback(f)
	}

	return result, nil
}

// GetUnresolvedFeedbackForBeads returns feedback items for specific beads that are not yet resolved on GitHub.
func (db *DB) GetUnresolvedFeedbackForBeads(ctx context.Context, beadIDs []string) ([]PRFeedback, error) {
	if len(beadIDs) == 0 {
		return nil, nil
	}

	// Convert string slice to sql.NullString slice for sqlc
	nullBeadIDs := make([]sql.NullString, len(beadIDs))
	for i, beadID := range beadIDs {
		nullBeadIDs[i] = sql.NullString{String: beadID, Valid: true}
	}

	// Use sqlc generated query
	feedbacks, err := db.queries.GetUnresolvedFeedbackForBeads(ctx, nullBeadIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to query unresolved feedback for beads: %w", err)
	}

	result := make([]PRFeedback, len(feedbacks))
	for i, f := range feedbacks {
		result[i] = convertPrFeedback(f)
	}

	return result, nil
}

// MarkFeedbackResolved marks feedback as resolved on GitHub.
func (db *DB) MarkFeedbackResolved(ctx context.Context, feedbackID string) error {
	// Use sqlc generated method
	err := db.queries.MarkPRFeedbackResolved(ctx, feedbackID)
	if err != nil {
		return fmt.Errorf("failed to mark feedback as resolved: %w", err)
	}

	return nil
}

// ScheduledTaskParams contains the parameters for scheduling a task.
type ScheduledTaskParams struct {
	WorkID         string
	TaskType       string
	ScheduledAt    time.Time
	Metadata       map[string]string
	IdempotencyKey string
	MaxAttempts    int
}

// MarkFeedbackResolvedAndScheduleTasks atomically marks feedback as resolved
// and schedules the associated GitHub tasks in a single transaction.
// This implements the transactional outbox pattern correctly.
func (db *DB) MarkFeedbackResolvedAndScheduleTasks(ctx context.Context, feedbackID string, tasks []ScheduledTaskParams) error {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	qtx := db.queries.WithTx(tx)

	// Mark feedback as resolved
	if err := qtx.MarkPRFeedbackResolved(ctx, feedbackID); err != nil {
		return fmt.Errorf("failed to mark feedback as resolved: %w", err)
	}

	// Schedule all tasks
	for _, task := range tasks {
		// Check if task with this idempotency key already exists
		if task.IdempotencyKey != "" {
			existing, err := qtx.GetTaskByIdempotencyKey(ctx, sql.NullString{String: task.IdempotencyKey, Valid: true})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("failed to check existing task: %w", err)
			}
			if existing.ID != "" {
				// Task already exists, skip
				continue
			}
		}

		id := uuid.New().String()

		metadataJSON := "{}"
		if task.Metadata != nil {
			data, err := json.Marshal(task.Metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal metadata: %w", err)
			}
			metadataJSON = string(data)
		}

		maxAttempts := task.MaxAttempts
		if maxAttempts <= 0 {
			maxAttempts = DefaultMaxAttempts
		}

		err := qtx.CreateScheduledTaskWithRetry(ctx, sqlc.CreateScheduledTaskWithRetryParams{
			ID:             id,
			WorkID:         task.WorkID,
			TaskType:       task.TaskType,
			ScheduledAt:    task.ScheduledAt,
			Status:         TaskStatusPending,
			Metadata:       metadataJSON,
			AttemptCount:   0,
			MaxAttempts:    int64(maxAttempts),
			IdempotencyKey: sql.NullString{String: task.IdempotencyKey, Valid: task.IdempotencyKey != ""},
		})
		if err != nil {
			return fmt.Errorf("failed to schedule task: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
