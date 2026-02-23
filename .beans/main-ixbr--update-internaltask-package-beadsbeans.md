---
# main-ixbr
title: Update internal/task/ package (beads→beans)
status: completed
type: task
priority: high
created_at: 2026-02-23T02:06:50Z
updated_at: 2026-02-23T02:52:32Z
parent: main-3hea
blocked_by:
    - main-1icu
    - main-3isk
---

Update all beads.Bead refs → beans.Bean, beads.Dependency → beans.Dependency.
- Planner interface and implementation
- DependencyGraph and TopologicalSort
- Complexity estimator
- Task type (BeadIDs→BeanIDs, Beads→Beans fields)
- Mocks (PlannerMock, ComplexityEstimatorMock)

## Files (7)
- internal/task/task.go
- internal/task/planner.go
- internal/task/deps.go
- internal/task/complexity.go
- internal/task/task_mock.go
- internal/task/planner_test.go
- internal/task/internal_test.go
