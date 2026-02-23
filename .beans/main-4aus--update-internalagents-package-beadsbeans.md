---
# main-4aus
title: Update internal/agents/ package (beads→beans)
status: todo
type: task
priority: high
created_at: 2026-02-23T02:06:50Z
updated_at: 2026-02-23T02:06:50Z
parent: main-3hea
blocked_by:
    - main-1icu
---

Update agent runner and templates.
- Runner imports and bead references
- Claude templates referencing beads/bd
- Pi templates if any

## Files (3+)
- internal/agents/runner/runner.go
- internal/agents/claude/templates/*.tmpl
- internal/agents/pi/templates/*.tmpl (if applicable)
