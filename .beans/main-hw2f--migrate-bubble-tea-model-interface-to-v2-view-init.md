---
# main-hw2f
title: Migrate Bubble Tea Model interface to v2 (View, Init, Update)
status: todo
type: task
created_at: 2026-03-12T01:10:00Z
updated_at: 2026-03-12T01:10:00Z
parent: main-mtvk
blocked_by:
    - main-xh70
---

Update all tea.Model implementations to match v2 signatures.

Key changes:
- `View() string` → `View() tea.View` (wrap with `tea.NewView(s)`)
- `tea.Msg` is now `uv.Event` (alias, mostly transparent)
- AltScreen, Cursor, MouseMode are now declarative fields on `tea.View`

Files to update:
- [ ] `internal/tui/tui_root.go` — root model View/Update
- [ ] `internal/tui/tui_plan.go` — planModel View/Update
- [ ] `internal/tui/tui_plan_render.go` — all render helpers return string → update callers
- [ ] `internal/tui/tui_plan_dialogs.go` — dialog rendering
- [ ] `internal/tui/tui_plan_work.go` — work panel logic
- [ ] `internal/tui/tui_plan_data.go` — data loading commands
- [ ] `cmd/proj.go` — tea.NewProgram setup
