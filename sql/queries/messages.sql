-- name: InsertMessage :execresult
INSERT INTO messages (source, text, work_id, task_id, bead_id, event_type)
VALUES (?, ?, ?, ?, ?, ?);

-- name: ListMessages :many
SELECT id, source, text, work_id, task_id, bead_id, event_type, created_at
FROM messages ORDER BY created_at ASC LIMIT ?;

-- name: ListRecentMessages :many
SELECT id, source, text, work_id, task_id, bead_id, event_type, created_at
FROM messages ORDER BY created_at DESC LIMIT ?;

-- name: DeleteMessage :execresult
DELETE FROM messages WHERE id = ?;
