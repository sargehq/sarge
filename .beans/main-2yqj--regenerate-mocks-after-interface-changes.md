---
# main-2yqj
title: Regenerate mocks after interface changes
status: completed
type: task
priority: high
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:14:58Z
parent: main-1icu
---

Run go generate after CLI and Reader interfaces are updated.
- Update interface names: BeadsCLIMock → BeansCLIMock, BeadsReaderMock → BeansReaderMock
- Regenerate with moq

## Files
- beads_mock.go → beans_mock.go
