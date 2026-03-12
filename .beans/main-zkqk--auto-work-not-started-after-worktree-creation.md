---
# main-zkqk
title: Auto work not started after worktree creation
status: completed
type: bug
priority: normal
created_at: 2026-03-12T16:41:57Z
updated_at: 2026-03-12T16:49:39Z
---

When work is created with auto=true, the control plane creates the worktree successfully but never triggers the automated workflow (estimate → implement). The auto-setup code was inside executeTask() in the sequencer, which only runs when there's already a task — but for fresh auto work there are no tasks yet, so it was unreachable dead code.

Fix: move the auto workflow setup into tryStartNextTask(), where the sequencer checks for ready tasks. When no ready tasks exist for an auto work with a ready worktree and zero tasks, create the estimate task. Remove the dead code from executeTask().

- [x] Move auto workflow setup from executeTask to tryStartNextTask
- [x] Remove dead code in executeTask
- [x] Verify build and tests pass

## Summary of Changes

Moved auto workflow initialization from `executeTask()` (dead code — unreachable for fresh work with no tasks) to `tryStartNextTask()` in the sequencer. Now when the sequencer polls and finds an auto work with a ready worktree but zero tasks, it creates the estimate task to kick off the automated workflow.

## Additional Fix: Sequencer stuck after task completion

The sequencer's `waitForCompletion` called `session.Wait()` after receiving `EventAgentEnd`, which blocks until the pi process exits. But in RPC mode, pi stays alive waiting for the next prompt after finishing a turn — it never exits on its own. This caused the sequencer goroutine to hang indefinitely after every task completion.

Fix: return `nil` on `EventAgentEnd` instead of waiting for process exit. The caller already calls `KillSession` to clean up the process.

- [x] Fix waitForCompletion to not block on process exit after EventAgentEnd
