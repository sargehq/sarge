---
# main-qxkd
title: GetPendingPrompt return value discarded in session panel
status: completed
type: bug
priority: normal
tags:
    - review-w-2fo.8
created_at: 2026-03-01T22:41:22Z
updated_at: 2026-03-01T22:42:44Z
parent: ir72
---

Removed premature GetPendingPrompt() call in session panel enter handler; the parent (tui_plan.go) already calls GetPendingPrompt() when it receives SessionPanelActionPrompt, so calling it in the handler was consuming and discarding the prompt text.
