-- name: CacheComplexity :exec
REPLACE INTO complexity_cache (bean_id, description_hash, complexity_score, estimated_tokens)
VALUES (?, ?, ?, ?);

-- name: GetCachedComplexity :one
SELECT complexity_score, estimated_tokens
FROM complexity_cache
WHERE bean_id = ? AND description_hash = ?;

-- name: GetAllCachedComplexity :many
SELECT bean_id, complexity_score, estimated_tokens
FROM complexity_cache;

-- name: CountEstimatedBeans :one
SELECT COUNT(DISTINCT bean_id) as count
FROM complexity_cache
WHERE bean_id IN (sqlc.slice('bean_ids'));
