---
# main-6qa2
title: Rewrite domain types (Bead → Bean struct)
status: todo
type: task
priority: critical
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:06:09Z
parent: main-1icu
---

Map fields between beads and beans data models:
- Status: open→todo, closed→completed
- Priority: numeric 0-4 → named (critical/high/normal/low/deferred)
- Type mappings preserved (task, bug, feature, epic)
- Relationships: dependencies → blocking/blocked-by, parent-child preserved
- Drop sqlite-specific fields (content_hash, compaction_level, etc.)
- Add beans-specific fields (slug, path, etag, tags)

## Files
- bead.go → bean.go
- status.go (update constants)
