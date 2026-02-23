---
# main-y977
title: Rewrite Reader (sqlite Client → beans CLI/GraphQL reader)
status: completed
type: task
priority: critical
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:14:58Z
parent: main-1icu
---

Replace direct sqlite reads with beans GraphQL queries:
- GetBeadsWithDeps → GraphQL query for beans + blockedBy + children
- ListBeads → beans list --json with filters
- GetReadyBeads → beans list --json --ready
- GetBead → beans show --json or beans graphql
- GetTransitiveDependencies → GraphQL recursive query
- GetBeadWithChildren → GraphQL children query
- Delete queries/ subpackage entirely (no more sqlc for beads DB)
- Delete schema.sql (beads DB schema)
- Remove sqlite3 driver import for beads

## Files
- client.go → full rewrite
- Delete queries/ directory
- Delete schema.sql
- client_test.go → update
