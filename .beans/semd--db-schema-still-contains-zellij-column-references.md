---
# semd
title: DB schema still contains zellij column references
status: completed
type: task
priority: normal
tags:
    - review-w-2fo.14
created_at: 2026-03-01T22:54:09Z
updated_at: 2026-03-02T21:44:48Z
parent: ir72
---

The DB layer (internal/db/bean.go, internal/db/work.go, internal/db/sqlc/) still contains zellij_session, zellij_tab, zellij_pane columns. A migration should be created to drop these columns and sqlc regenerated.

## Summary of Changes\n\nDropped zellij columns from the DB schema:\n- Created migration 007_drop_zellij_columns.sql to drop zellij_session/zellij_tab from works, zellij_session/zellij_pane from beans, and rename zellij_session to session_name in plan_sessions\n- Updated schema.sql, SQL queries, and regenerated sqlc\n- Updated Go wrapper code in bean.go, work.go, plan_session.go\n- Updated all callers and tests
