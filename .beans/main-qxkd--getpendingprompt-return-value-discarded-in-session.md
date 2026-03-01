---
# main-qxkd
title: GetPendingPrompt return value discarded in session panel
status: todo
type: bug
tags:
    - review-w-2fo.8
created_at: 2026-03-01T22:41:22Z
updated_at: 2026-03-01T22:41:22Z
parent: ir72
---

In internal/tui/tui_panel_session.go:444, GetPendingPrompt() is called for its side effect but the return value (actual prompt text) is discarded. The caller returns SessionPanelActionPrompt but the parent has no way to retrieve what was typed since the text is consumed and reset inside GetPendingPrompt.
