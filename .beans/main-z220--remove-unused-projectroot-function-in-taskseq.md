---
# main-z220
title: Remove unused projectRoot function in taskseq
status: todo
type: task
tags:
    - review-w-2fo.8
created_at: 2026-03-01T22:41:22Z
updated_at: 2026-03-01T22:41:22Z
parent: ir72
---

internal/taskseq/sequencer.go:819 defines projectRoot() which is never called. Dead code.
