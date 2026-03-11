---
# main-ow5d
title: Pi sessions embedded in work tabs
status: in-progress
type: feature
priority: high
created_at: 2026-03-11T18:51:06Z
updated_at: 2026-03-11T18:53:09Z
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
- [ ] Create WorkTab struct with types: Default, Work, Plan
- [ ] Each tab holds references to its pi session(s) and work data
- [ ] Update WorkTabsBar to render new tab types (Main, work tabs, plan tabs)

### Phase 2: Default Pi Session
- [ ] Spawn a default pi session at TUI startup (main repo dir, no prompt)
- [ ] Always show "Main" tab as first tab
- [ ] When Main tab is selected, show full-screen session panel

### Phase 3: Session Embedding in Work Tabs
- [ ] When a work tab is selected, show split: work details (top) + session (bottom)
- [ ] Sub-sessions within a work: agent, console, plan (ctrl+1/2/3 to switch)
- [ ] z key to maximize/minimize session panel (toggle between split and full)

### Phase 4: Plan Sessions as Tabs
- [ ] When p is pressed on a bean, create a new plan tab in the work tabs bar
- [ ] Remove the ViewSessionViewer overlay in favor of tab-based viewing

### Phase 5: Wiring & Polish
- [ ] Wire automatic session spawning for work sessions
- [ ] Update key bindings and mouse handling
- [ ] Update help text
