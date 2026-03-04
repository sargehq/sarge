---
# main-to0t
title: 'Phase 5b: Remove Claude agent backend, make pi the only agent'
status: completed
type: task
priority: normal
created_at: 2026-03-01T21:31:45Z
updated_at: 2026-03-01T22:39:08Z
parent: ir72
blocked_by:
    - main-otx3
---

Remove the Claude agent backend and all related code, making pi the only supported agent.

## Overview
With the pi bridge as the sole execution path, the Claude agent abstraction is no longer needed. This simplifies the agent layer — no more agent type switching, no more Claude-specific config.

## What to Remove

### Packages (delete entirely)
- `internal/agents/claude/` — Claude agent implementation and templates

### Agent abstraction simplification
- `internal/agents/agents.go` — Remove the `Agent` interface and `agentType` switching. The pi agent can be used directly, or the interface can be kept but with only one implementation.
- `internal/agents/agents_mock.go` — Remove or regenerate
- `internal/agents/types/` — Keep (shared by pi agent)
- `internal/agents/runner/` — Keep (or already replaced by bridge in Phase 2)

### Config cleanup
- `AgentConfig` struct in `internal/project/config.go` — Remove `Type` field (always pi)
- `ClaudeConfig` struct in `internal/project/config.go` — Remove entirely (SkipPermissions, TimeLimitMinutes, TaskTimeoutMinutes)
- `[agent]` section in `internal/project/templates/config.tmpl` — Remove or simplify (no type selection)
- `[claude]` section in `internal/project/templates/config.tmpl` — Remove entirely
- Task timeout logic in `cmd/orchestrate.go` currently reads from `cfg.Claude.GetTaskTimeout()` — move to a general config or pi-specific config

### Code references
- `cmd/orchestrate.go` — Uses `agents.NewAgent(proj.Config)` and `proj.Config.Claude.GetTaskTimeout()`. Simplify to always use pi.
- `internal/work/tabs.go` — `buildAgentCommand()` switches on agent type ("pi" vs "claude"). Remove Claude branch.
- `internal/tui/tui_plan_work.go` — Agent session creation references agent type
- `internal/mise/template.go` — Agent type selection in project setup
- `cmd/proj.go` — Agent type configuration during project init

### Mise template
- `internal/mise/template/mise.tmpl` — Remove agent type selection (claude vs pi)
- `internal/mise/template.go` — Remove agent type from template data

## Task Timeout Migration
`ClaudeConfig.GetTaskTimeout()` provides task timeout (default 60min). This should be moved to a general config field, e.g.:
- `[workflow] task_timeout_minutes = 60` (already has `WorkflowConfig`)
- Or `[pi] task_timeout_minutes = 60`

## Todo
- [ ] Delete internal/agents/claude/ package
- [ ] Simplify or remove Agent interface in internal/agents/agents.go
- [ ] Remove AgentConfig.Type field from project config
- [ ] Remove ClaudeConfig struct from project config
- [ ] Remove [agent] and [claude] sections from config template
- [ ] Migrate task timeout to WorkflowConfig or PiConfig
- [ ] Remove agent type switching from buildAgentCommand() in tabs.go
- [ ] Remove agent type selection from mise template / project init
- [ ] Clean up all references to claude agent type
- [ ] Run full test suite
