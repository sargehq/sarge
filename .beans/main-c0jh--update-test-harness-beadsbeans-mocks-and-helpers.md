---
# main-c0jh
title: Update test harness (beads→beans mocks and helpers)
status: todo
type: task
priority: high
created_at: 2026-02-23T02:07:03Z
updated_at: 2026-02-23T02:07:03Z
parent: main-a6ly
blocked_by:
    - main-1icu
    - main-3isk
---

Update TestHarness to use beans types and mocks:
- BeadsCLIMock → BeansCLIMock
- BeadsReaderMock → BeansReaderMock
- CreateBead → CreateBean
- CreateEpicWithChildren → update bead→bean naming
- SetBeadDependency → SetBeanDependency
- AddReviewIssues → update types
- SimulateReviewCompletion → update types
- All internal helper methods

## Files (2)
- internal/testutil/harness.go
- internal/testutil/harness_test.go
