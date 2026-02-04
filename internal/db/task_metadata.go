package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sargehq/sarge/internal/db/sqlc"
)

// Metadata key constants
const (
	// Add metadata keys here as needed
)

// SetTaskMetadata sets a metadata key-value pair on a task.
// If the key already exists, it updates the value.
func (db *DB) SetTaskMetadata(ctx context.Context, taskID, key, value string) error {
	err := db.queries.SetTaskMetadata(ctx, sqlc.SetTaskMetadataParams{
		TaskID: taskID,
		Key:    key,
		Value:  value,
	})
	if err != nil {
		return fmt.Errorf("failed to set metadata %s for task %s: %w", key, taskID, err)
	}
	return nil
}

// GetTaskMetadata gets a metadata value by task ID and key.
// Returns empty string and nil error if the key doesn't exist.
func (db *DB) GetTaskMetadata(ctx context.Context, taskID, key string) (string, error) {
	value, err := db.queries.GetTaskMetadata(ctx, sqlc.GetTaskMetadataParams{
		TaskID: taskID,
		Key:    key,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get metadata %s for task %s: %w", key, taskID, err)
	}
	return value, nil
}

// GetAllTaskMetadata returns all metadata for a task as a map.
func (db *DB) GetAllTaskMetadata(ctx context.Context, taskID string) (map[string]string, error) {
	rows, err := db.queries.GetAllTaskMetadata(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get all metadata for task %s: %w", taskID, err)
	}

	result := make(map[string]string)
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	return result, nil
}

