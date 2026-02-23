---
# main-z865
title: 'Create migration 002: rename bead tables to bean tables'
status: todo
type: task
priority: critical
created_at: 2026-02-23T02:06:25Z
updated_at: 2026-02-23T02:06:25Z
parent: main-3isk
blocked_by:
    - main-1icu
---

New migration file: 002_beads_to_beans.sql
- beads → beans table
- task_beads → task_beans (columns: bead_id → bean_id)
- work_beads → work_beans (columns: bead_id → bean_id)
- complexity_cache: bead_id → bean_id
- plan_sessions: bead_id → bean_id
- pr_feedback: bead_id → bean_id
- All indexes renamed accordingly

## Files
- internal/db/migrations/002_beads_to_beans.sql (new)
- internal/db/schema.sql (update reference schema)
