---
# main-nies
title: 'Phase 3: Route plan and interactive sessions through bridge'
status: completed
type: feature
priority: normal
created_at: 2026-03-01T21:27:30Z
updated_at: 2026-03-01T22:00:55Z
parent: ir72
blocked_by:
    - main-otx3
---

Wire plan sessions and interactive agent sessions through the pi bridge instead of zmx/zellij tabs.

## Overview
Currently, plan sessions (`sarge plan <beanID>`) and interactive agent/console sessions are spawned as terminal tabs in zmx or zellij. This phase routes them through the bridge so sarge manages all pi sessions directly.

## What Changes

### 1. Plan sessions
Current flow (`cmd/plan.go` → `internal/work/tabs.go:SpawnPlanSession`):
- Creates a zmx session or zellij tab running `sarge plan <beanID>`
- The plan command (`cmd/plan.go`) then runs `agent.RunInteractive()` which does `exec.Command("pi", prompt)`

New flow:
- `SpawnPlanSession` creates a bridge session instead
- Sends the plan prompt via the bridge RPC
- The TUI renders streaming output from the bridge session events

### 2. Interactive agent sessions
Current flow (`internal/work/tabs.go:OpenAgentSession`):
- Creates a zmx/zellij tab running `pi` (or `claude`) interactively
- User interacts directly in the terminal tab

New flow:
- `OpenAgentSession` creates a bridge session
- The sarge TUI provides the interaction surface (prompt input, streaming output)
- User sends messages via `prompt` RPC command
- Agent responses stream back via events

### 3. Console sessions
Current flow (`internal/work/tabs.go:OpenConsole`):
- Creates a zmx/zellij tab with a shell in the worktree

New flow:
- Console sessions can remain as simple `exec.Command` with PTY, or be deferred
- Alternative: embed a terminal emulator in the TUI (bigger scope, may defer)

### 4. OrchestratorManager interface changes
The `OrchestratorManager` interface (`internal/work/orchestrator.go`) currently has:
- `SpawnPlanSession()` — creates zmx/zellij tab
- `OpenConsole()` — creates zmx/zellij tab  
- `OpenAgentSession()` — creates zmx/zellij tab
- `ListWorkSessions()` — lists zmx sessions
- `AttachToSession()` — attaches to zmx session

These all need to be refactored to use the bridge:
- `SpawnPlanSession()` → create bridge session, send plan prompt
- `OpenAgentSession()` → create bridge session (interactive)
- `ListWorkSessions()` → list bridge sessions
- Remove `AttachToSession()` (no more external terminal attachment)
- `OpenConsole()` — TBD (may keep as exec or embed PTY)

## Key Files to Modify
- `internal/work/orchestrator.go` — OrchestratorManager, remove zmx/zellij deps
- `internal/work/tabs.go` — SpawnPlanSession, OpenConsole, OpenAgentSession
- `cmd/plan.go` — Plan command (use bridge instead of RunInteractive)
- `internal/tui/tui_plan_work.go` — TUI handlers for plan/agent/console (use bridge)
- `internal/tui/tui_plan.go` — Remove zmx/zellij fields, add bridge field

## Todo
- [ ] Refactor SpawnPlanSession to use bridge
- [ ] Refactor OpenAgentSession to use bridge  
- [ ] Refactor ListWorkSessions to query bridge sessions
- [ ] Remove AttachToSession (no longer needed)
- [ ] Decide on console session approach (defer or PTY)
- [ ] Update TUI to connect to bridge sessions for plan/agent
- [ ] Remove zmx.Client and zellij.SessionManager from OrchestratorManager
