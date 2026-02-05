-- name: CreatePRFeedback :exec
INSERT INTO pr_feedback (
    id, work_id, pr_url, feedback_type, title, description,
    source_url, source_id, priority,
    source_type, source_name, context
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetPRFeedback :one
SELECT * FROM pr_feedback WHERE id = ?;

-- name: GetPRFeedbackByBead :one
SELECT * FROM pr_feedback WHERE bead_id = ? LIMIT 1;

-- name: ListPRFeedback :many
SELECT * FROM pr_feedback WHERE work_id = ? ORDER BY created_at DESC;

-- name: ListUnprocessedPRFeedback :many
SELECT * FROM pr_feedback
WHERE work_id = ? AND processed_at IS NULL
ORDER BY priority ASC, created_at ASC;

-- name: GetUnresolvedFeedbackForWork :many
SELECT * FROM pr_feedback
WHERE work_id = ?
  AND bead_id IS NOT NULL
  AND resolved_at IS NULL
  AND source_id IS NOT NULL
ORDER BY created_at ASC;

-- name: GetUnresolvedFeedbackForBeads :many
SELECT * FROM pr_feedback
WHERE bead_id IN (sqlc.slice('bead_ids'))
  AND resolved_at IS NULL
  AND source_id IS NOT NULL
ORDER BY created_at ASC;

-- name: MarkPRFeedbackProcessed :exec
UPDATE pr_feedback
SET processed_at = CURRENT_TIMESTAMP, bead_id = ?
WHERE id = ?;

-- name: MarkPRFeedbackResolved :exec
UPDATE pr_feedback
SET resolved_at = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: HasExistingFeedback :one
SELECT COUNT(*) as count FROM pr_feedback
WHERE work_id = ? AND title = ? AND source_type = ? AND source_name = ?;

-- name: DeletePRFeedback :exec
DELETE FROM pr_feedback WHERE id = ?;

-- name: DeletePRFeedbackForWork :exec
DELETE FROM pr_feedback WHERE work_id = ?;

-- name: GetPRFeedbackBySourceID :one
SELECT * FROM pr_feedback
WHERE work_id = ? AND source_id = ?
LIMIT 1;

-- name: CountUnassignedFeedbackForWork :one
-- Count PR feedback items that have beads which are not yet assigned to any task and not resolved/closed.
SELECT COUNT(*) as count FROM pr_feedback pf
WHERE pf.work_id = ?
  AND pf.bead_id IS NOT NULL
  AND pf.resolved_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM task_beads tb
    JOIN tasks t ON tb.task_id = t.id
    WHERE tb.bead_id = pf.bead_id
      AND t.work_id = pf.work_id
  );

-- name: GetUnassignedFeedbackBeadIDs :many
-- Get bead IDs from PR feedback items that are not yet assigned to any task and not resolved/closed.
SELECT pf.bead_id FROM pr_feedback pf
WHERE pf.work_id = ?
  AND pf.bead_id IS NOT NULL
  AND pf.resolved_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM task_beads tb
    JOIN tasks t ON tb.task_id = t.id
    WHERE tb.bead_id = pf.bead_id
      AND t.work_id = pf.work_id
  )
ORDER BY pf.created_at ASC;

-- name: HasExistingFeedbackBySourceID :one
-- Only consider feedback "existing" if a bead was actually created.
-- This allows retry if bead creation failed on a previous attempt.
SELECT COUNT(*) as count FROM pr_feedback
WHERE work_id = ? AND source_id = ? AND bead_id IS NOT NULL;