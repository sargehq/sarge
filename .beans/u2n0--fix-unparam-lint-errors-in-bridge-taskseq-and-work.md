---
# u2n0
title: Fix unparam lint errors in bridge, taskseq, and work packages
status: completed
type: bug
priority: normal
created_at: 2026-03-01T22:59:25Z
updated_at: 2026-03-02T21:47:22Z
parent: ir72
---

6 unparam lint errors: (1) internal/bridge/bridge_test.go:17 - writeMockPi result 0 (string) never used. (2) internal/taskseq/sequencer.go:349 - waitForCompletion taskID unused. (3) internal/taskseq/sequencer_test.go:52 - createTestWork auto always false. (4-6) internal/work/tabs.go lines 36,121,195 - ctx param unused in openConsoleBridge, openAgentSessionBridge, spawnPlanSessionBridge.

## Summary of Changes\n\nFixed 6 unparam lint errors:\n- Removed unused return value from writeMockPi in bridge_test.go\n- Used _ for unused taskID param in waitForCompletion in sequencer.go\n- Removed always-false auto param from createTestWork in sequencer_test.go\n- Used _ for unused ctx params in openConsoleBridge, openAgentSessionBridge, spawnPlanSessionBridge in tabs.go
