---
# main-8xf1
title: Verify VT emulator rendering with v2 cell-buffer renderer
status: todo
type: task
created_at: 2026-03-12T01:10:32Z
updated_at: 2026-03-12T01:10:32Z
parent: main-mtvk
blocked_by:
    - main-hw2f
    - main-uu56
    - main-h4zi
    - main-09bn
---

After migration, verify the VT emulator flickering is resolved.

- [ ] Run sarge TUI with an active PTY session
- [ ] Confirm session panel output is stable (no flickering/scrolling)
- [ ] Test rapid output scenarios (e.g. pi startup, large file display)
- [ ] Test terminal resize during active session
- [ ] Test session switching between multiple PTY sessions
- [ ] Verify input forwarding still works correctly with new KeyMsg API
- [ ] Check if `x/vt` Render() output is efficiently consumed by v2 renderer
- [ ] Consider if `x/vt` can expose cell buffer directly to skip ANSI round-trip (future optimization)
