---
# main-ekdf
title: Update internal/project/ package (beads→beans)
status: completed
type: task
priority: high
created_at: 2026-02-23T02:06:50Z
updated_at: 2026-02-23T02:52:32Z
parent: main-3hea
blocked_by:
    - main-1icu
---

Update project config and setup:
- BeadsConfig → BeansConfig (toml tag: beads → beans)
- BeadsPathRepo → BeansPathRepo (main/.beads → main/.beans)
- BeadsPathProject → BeansPathProject (.co/.beads → .co/.beans)
- setupBeads → setupBeans (use beans init instead of bd init)
- Remove Reinit/InstallHooks calls (beans doesn't need them)
- Config template updates

## Files (2+)
- internal/project/config.go
- internal/project/project.go
- internal/project/config_test.go
- internal/project/templates/config.tmpl
