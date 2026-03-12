---
# main-09bn
title: Migrate Bubbles components and Huh forms to v2
status: todo
type: task
created_at: 2026-03-12T01:10:24Z
updated_at: 2026-03-12T01:10:24Z
parent: main-mtvk
blocked_by:
    - main-xh70
---

Update bubbles (spinner, viewport, etc.) and huh form usage to v2.

Key changes:
- Import: `github.com/charmbracelet/bubbles/*` → `charm.land/bubbles/v2/*`
- Import: `github.com/charmbracelet/huh` → `charm.land/huh/v2`
- Component Init/Update/View signatures follow bubbletea v2 conventions

Files to update:
- [ ] `internal/tui/tui_plan.go` — spinner usage
- [ ] `internal/tui/tui_panel_work_tabs.go` — spinner in tabs
- [ ] `internal/tui/tui_panel_bead_form.go` — huh form usage
- [ ] `internal/tui/tui_panel_create_work.go` — huh form usage
- [ ] `internal/tui/tui_panel_details.go` — viewport or similar
- [ ] `internal/beans/pubsub/tea.go` — tea integration helpers
- [ ] `internal/beans/pubsub/tea_test.go` — update tests
