-- name: AddWorkBean :exec
INSERT INTO work_beans (work_id, bean_id, position)
VALUES (?, ?, ?);

-- name: AddWorkBeansBatch :exec
INSERT INTO work_beans (work_id, bean_id, position)
VALUES (?, ?, ?)
ON CONFLICT (work_id, bean_id) DO UPDATE SET
    position = excluded.position;

-- name: RemoveWorkBean :execrows
DELETE FROM work_beans
WHERE work_id = ? AND bean_id = ?;

-- name: GetWorkBeans :many
SELECT work_id, bean_id, position, created_at
FROM work_beans
WHERE work_id = ?
ORDER BY position;

-- name: GetUnassignedWorkBeans :many
SELECT wb.work_id, wb.bean_id, wb.position, wb.created_at
FROM work_beans wb
WHERE wb.work_id = ?
  AND NOT EXISTS (
    SELECT 1 FROM task_beans tb
    JOIN work_tasks wt ON tb.task_id = wt.task_id
    WHERE wt.work_id = wb.work_id AND tb.bean_id = wb.bean_id
  )
ORDER BY wb.position;

-- name: IsBeanInTask :one
SELECT COUNT(*) > 0 as in_task
FROM task_beans tb
JOIN work_tasks wt ON tb.task_id = wt.task_id
WHERE wt.work_id = ? AND tb.bean_id = ?;

-- name: DeleteWorkBeans :execrows
DELETE FROM work_beans
WHERE work_id = ?;

-- name: GetMaxWorkBeanPosition :one
SELECT CAST(COALESCE(MAX(position), -1) AS INTEGER) as max_position
FROM work_beans
WHERE work_id = ?;

-- name: GetAllAssignedBeans :many
-- Returns all beans assigned to any work, with their work ID.
-- This is used by plan mode to show which beans are already assigned.
SELECT bean_id, work_id
FROM work_beans
ORDER BY bean_id;
