---
# main-1mhq
title: PTY session not terminated when sarge complete runs in TUI
status: completed
type: bug
priority: normal
created_at: 2026-03-12T18:21:49Z
updated_at: 2026-03-12T18:22:20Z
---

When the pi agent calls `sarge complete` during TUI task execution, it marks the task as completed in the DB but the PTY session keeps running. The old `runner.MonitorAgent` watched for DB task status changes and sent SIGTERM, but the sequencer's `executeTask()` only waits for process exit or context cancellation.

## Root Cause
In `internal/taskseq/sequencer.go`, `executeTask()` spawns a PTY session and then does:
```go
select {
case <-exited:
case <-taskCtx.Done():
}
```
No case watches for task status changes in the DB.

## Fix
Add a DB watcher goroutine that polls/watches for task completion and kills the PTY session when detected. Use the existing tracking watcher infrastructure.

## Todo
- [x] Add DB status monitoring in executeTask select loop
- [x] Kill PTY session when task status changes to completed/failed
- [x] Verify build compiles

## Summary of Changes

Added a DB status monitoring goroutine in the sequencer's `executeTask()`. It polls every 2 seconds for task status changes. When the task is marked completed/failed (by `sarge complete`), it gives the agent 5 seconds to exit gracefully, then kills the PTY session. This matches the behavior of the old `runner.MonitorAgent` but adapted for the PTY-based sequencer.
