-- name: RegisterProcess :exec
INSERT INTO processes (id, process_type, work_id, pid, hostname, heartbeat, started_at)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT (id) DO UPDATE SET
    pid = excluded.pid,
    hostname = excluded.hostname,
    heartbeat = CURRENT_TIMESTAMP;

-- name: UpdateHeartbeat :exec
UPDATE processes
SET heartbeat = CURRENT_TIMESTAMP
WHERE id = ?;

-- name: UpdateHeartbeatWithTime :exec
UPDATE processes
SET heartbeat = ?
WHERE id = ?;

-- name: GetProcess :one
SELECT * FROM processes WHERE id = ?;

-- name: GetProcessByWorkID :one
SELECT * FROM processes
WHERE work_id = ? AND process_type = 'orchestrator'
LIMIT 1;

-- name: GetControlPlaneProcess :one
SELECT * FROM processes
WHERE process_type = 'control_plane'
LIMIT 1;

-- name: GetStaleProcesses :many
SELECT * FROM processes
WHERE datetime(heartbeat) < datetime('now', ? || ' seconds');

-- name: IsOrchestratorAlive :one
SELECT EXISTS(
    SELECT 1 FROM processes
    WHERE work_id = ?
      AND process_type = 'orchestrator'
      AND datetime(heartbeat) >= datetime('now', ? || ' seconds')
) as alive;

-- name: IsControlPlaneAlive :one
SELECT EXISTS(
    SELECT 1 FROM processes
    WHERE process_type = 'control_plane'
      AND datetime(heartbeat) >= datetime('now', ? || ' seconds')
) as alive;

-- name: DeleteProcess :exec
DELETE FROM processes WHERE id = ?;

-- name: DeleteStaleProcesses :exec
DELETE FROM processes
WHERE datetime(heartbeat) < datetime('now', ? || ' seconds');

-- name: DeleteControlPlaneProcess :exec
DELETE FROM processes WHERE process_type = 'control_plane';

-- name: DeleteOrchestratorByWorkID :exec
DELETE FROM processes WHERE work_id = ? AND process_type = 'orchestrator';

-- name: GetOrchestratorProcess :one
SELECT * FROM processes
WHERE work_id = ? AND process_type = 'orchestrator'
LIMIT 1;

-- name: GetAllProcesses :many
SELECT * FROM processes ORDER BY started_at DESC;

-- name: UpdatePprofPort :exec
UPDATE processes
SET pprof_port = ?
WHERE id = ?;
