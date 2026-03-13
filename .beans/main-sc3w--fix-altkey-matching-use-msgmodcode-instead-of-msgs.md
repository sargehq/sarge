---
# main-sc3w
title: 'Fix alt+key matching: use msg.Mod/Code instead of msg.String()'
status: scrapped
type: bug
priority: normal
created_at: 2026-03-13T12:24:29Z
updated_at: 2026-03-13T13:19:07Z
---

msg.String() returns the Text field which doesn't include modifier info for alt+key. Need to check msg.Mod&ModAlt and msg.Code directly.

## Todo
- [x] Add altKey() and altShiftKey() helpers in tui_shared.go
- [x] Refactor tui_plan.go: extracted handleAltKey() and handleAltShiftKey() methods
- [x] Refactor tui_panel_work_details.go: extracted handleAltAction() method
- [x] Session interception uses altKey()/altShiftKey() helpers
- [x] tui_root.go uses altKey() for quit
- [x] Build and test pass

## Summary of Changes
msg.String() returns the Text field which doesn't include modifier info. For alt+n on macOS, String() might return 'ñ' or just 'n' — never 'alt+n'. Fixed by checking msg.Mod & msg.Code directly via helper functions.

## Reasons for Scrapping
alt+key approach was fundamentally broken on macOS. Switched to ctrl+key with session-blocks-all-except-ESC approach instead.
