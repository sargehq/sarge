---
# main-14yj
title: Sequencer uses headless bridge sessions instead of viewable PTY sessions
status: completed
type: bug
created_at: 2026-03-12T17:07:15Z
updated_at: 2026-03-12T17:07:15Z
---

The task sequencer spawned pi in RPC mode via bridge sessions, which are headless and invisible in the TUI. Replaced with PTY sessions that use InitialPrompt to send the task prompt. PTY sessions are visible in the TUI session viewer, so users can watch agent progress.

- [x] Replace bridge.Bridge with ptysession.Manager in Sequencer
- [x] Spawn PTY sessions with InitialPrompt instead of bridge RPC sessions
- [x] Wait for process exit instead of bridge EventAgentEnd
- [x] Update killAllActiveSessions to use ptyManager
- [x] Update TUI to pass ptyManager to sequencer
- [x] Fix tests

## Summary of Changes

Replaced the sequencer's bridge-based (headless RPC) session spawning with PTY sessions. Task sessions now appear in the TUI session viewer with IDs like task-w-xxx.N, so users can see agent output in real time. The pi process runs normally (not RPC mode) and exits when done, eliminating the prior hang issue as well.
