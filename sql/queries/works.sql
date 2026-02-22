-- name: CreateWork :exec
INSERT INTO works (id, status, name, worktree_path, branch_name, base_branch, root_issue_id, auto)
VALUES (?, 'pending', ?, ?, ?, ?, ?, ?);

-- name: StartWork :execrows
UPDATE works
SET status = 'processing',
    zellij_session = ?,
    zellij_tab = ?,
    started_at = ?
WHERE id = ?;

-- name: CompleteWork :execrows
UPDATE works
SET status = 'completed',
    pr_url = ?,
    completed_at = ?
WHERE id = ?;

-- name: FailWork :execrows
UPDATE works
SET status = 'failed',
    error_message = ?,
    completed_at = ?
WHERE id = ?;

-- name: IdleWork :execrows
UPDATE works
SET status = 'idle'
WHERE id = ? AND status NOT IN ('merged', 'completed');

-- name: IdleWorkWithPR :execrows
UPDATE works
SET status = 'idle',
    pr_url = ?
WHERE id = ? AND status NOT IN ('merged', 'completed');

-- name: RestartWork :execrows
UPDATE works
SET status = 'processing',
    error_message = ''
WHERE id = ? AND status = 'failed';

-- name: ResumeWork :execrows
UPDATE works
SET status = 'processing'
WHERE id = ? AND status IN ('idle', 'completed');

-- name: GetWork :one
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
WHERE id = ?;

-- name: ListWorks :many
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
ORDER BY created_at DESC;

-- name: ListWorksByStatus :many
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
WHERE status = ?
ORDER BY created_at DESC;

-- name: GetLastWorkID :one
SELECT id FROM works
ORDER BY created_at DESC
LIMIT 1;

-- name: GetWorkByDirectory :one
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
WHERE worktree_path LIKE ?
LIMIT 1;

-- name: AddTaskToWork :exec
INSERT INTO work_tasks (work_id, task_id, position)
VALUES (?, ?, ?);

-- name: GetWorkTasks :many
SELECT t.id, t.status,
       COALESCE(t.task_type, 'implement') as task_type,
       t.complexity_budget,
       t.actual_complexity,
       t.work_id,
       t.worktree_path,
       t.pr_url,
       t.error_message,
       t.started_at,
       t.completed_at,
       t.created_at,
       t.spawned_at,
       t.spawn_status
FROM tasks t
JOIN work_tasks wt ON t.id = wt.task_id
WHERE wt.work_id = ?
ORDER BY t.task_number DESC;

-- name: DeleteWorkTasks :execrows
DELETE FROM work_tasks
WHERE work_id = ?;

-- name: DeleteWork :execrows
DELETE FROM works
WHERE id = ?;

-- name: InitializeTaskCounter :exec
INSERT INTO work_task_counters (work_id, next_task_num)
VALUES (?, 1)
ON CONFLICT (work_id) DO NOTHING;

-- name: GetAndIncrementTaskCounter :one
UPDATE work_task_counters
SET next_task_num = next_task_num + 1
WHERE work_id = ?
RETURNING next_task_num - 1 as task_num;

-- name: UpdateWorkWorktreePath :execrows
UPDATE works
SET worktree_path = ?
WHERE id = ?;

-- name: UpdateWorkPRStatus :execrows
UPDATE works
SET ci_status = ?,
    approval_status = ?,
    approvers = ?,
    pr_state = ?,
    mergeable_state = ?,
    last_pr_poll_at = ?
WHERE id = ?;

-- name: MergeWork :execrows
UPDATE works
SET status = 'merged',
    pr_state = 'merged',
    completed_at = ?
WHERE id = ?;

-- name: SetWorkHasUnseenPRChanges :execrows
UPDATE works
SET has_unseen_pr_changes = ?
WHERE id = ?;

-- name: MarkWorkPRSeen :execrows
UPDATE works
SET has_unseen_pr_changes = FALSE
WHERE id = ?;

-- name: GetWorksWithUnseenChanges :many
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
WHERE has_unseen_pr_changes = TRUE
ORDER BY created_at DESC;

-- name: GetWorksWithPRs :many
SELECT id, status,
       name,
       zellij_session,
       zellij_tab,
       worktree_path,
       branch_name,
       base_branch,
       root_issue_id,
       pr_url,
       error_message,
       started_at,
       completed_at,
       created_at,
       auto,
       ci_status,
       approval_status,
       approvers,
       last_pr_poll_at,
       has_unseen_pr_changes,
       pr_state,
       mergeable_state
FROM works
WHERE pr_url != ''
ORDER BY created_at DESC;

-- name: SetWorkPRURL :execrows
UPDATE works
SET pr_url = ?
WHERE id = ? AND (pr_url = '' OR pr_url IS NULL);
