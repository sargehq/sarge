---
# main-pute
title: 'Phase 5: Remove all zmx and zellij code'
status: todo
type: task
created_at: 2026-03-01T21:28:17Z
updated_at: 2026-03-01T21:28:17Z
parent: ir72
blocked_by:
    - main-ab2m
---

Remove all zmx and zellij code, config, and references from the codebase.

## Overview
After Phases 1-4 establish the bridge-based architecture, this phase removes all traces of zmx and zellij. This is a cleanup phase that should only happen once all functionality has been migrated.

## What to Remove

### Packages (delete entirely)
- `internal/zmx/` — zmx.go, zmx_mock.go, zmx_test.go
- `internal/zellij/` — zellij.go, zellij_mock.go, zellij_test.go, tab.kdl.tmpl

### TUI files
- `internal/tui/tui_panel_zmx_picker.go` — ZmxPickerPanel (replaced by session picker)

### Config
- `MultiplexerConfig` struct in `internal/project/config.go` — Type, Terminal, AttachMode, AttachOrchestrator fields
- `ZellijConfig` struct in `internal/project/config.go` — KillTabsOnDestroy field
- `[multiplexer]` and `[zellij]` sections in `internal/project/templates/config.tmpl`
- References in `internal/mise/template.go` and `internal/mise/template/mise.tmpl`

### Code references (564 lines across 34+ non-test files)
Files with zmx/zellij references to clean up:
- `cmd/control_plane.go` — remove zmx control plane logic
- `cmd/orchestrate.go` — remove zellij tab references in comments
- `cmd/plan.go` — remove zmx/zellij session logic
- `cmd/proj.go` — remove multiplexer config
- `cmd/run.go` — remove zmx/zellij references
- `cmd/status.go` — remove zmx session status
- `cmd/work.go` — remove zmx/zellij tab operations
- `cmd/work_import_pr.go` — remove zmx references
- `internal/control/plane.go` — remove zmx/zellij control plane
- `internal/control/spawn.go` — remove EnsureControlPlane zmx/zellij branches
- `internal/db/bean.go` — remove zellij_session column references
- `internal/db/plan_session.go` — remove zellij session tracking
- `internal/db/work.go` — remove zmx session tracking
- `internal/db/sqlc/` — regenerate after schema changes
- `internal/work/orchestrator.go` — remove zmx/zellij from DefaultOrchestratorManager
- `internal/work/tabs.go` — remove all zmx/zellij tab implementations
- `internal/tui/tui_plan.go` — remove zmx/zellij fields and imports
- `internal/tui/tui_plan_work.go` — remove zmx session listing/attaching
- `internal/tui/tui_plan_render.go` — remove zmx references
- `internal/tui/tui_shared.go` — remove zmx references
- `internal/tui/tui_panel_work_details.go` — remove zmx references
- `internal/tui/tui_panel_work_tabs.go` — remove zellij style comments

### Database schema
- `sql/queries/beans.sql` — remove zellij_session references  
- `sql/queries/works.sql` — remove zmx session references
- `internal/db/schema.sql` and `internal/db/migrations/001_initial.sql` — check for multiplexer columns
- Create a new migration to drop any multiplexer-related columns

### Test files
- `internal/work/orchestrator_zmx_test.go` — delete
- `internal/work/terminate_tabs_test.go` — delete or adapt
- `internal/work/destroy_integration_test.go` — remove zmx/zellij references
- `internal/work/execution_integration_test.go` — remove zmx/zellij references
- All mock files for zmx/zellij

### Also remove Claude agent support (if desired)
- The issue mentions "the primary interface becomes a pi session" — confirm whether `internal/agents/claude/` should also be removed, or kept as an alternative agent backend.

## Todo
- [ ] Delete internal/zmx/ package
- [ ] Delete internal/zellij/ package  
- [ ] Delete tui_panel_zmx_picker.go
- [ ] Remove MultiplexerConfig and ZellijConfig from project config
- [ ] Remove multiplexer/zellij sections from config template
- [ ] Clean up all 34+ files with zmx/zellij references
- [ ] Create DB migration to drop multiplexer columns (if any)
- [ ] Regenerate sqlc after schema changes
- [ ] Delete zmx/zellij test files
- [ ] Update go.mod (remove any zmx/zellij-only dependencies)
- [ ] Run full test suite to verify no breakage
