---
# ir72
title: 'sarge v2: the sage begins'
status: completed
type: epic
priority: normal
created_at: 2026-03-01T21:22:15Z
updated_at: 2026-03-01T21:44:41Z
---

The primary interface to search becomes a pi session.
I want to build a pi bridge that surge connects with Unix domain sockets to the bridge. The bridge starts a pi instance which uses the standard in and standard out RPC mechanism.
Remove ZMX and Zellage support.
I want Sarge to act as a multiplexer for the Pi sessions, Talking through the bridge.

## Plan

Replace zmx/zellij terminal multiplexer architecture with a pi bridge that manages pi RPC sessions directly. Sarge becomes the session multiplexer, with the TUI as the primary user interface.

### Architecture Change
- **Before**: 3+ processes — sarge TUI + sarge control (background) + sarge orchestrate per work (in zmx/zellij tabs) + pi/claude (fork/exec)
- **After**: 1 process — sarge (TUI + control plane goroutine + task sequencer + pi bridge managing RPC sessions)

### Phase Breakdown (sequential, each blocks the next)
1. **Phase 1** (main-wzfv): Build `internal/bridge` package — spawn pi RPC processes, JSON protocol, session management
2. **Phase 2** (main-otx3): Single-process sarge — bridge execution, in-process control plane, task sequencer (replaces orchestrator + control plane processes)
3. **Phase 3** (main-nies): Route plan sessions & interactive agents through bridge
4. **Phase 4** (main-ab2m): TUI session viewer panel — render pi output, accept user input, replace external terminals
5. **Phase 5** (main-pute): Remove all zmx/zellij code — delete packages, clean 34+ files, drop config sections
6. **Phase 5b** (main-to0t): Remove Claude agent backend — delete claude package, simplify agent layer, pi-only

### Key Technical Details
- Pi RPC protocol: JSON lines over stdin/stdout (`pi --mode rpc`)
- Supports: prompt, abort, steer, follow_up, get_state commands
- Streams: message_update (text_delta), tool_execution_*, agent_start/end events
- ~564 lines of zmx/zellij references across 34+ files to clean up
