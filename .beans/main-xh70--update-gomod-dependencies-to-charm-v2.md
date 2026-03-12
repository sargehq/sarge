---
# main-xh70
title: Update go.mod dependencies to Charm v2
status: todo
type: task
created_at: 2026-03-12T01:09:52Z
updated_at: 2026-03-12T01:09:52Z
parent: main-mtvk
---

Replace all Charmbracelet v1 dependencies with v2 equivalents.

- [ ] Add `charm.land/bubbletea/v2` v2.0.2
- [ ] Add `charm.land/lipgloss/v2` v2.0.2
- [ ] Add `charm.land/bubbles/v2` v2.0.0
- [ ] Add `charm.land/huh/v2` v2.0.3
- [ ] Remove old `github.com/charmbracelet/bubbletea` v1 deps
- [ ] Remove old `github.com/charmbracelet/lipgloss` v1 deps
- [ ] Remove old `github.com/charmbracelet/bubbles` v1 deps
- [ ] Remove old `github.com/charmbracelet/huh` v0.8
- [ ] Run `go mod tidy` and verify dependency graph
- [ ] Ensure `x/vt` remains compatible (still on github.com/charmbracelet path)
