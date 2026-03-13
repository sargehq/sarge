---
# main-kmwe
title: Change TUI hotkeys to ctrl-/cmd- prefixes for pi session compatibility
status: completed
type: task
priority: normal
created_at: 2026-03-13T04:23:26Z
updated_at: 2026-03-13T13:19:24Z
---

Pi sessions need complete keyboard control, so all TUI hotkeys must use ctrl- or cmd- modifiers instead of bare letters.

## Todo
- [x] Update handleKeyPress in tui_plan.go to use ctrl- prefixed keys
- [x] Update status bar labels in tui_panel_status.go
- [x] Update work details panel hotkeys in tui_panel_work_task.go and tui_panel_work_details.go
- [x] Update help screen text in tui_plan_render.go
- [x] Intercept ctrl+key combos in session handler before PTY forwarding
- [x] Update mouse click handlers to send ctrl+key
- [x] Update tui_root.go quit key
- [x] Build and test pass

## Summary of Changes
Changed all TUI action hotkeys from bare letters to ctrl+ prefixed keys so pi sessions get complete keyboard control. Bare keys in a session panel are forwarded to the PTY; ctrl+key combos are intercepted by the TUI regardless of active panel.

### Key mapping:
- Issue actions: ctrl+n (new), ctrl+e (edit), ctrl+a (child), ctrl+x (close), ctrl+d (delete), ctrl+w (work), ctrl+p (plan)
- Work actions: ctrl+t (term), ctrl+c (chat), ctrl+i (IDE), ctrl+r (run), ctrl+o (orch), ctrl+v (review), ctrl+f (feedback), ctrl+g (session)
- Other: ctrl+s (sort), ctrl+z (maximize), ctrl+q (quit), ctrl+shift+e (editor), ctrl+shift+m (PR import), ctrl+shift+a (add to work), ctrl+shift+1-9 (sub-sessions)
- Unchanged: j/k/arrows (nav), tab (cycle), 1-9 (tabs), space (select), esc, O/C/R/V/L (filters), / (search), ? (help), [ ] (resize)

## Revised approach
Use ctrl+key for all TUI hotkeys. When a pi session is focused, only ESC exits — all other keys (including ctrl+key) go to the session. This gives pi complete control when active, while ctrl+key works everywhere else.

## Summary of Changes
All TUI action hotkeys now use ctrl+key (detected via msg.Mod/msg.Code helpers). When a pi session panel is focused, only ESC exits — all other keys go to the PTY, giving pi complete control. Verified with keytest that ctrl+key produces ModCtrl=true reliably on macOS with Kitty protocol.
