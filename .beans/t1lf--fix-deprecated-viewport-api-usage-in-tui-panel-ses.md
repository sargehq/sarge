---
# t1lf
title: Fix deprecated viewport API usage in tui_panel_session.go
status: completed
type: bug
priority: normal
created_at: 2026-03-01T22:59:25Z
updated_at: 2026-03-02T21:36:06Z
parent: ir72
---

2 staticcheck lint errors in internal/tui/tui_panel_session.go: (1) Line 485: SA1019: p.viewport.HalfViewDown is deprecated, use Model.HalfPageDown instead. (2) Line 487: SA1019: p.viewport.HalfViewUp is deprecated, use Model.HalfPageUp instead.

## Summary of Changes\n\nReplaced deprecated HalfViewDown/HalfViewUp with HalfPageDown/HalfPageUp in tui_panel_session.go.
