---
# main-58eo
title: Work panel content width calculation off by 2 — separators overflow and left box too short
status: completed
type: bug
priority: normal
created_at: 2026-03-12T18:14:38Z
updated_at: 2026-03-12T18:15:23Z
---

The work overview, summary, and task sub-panels use `contentWidth := panelWidth - 2` which only accounts for padding (Padding(0,1) = 2 chars). But lipgloss v2's `.Width()` includes both border AND padding, so content wraps at `Width - 4` (border 2 + padding 2). The IssuesPanel correctly uses `p.width - 4`.

This causes:
1. Right panel (Details): separator lines (strings.Repeat("─", contentWidth)) are 2 chars too wide, wrapping to next line
2. Left panel (Work): same issue with separator, eating vertical space and making the box appear too short

## Files to fix
- `internal/tui/tui_panel_work_overview.go`: line 249 `contentWidth := panelWidth - 2` → `panelWidth - 4`
- `internal/tui/tui_panel_work_summary.go`: line 119 `contentWidth := panelWidth - 2` → `panelWidth - 4`, viewport SetWidth
- `internal/tui/tui_panel_work_task.go`: lines 145,199 `contentWidth := panelWidth - 2` → `panelWidth - 4`, viewport SetWidth

## Todo
- [x] Fix contentWidth in tui_panel_work_overview.go
- [x] Fix contentWidth in tui_panel_work_summary.go  
- [x] Fix contentWidth in tui_panel_work_task.go
- [x] Fix viewport SetWidth in summary and task panels
- [x] Verify build compiles

## Summary of Changes

Fixed content width calculation in three work panel files. All used `panelWidth - 2` (only accounting for padding), but lipgloss v2's `.Width()` includes both border (2 chars) and padding (2 chars), so the correct offset is `panelWidth - 4`. The IssuesPanel already had the correct calculation (`p.width - 4`).

### Files changed:
- `internal/tui/tui_panel_work_overview.go`: contentWidth fix (line 249)
- `internal/tui/tui_panel_work_summary.go`: contentWidth fix + viewport SetWidth fix
- `internal/tui/tui_panel_work_task.go`: contentWidth fix (2 occurrences) + viewport SetWidth fix
