---
# main-yu6g
title: Update internal/progress/ package (beads→beans)
status: todo
type: task
priority: high
created_at: 2026-02-23T02:06:50Z
updated_at: 2026-02-23T02:06:50Z
parent: main-3hea
blocked_by:
    - main-1icu
    - main-3isk
---

Update progress tracking types and fetch logic.
- BeadProgress → BeanProgress
- WorkBeads → WorkBeans
- UnassignedBeads → UnassignedBeans
- All bead references in fetch logic

## Files (2)
- internal/progress/fetch.go
- internal/progress/types.go
