---
# main-7p67
title: Bridge session silently drops events when channel full
status: todo
type: task
tags:
    - review-w-2fo.8
created_at: 2026-03-01T22:41:22Z
updated_at: 2026-03-01T22:41:22Z
parent: ir72
---

In internal/bridge/session.go:300-302, events are silently dropped when the 256-buffer channel is full. Dropped agent_end events could leave TUI in stuck streaming state. Consider adding a drop counter/log and increasing buffer.
