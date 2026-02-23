---
# main-cuml
title: Update sqlc queries and regenerate
status: completed
type: task
priority: critical
created_at: 2026-02-23T02:06:25Z
updated_at: 2026-02-23T04:05:11Z
parent: main-3isk
---

Update all .sql query files to use new table/column names, then regenerate sqlc code.
- Rename all bead references in query files
- Run sqlc generate
- Update Go wrapper code

## Files
- internal/db/sqlc/*.sql.go (regenerated)
- internal/db/sqlc/querier.go (regenerated)
- Any .sql source files
