---
# main-ab2m
title: 'Phase 4: TUI session viewer and interaction panel'
status: todo
type: feature
created_at: 2026-03-01T21:27:52Z
updated_at: 2026-03-01T21:27:52Z
parent: ir72
blocked_by:
    - main-nies
---

Build the TUI panel that renders pi session output and accepts user input, replacing external terminal tabs.

## Overview
With zmx/zellij removed, the sarge TUI becomes the only interface for viewing agent activity and interacting with pi sessions. This phase adds a session viewer/interaction panel to the existing Bubbletea TUI.

## What to Build

### 1. Session viewer panel
A new TUI panel (like the existing work details, issues, etc.) that:
- Shows streaming text output from a pi bridge session
- Renders tool execution events (bash commands, file edits, reads)
- Shows thinking output (if thinking is enabled)
- Supports scrollback through session history
- Auto-scrolls to bottom during active streaming

### 2. Session interaction
For interactive sessions (agent sessions, plan sessions):
- Text input at the bottom for sending prompts
- Support for `steer` (interrupt) via a keybinding
- Support for `abort` via a keybinding
- Handle `extension_ui_request` dialogs (select, confirm, input)

### 3. Session switching
- The work tabs bar already shows works; extend it or add a sub-tab for sessions within a work
- Show session type indicators: orch (task execution), agent (interactive), plan
- Click/keyboard to switch between sessions
- Show streaming status (spinner when active)

### 4. Replace ZmxPickerPanel
- `internal/tui/tui_panel_zmx_picker.go` currently picks zmx sessions to attach to
- Replace with a bridge session picker that shows active bridge sessions
- No more "attach" concept — just switch focus to the session in the TUI

## Key Files to Create
- `internal/tui/tui_panel_session.go` — Session viewer/interaction panel
- `internal/tui/tui_panel_session_picker.go` — Session picker (replaces zmx picker)

## Key Files to Modify  
- `internal/tui/tui_plan.go` — Add session panel, wire bridge events
- `internal/tui/tui_plan_work.go` — Replace zmx session handling with bridge sessions
- `internal/tui/tui_plan_render.go` — Render session panel in layout
- `internal/tui/tui_shared.go` — Add Panel enum for session panel

## Event Flow
1. Bridge session emits events on a Go channel
2. TUI subscribes to the active session's events via `tea.Cmd`
3. `message_update` events with `text_delta` append to the session buffer
4. `tool_execution_*` events render tool call UI (command, output, result)
5. `agent_end` shows completion status

## Todo
- [ ] Create session viewer panel with scrollable output
- [ ] Render text deltas, thinking, and tool executions
- [ ] Add text input for interactive sessions
- [ ] Wire bridge events into Bubbletea message loop
- [ ] Add session picker/switcher (replaces zmx picker)
- [ ] Handle extension UI requests (select, confirm, input dialogs)
- [ ] Add keybindings for abort, steer
- [ ] Update layout to include session panel
