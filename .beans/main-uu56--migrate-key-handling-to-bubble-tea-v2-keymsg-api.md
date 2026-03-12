---
# main-uu56
title: Migrate key handling to Bubble Tea v2 KeyMsg API
status: todo
type: task
created_at: 2026-03-12T01:10:10Z
updated_at: 2026-03-12T01:10:10Z
parent: main-mtvk
blocked_by:
    - main-xh70
---

Bubble Tea v2 replaces the v1 KeyMsg type with an interface + concrete types.

Key changes:
- `tea.KeyMsg` is now an interface; concrete types are `tea.KeyPressMsg` and `tea.KeyReleaseMsg`
- `msg.Type` / `tea.KeyRunes` removed — use `msg.Key()` and key constants
- `msg.Runes` removed — use `msg.Text` for printable text
- `msg.String()` still works for display but matching logic changes
- Key constants moved: `tea.KeyEnter`, `tea.KeyEsc`, etc. still exist but types differ

Files to update:
- [ ] `internal/tui/tui_panel_session.go` — `keyMsgToBytes()` full rewrite for new Key API
- [ ] `internal/tui/tui_panel_session.go` — `Update(msg tea.KeyMsg)` signature
- [ ] `internal/tui/tui_plan.go` — all `case tea.KeyMsg` switch arms
- [ ] `internal/tui/tui_plan_dialogs.go` — dialog key handling
- [ ] `internal/tui/tui_panel_issues.go` — key handling
- [ ] `internal/tui/tui_panel_work_overview.go` — key handling
- [ ] `internal/tui/tui_panel_work_tabs.go` — key handling
- [ ] `internal/tui/tui_panel_work_details.go` — key handling
- [ ] `internal/tui/tui_panel_session_picker.go` — key handling
- [ ] `internal/tui/tui_panel_create_work.go` — key handling
- [ ] `internal/tui/tui_panel_bead_form.go` — key handling
- [ ] `internal/tui/tui_panel_pr_import.go` — key handling
- [ ] `internal/tui/tui_panel_linear_import.go` — key handling
