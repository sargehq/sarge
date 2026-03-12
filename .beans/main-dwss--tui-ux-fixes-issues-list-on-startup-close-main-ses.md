---
# main-dwss
title: 'TUI UX fixes: issues list on startup, close main session, click main tab'
status: completed
type: bug
priority: normal
created_at: 2026-03-12T12:38:18Z
updated_at: 2026-03-12T12:39:58Z
---

Three UX issues:
1. When the TUI starts for the first time there's no issues list visible
2. When the main session is open there's no way to close it
3. Clicking on the main work tab doesn't actually work

## Tasks
- [x] Fix issues list not showing on first start (Main tab takes over full screen)
- [x] Add a way to close/exit the main session (e.g., Esc key)
- [x] Fix clicking on the Main tab in the work tabs bar

## Summary of Changes

Modified `internal/tui/tui_plan.go` to fix three UX issues:

1. **Issues list not showing on first start**: Changed `View()` so WorkTabDefault only renders fullscreen session when `activePanel == PanelSession`, otherwise falls through to the normal two-column layout. Since the initial state is `PanelLeft`, users now see the issues list on startup.

2. **No way to close main session**: Updated the Esc key handler so pressing Esc while in the Main tab's session view sets `activePanel = PanelLeft`, returning to the issues list.

3. **Clicking Main tab doesn't work**: Fixed `activateTab()` to toggle the Main tab between session view and issues list when clicked while already active. Also updated `IsShowingSession()` and `hasRawSessionContent()` to be consistent with the new Main tab behavior.
