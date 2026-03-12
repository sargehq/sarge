---
# main-zkqk
title: Auto work not started after worktree creation
status: in-progress
type: bug
created_at: 2026-03-12T16:41:57Z
updated_at: 2026-03-12T16:41:57Z
---

When work is created with auto=true, the control plane creates the worktree successfully but never triggers the automated workflow (estimate → implement). The auto-setup code was inside executeTask() in the sequencer, which only runs when there's already a task — but for fresh auto work there are no tasks yet, so it was unreachable dead code.

Fix: move the auto workflow setup into tryStartNextTask(), where the sequencer checks for ready tasks. When no ready tasks exist for an auto work with a ready worktree and zero tasks, create the estimate task. Remove the dead code from executeTask().

- [x] Move auto workflow setup from executeTask to tryStartNextTask
- [x] Remove dead code in executeTask
- [x] Verify build and tests pass
