---
# main-gkh9
title: Replace RPC bridge with PTY + x/vt for interactive session display
status: completed
type: feature
priority: normal
created_at: 2026-03-04T00:13:20Z
updated_at: 2026-03-04T00:52:27Z
---

Replace the custom RPC event rendering in SessionPanel with embedded PTY output using charmbracelet/x/vt virtual terminal emulator.

## Context
Currently, interactive sessions (plan, agent, console) spawn pi in RPC mode, parse structured JSON events, and re-render a worse version of pi's UI in SessionPanel. The sequencer also uses bridge sessions for headless task execution.

## Approach
- Keep the bridge/RPC for **sequencer** (headless task execution) — it needs structured events (agent_end) and prompt/abort control, no UI needed
- For **interactive TUI sessions** (plan, agent, console), switch to PTY + x/vt:
  1. Spawn pi normally (not RPC mode) with a PTY via creack/pty
  2. Feed PTY output into a vt.Emulator (parses all ANSI)
  3. Render emulator.Render() in the session panel
  4. Forward keystrokes from TUI to the PTY
  5. Detect completion by process exit

## Todo
- [x] Add dependencies: creack/pty, charmbracelet/x/vt
- [x] Create internal/ptysession package with PTY session management
- [x] Rewrite SessionPanel to use vt.Emulator for rendering
- [x] Update OrchestratorManager interactive methods to spawn PTY sessions instead of bridge
- [x] Update planModel to use PTY sessions for interactive viewing
- [x] Wire up keyboard input forwarding from TUI to PTY
- [x] Handle resize propagation to PTY + vt.Emulator
- [x] Test plan, agent, and console session types (manual testing needed)

## Summary of Changes

### New packages
- `internal/ptysession/session.go` — PTY session backed by `creack/pty` + `x/vt` emulator. Handles spawn, I/O forwarding, resize, render, and async output notification.
- `internal/ptysession/manager.go` — Session registry (spawn, get, kill, list) analogous to `bridge.Bridge`.

### Modified files
- `internal/tui/tui_panel_session.go` — Completely rewritten. Now just renders `session.Render()` and forwards all key input to PTY via `keyMsgToBytes()`. No more custom event parsing, text accumulation, viewport management, or input modes.
- `internal/tui/tui_plan.go` — Replaced `bridgeEventMsg` with `ptyOutputMsg`. Added `ptyManager` and `teaProgram` fields. Replaced `viewBridgeSession`/`waitForBridgeEvent` with `viewPTYSession`. PTY output callback wakes Bubbletea via `Program.Send()`.
- `internal/tui/tui_plan_work.go` — Updated spawn/open methods to check PTY manager instead of bridge. Updated msg types.
- `internal/tui/tui_root.go` — Wires `tea.Program` reference to planModel after creation.
- `internal/work/orchestrator.go` — Interface now returns `*ptysession.Session`. `DefaultOrchestratorManager` holds both bridge (headless) and PTY manager (interactive).
- `internal/work/tabs.go` — Rewrote `OpenConsole`, `OpenAgentSession`, `SpawnPlanSession` to use PTY sessions. Added `buildPTYConfig` helper.

### Architecture
- **Bridge/RPC**: Preserved for sequencer only (headless task execution with structured events)
- **PTY + x/vt**: Used for all interactive sessions (plan, agent, console) — pi renders its own UI natively
