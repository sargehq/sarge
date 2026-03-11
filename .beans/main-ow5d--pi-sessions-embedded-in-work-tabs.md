---
# main-ow5d
title: Pi sessions embedded in work tabs
status: completed
type: feature
priority: high
created_at: 2026-03-11T18:51:06Z
updated_at: 2026-03-11T19:00:37Z
---

## Overview

Rework the TUI so that each work tab embeds a pi PTY session alongside the work details panel. Plan sessions also get their own work tab. A default/global pi session tab is always present at startup.

## Design

### Tab Types
1. **Default tab** — Always-present global pi session ("Main"), spawned at TUI startup. For managing the overall session/running commands in the main repo.
2. **Work tabs** — One per active work. Shows work details + embedded pi session(s). The pi session panel can be maximized.
3. **Plan tabs** — Created on `p` key for a bean. Shows a planning pi session as its own tab in the work tabs bar.

### Within a Work Tab
- Work details panel (tasks, beans, orchestrator health) shown as a panel
- Pi session(s) shown as a split within the work: agent, console, plan sub-sessions
- A way to maximize the session view (toggle between split and full)

### Session Lifecycle
- **Work sessions**: pi session spawns automatically when needed (work creation/run)
- **Plan sessions**: spawned on `p` key press, appears as new tab
- **Default session**: spawned at TUI startup, always available

## Implementation Plan

### Phase 1: Tab Model
- [x] Create WorkTab struct with types: Default, Work, Plan
- [x] Each tab holds references to its pi session(s) and work data
- [x] Update WorkTabsBar to render new tab types (Main, work tabs, plan tabs)

### Phase 2: Default Pi Session
- [x] Spawn a default pi session at TUI startup (main repo dir, no prompt)
- [x] Always show "Main" tab as first tab
- [x] When Main tab is selected, show full-screen session panel

### Phase 3: Session Embedding in Work Tabs
- [x] When a work tab is selected, show split: work details (top) + session (bottom)
- [x] Sub-sessions within a work: agent, console, plan (ctrl+1/2/3 to switch)
- [x] z key to maximize/minimize session panel (toggle between split and full)

### Phase 4: Plan Sessions as Tabs
- [x] When p is pressed on a bean, create a new plan tab in the work tabs bar
- [x] Remove the ViewSessionViewer overlay in favor of tab-based viewing

### Phase 5: Wiring & Polish
- [x] Wire automatic session spawning for work sessions
- [x] Update key bindings and mouse handling
- [x] Update help text

## Summary of Changes

### New Files
- `internal/tui/tui_work_tab.go` — WorkTab model with Default/Work/Plan types, sub-session tracking, and session maximize state

### Modified Files
- `internal/tui/tui_panel_work_tabs.go` — Dual rendering: new tab-based rendering with distinct colors per type (teal for Main, purple for Plan, gray/orange for Work) + legacy fallback
- `internal/tui/tui_plan.go` — Tab lifecycle management (syncTabs, activateTab), default session spawning, session input forwarding, z/ctrl+1-3/0 key bindings, Tab cycling through session panel
- `internal/tui/tui_plan_render.go` — New renderSessionFullscreen() and renderWorkTabContent() for tab-based layouts
- `internal/tui/tui_plan_work.go` — spawnDefaultSession() for the always-present Main pi session
