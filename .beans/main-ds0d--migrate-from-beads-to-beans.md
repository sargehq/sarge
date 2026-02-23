---
# main-ds0d
title: Migrate from Beads to Beans
status: completed
type: milestone
priority: critical
created_at: 2026-02-23T02:05:45Z
updated_at: 2026-02-23T04:08:12Z
---

Replace the beads issue tracker (bd CLI + sqlite) with beans (beans CLI + markdown files + GraphQL) across the entire codebase.

## Scope
- ~40 Go source files to modify
- ~4,300 lines in internal/beads/ to rewrite
- New DB migration for table renames
- sqlc regeneration
- All tests need updating

## Key Architecture Changes
- SQLite database → Markdown files in .beans/
- Direct SQL reads via sqlc → GraphQL API or CLI with --json
- bd CLI for writes → beans CLI for reads AND writes
- Numeric priority (0-4) → Named priority (critical/high/normal/low/deferred)
- Status: open/closed → Status: draft/todo/in-progress/completed/scrapped
- bd sync needed → Just git commit (plain files)
