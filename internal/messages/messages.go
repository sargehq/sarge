package messages

import (
	"context"
	"time"

	"github.com/sargehq/sarge/internal/db"
)

// Source constants for message origin.
const (
	SourceUser   = "user"
	SourceSystem = "system"
)

// Event type constants.
const (
	EventPrompt      = "prompt"
	EventWorkCreated = "work_created"
	EventError       = "error"
	EventInfo        = "info"
)

// Message represents a chat message.
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

// WriteParams holds parameters for writing a message.
type WriteParams struct {
	Source    string
	Text      string
	WorkID    string
	TaskID    string
	BeadID    string
	EventType string
}

// Write persists a message to the database.
func Write(ctx context.Context, database *db.DB, params WriteParams) error {
	_, err := database.InsertMessage(ctx, params.Source, params.Text, params.WorkID, params.TaskID, params.BeadID, params.EventType)
	return err
}

// List returns messages in chronological order, up to limit.
func List(ctx context.Context, database *db.DB, limit int64) ([]Message, error) {
	rows, err := database.ListMessages(ctx, limit)
	if err != nil {
		return nil, err
	}
	return convertDBMessages(rows), nil
}

func convertDBMessages(rows []db.Message) []Message {
	msgs := make([]Message, len(rows))
	for i, r := range rows {
		msgs[i] = Message{
			ID:        r.ID,
			Source:    r.Source,
			Text:      r.Text,
			WorkID:    r.WorkID,
			TaskID:    r.TaskID,
			BeadID:    r.BeadID,
			EventType: r.EventType,
			CreatedAt: r.CreatedAt,
		}
	}
	return msgs
}
