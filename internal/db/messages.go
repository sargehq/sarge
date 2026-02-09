package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// Message represents a chat message with idiomatic Go types (no sql.Null*).
type Message struct {
	ID        int64
	Source    string
	Text      string
	WorkID    string
	TaskID    string
	BeadID    string
	EventType string
	CreatedAt time.Time
}

// InsertMessage inserts a new message into the database.
func (db *DB) InsertMessage(ctx context.Context, source, text, workID, taskID, beadID, eventType string) (int64, error) {
	result, err := db.queries.InsertMessage(ctx, sqlc.InsertMessageParams{
		Source:    source,
		Text:      text,
		WorkID:    toNullString(workID),
		TaskID:    toNullString(taskID),
		BeadID:    toNullString(beadID),
		EventType: toNullString(eventType),
	})
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// DeleteMessage deletes a message by ID.
func (db *DB) DeleteMessage(ctx context.Context, id int64) error {
	_, err := db.queries.DeleteMessage(ctx, id)
	return err
}

// ListMessages returns messages in chronological order, up to limit.
func (db *DB) ListMessages(ctx context.Context, limit int64) ([]Message, error) {
	rows, err := db.queries.ListMessages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return convertMessages(rows), nil
}

// ListRecentMessages returns recent messages in reverse chronological order, up to limit.
func (db *DB) ListRecentMessages(ctx context.Context, limit int64) ([]Message, error) {
	rows, err := db.queries.ListRecentMessages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return convertMessages(rows), nil
}

func convertMessages(rows []sqlc.Message) []Message {
	msgs := make([]Message, len(rows))
	for i, r := range rows {
		msgs[i] = Message{
			ID:        r.ID,
			Source:    r.Source,
			Text:      r.Text,
			WorkID:    fromNullString(r.WorkID),
			TaskID:    fromNullString(r.TaskID),
			BeadID:    fromNullString(r.BeadID),
			EventType: fromNullString(r.EventType),
			CreatedAt: r.CreatedAt,
		}
	}
	return msgs
}

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func fromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
