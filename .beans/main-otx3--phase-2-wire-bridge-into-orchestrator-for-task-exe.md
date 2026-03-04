---
# main-otx3
title: 'Phase 2: Single-process sarge — bridge execution, in-process control plane, task sequencer'
status: completed
type: feature
priority: normal
created_at: 2026-03-01T21:27:06Z
updated_at: 2026-03-01T22:10:42Z
parent: ir72
blocked_by:
    - main-wzfv
---

Wire the pi bridge into sarge as the task execution engine, replacing the orchestrator process. Move the control plane in-process. Sarge becomes a single process: TUI + bridge + control plane.

## Architecture Change
**Before**: 3 separate processes per project
- `sarge` TUI process
- `sarge control` process (background tasks, spawned in zmx/zellij tab)
- `sarge orchestrate --work <id>` process per work (task execution, spawned in zmx/zellij tab)

**After**: 1 sarge process
- TUI (Bubbletea)
- Bridge goroutine (manages pi RPC sessions for task execution)
- Control plane goroutine (background tasks: git, GitHub API, CI watchers)
- Task sequencer (replaces orchestrator: polls ready tasks, sends to bridge)

## What Changes

### 1. Remove orchestrator as separate process
The orchestrator (`cmd/orchestrate.go`) currently:
- Polls `DB.GetReadyTasksForWork()` for a single work
- Calls `agent.Run()` (fork/exec pi) sequentially
- Handles post-task logic (post-estimation, review loops)
- Runs in a zmx/zellij tab

Replace with an in-process task sequencer that:
- Watches all works for ready tasks (via DB watcher events, like control plane does)
- Spawns bridge sessions to execute tasks
- Monitors `agent_end` events for completion
- Runs post-task logic inline
- Lives as a goroutine in the main sarge process

### 2. Move control plane in-process
`internal/control/loop.go` `RunControlPlaneLoop()` is already a clean event loop:
- Subscribes to DB watcher events
- Processes scheduled tasks (git push, GitHub API, worktree ops, CI watching)
- Has async executor for long-running tasks
- Periodic timers for safety-net checks and cleanup

This can run as a goroutine started by the main sarge process. Changes:
- Remove `cmd/control_plane.go` command (or keep as fallback)
- Remove `control.EnsureControlPlane()` / `ensureControlPlaneZmx()` / `ensureControlPlaneZellij()` from `internal/control/spawn.go`
- Start control plane loop as goroutine from `cmd/run.go` or wherever sarge TUI starts
- Remove the `HandleSpawnOrchestratorTask` handler (no longer spawns orchestrator processes)
- Keep heartbeat/procmon for crash detection, but simplified (single process)

### 3. Task sequencer (replaces orchestrator)
New component, similar to `cmd/orchestrate.go` but:
- Manages ALL works (not one work per process)
- Uses bridge sessions instead of fork/exec
- Event-driven via DB watcher (not polling)

Core loop per work:
```
1. DB watcher signals task change
2. For each work with ready tasks:
   a. If work already has an active bridge session → skip (task running)
   b. Pick next ready task
   c. Build prompt via agent.BuildPrompt()
   d. Spawn bridge session, send prompt
   e. On agent_end → check task status, run post-task logic
   f. Loop to pick next task
```

### 4. Remove SpawnOrchestrator flow
Currently: control plane handles `TaskTypeSpawnOrchestrator` by creating a zmx/zellij tab running `sarge orchestrate --work <id>`.
After: Remove this task type entirely. The in-process task sequencer handles all works directly.

Files affected:
- `internal/control/task_types.go` — remove `TaskTypeSpawnOrchestrator`
- `internal/control/plane.go` — remove `HandleSpawnOrchestratorTask`
- `internal/db/` — remove spawn_orchestrator scheduled task type
- `cmd/work.go` — work creation no longer schedules spawn_orchestrator task

### 5. Adapt agent execution
Currently `internal/agents/pi/pi.go` `Run()` does:
1. `BuildPrompt()` → get prompt string
2. `database.StartTask()` → mark task as processing
3. `exec.CommandContext("pi", prompt)` → fork/exec
4. `runner.MonitorAgent()` → wait for exit

New flow:
1. `BuildPrompt()` → get prompt string (unchanged)
2. `database.StartTask()` → mark task as processing
3. `bridge.SpawnSession(id, workDir, cfg)` → create RPC session
4. `session.Prompt(prompt)` → send via RPC
5. Listen for `agent_end` event → check task status, handle completion/failure

## Key Files to Create
- `internal/taskseq/sequencer.go` — Task sequencer (replaces orchestrator)

## Key Files to Modify
- `cmd/run.go` (or main TUI entry) — Start control plane + task sequencer as goroutines
- `internal/control/loop.go` — Make startable as goroutine (already mostly there)
- `internal/control/spawn.go` — Remove EnsureControlPlane zmx/zellij logic
- `internal/control/plane.go` — Remove OrchestratorSpawner, HandleSpawnOrchestratorTask
- `internal/agents/pi/pi.go` — Add bridge-based execution method
- `internal/agents/runner/runner.go` — Replace or adapt for bridge monitoring

## Key Files to Remove/Deprecate
- `cmd/orchestrate.go` — Entire orchestrate command (replaced by in-process sequencer)
- `cmd/control_plane.go` — Control command (moved in-process)

## Todo
- [ ] Create internal/taskseq/sequencer.go — event-driven task sequencer for all works
- [ ] Adapt pi agent to execute via bridge session instead of exec.Command
- [ ] Move control plane loop startup into main sarge process
- [ ] Remove HandleSpawnOrchestratorTask and TaskTypeSpawnOrchestrator
- [ ] Remove cmd/orchestrate.go command
- [ ] Remove cmd/control_plane.go command (or keep as debug fallback)
- [ ] Remove EnsureControlPlane zmx/zellij spawn logic
- [ ] Integrate task sequencer with bridge for pi session management
- [ ] Handle post-task logic (post-estimation, review loops) in sequencer
- [ ] Handle task timeouts through bridge abort
- [ ] Test: task execution lifecycle through bridge
- [ ] Test: control plane background tasks running in-process
