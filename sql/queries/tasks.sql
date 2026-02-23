-- name: CreateTask :exec
INSERT INTO tasks (id, status, task_type, complexity_budget, work_id, task_number)
VALUES (?, 'pending', ?, ?, ?, ?);

-- name: CreateTaskBean :exec
INSERT INTO task_beans (task_id, bean_id, status)
VALUES (?, ?, 'pending');

-- name: StartTask :execrows
UPDATE tasks
SET status = 'processing',
    worktree_path = ?,
    started_at = ?
WHERE id = ?;

-- name: CompleteTask :execrows
UPDATE tasks
SET status = 'completed',
    pr_url = ?,
    completed_at = ?
WHERE id = ?;

-- name: FailTask :execrows
UPDATE tasks
SET status = 'failed',
    error_message = ?,
    completed_at = ?
WHERE id = ?;

-- name: ResetTaskStatus :execrows
UPDATE tasks
SET status = 'pending',
    started_at = NULL,
    error_message = ''
WHERE id = ?;

-- name: GetTask :one
SELECT id, status,
       COALESCE(task_type, 'implement') as task_type,
       complexity_budget,
       actual_complexity,
       work_id,
       worktree_path,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       spawned_at,
       spawn_status
FROM tasks
WHERE id = ?;

-- name: GetTaskBeans :many
SELECT bean_id
FROM task_beans
WHERE task_id = ?;

-- name: GetTaskForBean :one
SELECT task_id
FROM task_beans
WHERE bean_id = ?;

-- name: CompleteTaskBean :execrows
UPDATE task_beans
SET status = 'completed'
WHERE task_id = ? AND bean_id = ?;

-- name: FailTaskBean :execrows
UPDATE task_beans
SET status = 'failed'
WHERE task_id = ? AND bean_id = ?;

-- name: CountTaskBeanStatuses :one
SELECT COUNT(*) as total,
       COUNT(CASE WHEN status = 'completed' THEN 1 END) as completed
FROM task_beans
WHERE task_id = ?;

-- name: ListTasks :many
SELECT id, status,
       COALESCE(task_type, 'implement') as task_type,
       complexity_budget,
       actual_complexity,
       work_id,
       worktree_path,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       spawned_at,
       spawn_status
FROM tasks
ORDER BY created_at DESC;

-- name: ListTasksByStatus :many
SELECT id, status,
       COALESCE(task_type, 'implement') as task_type,
       complexity_budget,
       actual_complexity,
       work_id,
       worktree_path,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       spawned_at,
       spawn_status
FROM tasks
WHERE status = ?
ORDER BY created_at DESC;

-- name: DeleteTaskBeansForWork :execrows
DELETE FROM task_beans
WHERE task_id IN (
    SELECT task_id FROM work_tasks WHERE work_id = ?
);

-- name: DeleteTasksForWork :execrows
DELETE FROM tasks
WHERE work_id = ?;

-- name: GetTaskBeanStatus :one
SELECT status
FROM task_beans
WHERE task_id = ? AND bean_id = ?;

-- name: DeleteWorkTaskByTask :execrows
DELETE FROM work_tasks
WHERE task_id = ?;

-- name: DeleteTaskBeansByTask :execrows
DELETE FROM task_beans
WHERE task_id = ?;

-- name: DeleteTask :execrows
DELETE FROM tasks
WHERE id = ?;

-- name: ResetTaskBeanStatuses :execrows
UPDATE task_beans
SET status = 'pending'
WHERE task_id = ?;

-- name: GetTaskBeansWithStatus :many
SELECT task_id, bean_id, status
FROM task_beans
WHERE task_id = ?;

-- name: ResetTaskBeanStatus :execrows
UPDATE task_beans
SET status = 'pending'
WHERE task_id = ? AND bean_id = ?;

-- name: SpawnTask :execrows
UPDATE tasks
SET spawned_at = ?,
    spawn_status = ?
WHERE id = ?;

-- name: UpdateTaskActivity :execrows
UPDATE tasks
SET last_activity = ?
WHERE id = ? AND status = 'processing';

-- name: GetTaskBeansForWork :many
SELECT tb.task_id, tb.bean_id, tb.status
FROM task_beans tb
JOIN tasks t ON tb.task_id = t.id
WHERE t.work_id = ?;

-- name: GetTasksWithActivity :many
SELECT id, status,
       COALESCE(task_type, 'implement') as task_type,
       complexity_budget,
       actual_complexity,
       work_id,
       worktree_path,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       spawned_at,
       spawn_status,
       last_activity
FROM tasks
WHERE status = 'processing'
ORDER BY last_activity DESC;

-- name: GetPRTaskForWork :one
SELECT id, status,
       COALESCE(task_type, 'implement') as task_type,
       complexity_budget,
       actual_complexity,
       work_id,
       worktree_path,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       spawned_at,
       spawn_status
FROM tasks
WHERE work_id = ?
  AND task_type = 'pr'
  AND status IN ('pending', 'processing', 'completed')
ORDER BY created_at DESC
LIMIT 1;
