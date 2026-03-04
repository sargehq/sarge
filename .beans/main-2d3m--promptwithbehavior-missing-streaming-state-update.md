---
# main-2d3m
title: PromptWithBehavior missing streaming state update
status: completed
type: bug
priority: normal
tags:
    - review-w-2fo.8
created_at: 2026-03-01T22:41:22Z
updated_at: 2026-03-01T22:41:53Z
parent: ir72
---

In internal/bridge/session.go:206, PromptWithBehavior() does not call s.streaming.Store(true) unlike Prompt() at line 198. This means callers checking IsStreaming() will get incorrect state when PromptWithBehavior is used.
