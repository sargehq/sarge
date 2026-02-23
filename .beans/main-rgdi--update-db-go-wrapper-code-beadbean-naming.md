---
# main-rgdi
title: Update DB Go wrapper code (bead→bean naming)
status: todo
type: task
priority: critical
created_at: 2026-02-23T02:06:25Z
updated_at: 2026-02-23T02:06:25Z
parent: main-3isk
---

Rename all DB wrapper functions and types:
- ListBeads → ListBeans
- GetTaskBeads → GetTaskBeans
- GetTaskBeadsWithStatus → GetTaskBeansWithStatus
- GetWorkBeads → GetWorkBeans
- AddWorkBeads → AddWorkBeans
- GetUnassignedWorkBeads → GetUnassignedWorkBeans
- DeleteWorkBeads → DeleteWorkBeans
- GetAllAssignedBeads → GetAllAssignedBeans
- CountTaskBeadStatuses → CountTaskBeanStatuses
- AreAllBeadsEstimated → AreAllBeansEstimated
- CheckAndCompleteTask bead references
- Rename files: bead.go→bean.go, work_beads.go→work_beans.go

## Files
- internal/db/bead.go → internal/db/bean.go
- internal/db/work_beads.go → internal/db/work_beans.go
- internal/db/task.go
- internal/db/complexity.go
- internal/db/pr_feedback.go
- internal/db/work.go
