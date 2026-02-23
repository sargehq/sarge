-- name: StartBean :exec
INSERT INTO beans (id, status, title, zellij_session, zellij_pane, worktree_path, started_at, updated_at)
VALUES (?, 'processing', ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    status = 'processing',
    title = excluded.title,
    zellij_session = excluded.zellij_session,
    zellij_pane = excluded.zellij_pane,
    worktree_path = excluded.worktree_path,
    started_at = excluded.started_at,
    updated_at = excluded.updated_at;

-- name: CompleteBean :execrows
UPDATE beans
SET status = 'completed',
    pr_url = ?,
    completed_at = ?,
    updated_at = ?
WHERE id = ?;

-- name: FailBean :execrows
UPDATE beans
SET status = 'failed',
    error_message = ?,
    completed_at = ?,
    updated_at = ?
WHERE id = ?;

-- name: GetBean :one
SELECT id, status, title, pr_url, error_message, zellij_session, zellij_pane,
       worktree_path, started_at, completed_at, created_at, updated_at
FROM beans
WHERE id = ?;

-- name: GetBeanStatus :one
SELECT status
FROM beans
WHERE id = ?;

-- name: ListBeans :many
SELECT id, status, title, pr_url, error_message, zellij_session, zellij_pane,
       worktree_path, started_at, completed_at, created_at, updated_at
FROM beans
ORDER BY created_at DESC;

-- name: ListBeansByStatus :many
SELECT id, status, title, pr_url, error_message, zellij_session, zellij_pane,
       worktree_path, started_at, completed_at, created_at, updated_at
FROM beans
WHERE status = ?
ORDER BY created_at DESC;
