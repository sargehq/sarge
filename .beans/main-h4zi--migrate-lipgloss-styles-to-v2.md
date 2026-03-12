---
# main-h4zi
title: Migrate Lipgloss styles to v2
status: todo
type: task
created_at: 2026-03-12T01:10:17Z
updated_at: 2026-03-12T01:10:17Z
parent: main-mtvk
blocked_by:
    - main-xh70
---

Update all lipgloss imports and usage to v2 API.

Key changes:
- Import path: `github.com/charmbracelet/lipgloss` → `charm.land/lipgloss/v2`
- Style API is largely compatible but some methods may have changed signatures
- Color handling may differ

Files to update:
- [ ] `internal/tui/tui_shared.go` — shared style definitions
- [ ] `internal/tui/tui_plan_render.go` — render helpers using lipgloss
- [ ] `internal/tui/tui_panel_status.go` — status bar styling
- [ ] `internal/tui/tui_panel_work_overview.go` — work list styling
- [ ] `internal/tui/tui_panel_work_details.go` — details styling
- [ ] `internal/tui/tui_panel_work_summary.go` — summary styling
- [ ] `internal/tui/tui_panel_work_tabs.go` — tabs styling
- [ ] All other panel files using lipgloss styles
