---
# r1bm
title: Fix errcheck and gosec lint errors in tui_root.go
status: completed
type: bug
priority: normal
created_at: 2026-03-01T22:59:25Z
updated_at: 2026-03-02T21:36:37Z
parent: ir72
---

3 lint errors in internal/tui/tui_root.go: (1) Line 214: Error return value of seqWatcher.Stop is not checked (errcheck). (2) Line 235: Error return value of sequencer.Run is not checked (errcheck). (3) Line 176: G104: Errors unhandled - os.Setenv call (gosec).

## Summary of Changes\n\nFixed 3 lint errors in tui_root.go:\n1. os.Setenv error now suppressed with nolint comment\n2. seqWatcher.Stop() error now checked and logged\n3. sequencer.Run() error now checked and logged
