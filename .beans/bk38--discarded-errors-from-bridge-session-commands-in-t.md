---
# bk38
title: Discarded errors from bridge session commands in TUI
status: completed
type: task
priority: normal
tags:
    - review-w-2fo.14
created_at: 2026-03-01T22:54:09Z
updated_at: 2026-03-02T21:37:03Z
parent: ir72
---

In internal/tui/tui_plan.go:1206,1216,1225, errors from session.Abort(), session.Steer(), and session.Prompt() are silently discarded. These should set m.statusMessage/m.statusIsError so the user sees feedback when commands fail.

## Summary of Changes\n\nHandled errors from session.Abort(), session.Steer(), and session.Prompt() in tui_plan.go. Errors now set m.statusMessage and m.statusIsError so users see feedback when commands fail.
