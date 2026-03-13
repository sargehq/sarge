---
# main-jybl
title: 'Fix TUI hotkeys: use alt+ instead of ctrl+ to avoid pi hotkey conflicts'
status: scrapped
type: bug
priority: normal
created_at: 2026-03-13T12:16:15Z
updated_at: 2026-03-13T13:19:03Z
---

The ctrl+ hotkeys conflict with pi session hotkeys (ctrl+c, ctrl+d, ctrl+z, ctrl+p, ctrl+t, ctrl+o, ctrl+g, ctrl+v, ctrl+l, ctrl+k). Need to use alt+letter instead since pi only uses alt+enter and alt+up.

## Todo
- [x] Change all ctrl+key hotkeys to alt+key in tui_plan.go
- [x] Change all ctrl+key hotkeys to alt+key in tui_panel_work_details.go
- [x] Update session interception to catch alt+key instead of ctrl+key
- [x] Update status bar labels in tui_panel_status.go (⌥ prefix)
- [x] Update help screen in tui_plan_render.go
- [x] Update work task panel hint in tui_panel_work_task.go
- [x] Update mouse click handlers to send ModAlt
- [x] Update tui_root.go quit key to alt+q
- [x] Keep ctrl+shift+1-9 for sub-sessions (no pi conflict)
- [x] Build and test pass

## Summary of Changes
Changed all TUI hotkeys from ctrl+ to alt+ to avoid conflicts with pi session hotkeys (ctrl+c, ctrl+d, ctrl+z, ctrl+p, ctrl+t, ctrl+o, ctrl+g, ctrl+v, ctrl+l, ctrl+k). Pi only uses alt+enter and alt+up, so all alt+letter combos are free for TUI use.

## Reasons for Scrapping
alt/option key on macOS is treated as a compose/dead key, not as Alt modifier. ModAlt is never set. alt+key is completely unusable.
