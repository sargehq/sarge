---
# main-wzfv
title: 'Phase 1: Pi Bridge — Unix socket RPC bridge for pi sessions'
status: todo
type: feature
created_at: 2026-03-01T21:26:46Z
updated_at: 2026-03-01T21:26:46Z
parent: ir72
---

Build the core pi bridge component that manages pi RPC sessions over Unix domain sockets.

## Overview
Create a new `internal/bridge` package that:
1. Spawns `pi --mode rpc --no-session` as a child process
2. Communicates with it over stdin/stdout using the JSON-RPC protocol (see pi docs: rpc.md)
3. Exposes a Go API for sarge to send prompts, receive streaming events, abort, etc.
4. Manages multiple concurrent pi sessions (one per work task, plan session, or interactive agent)

## Key Design Decisions
- Each pi session = one spawned `pi --mode rpc` process
- The bridge manages the lifecycle: spawn, prompt, monitor events, abort, kill
- Sessions are identified by a session ID (e.g. `orch-<workID>`, `plan-<beanID>`, `agent-<workID>`)
- The bridge must handle: process crashes, restart, graceful shutdown (SIGTERM)

## Files to Create
- `internal/bridge/bridge.go` — Bridge manager (session registry, spawn/kill)
- `internal/bridge/session.go` — Single pi RPC session (stdin/stdout JSON protocol)
- `internal/bridge/events.go` — Event types mapped from pi RPC events
- `internal/bridge/bridge_test.go` — Tests

## Pi RPC Protocol Reference
Pi's RPC mode uses JSON lines over stdin/stdout:
- **Commands** (stdin): `{"type": "prompt", "message": "..."}`, `{"type": "abort"}`, etc.
- **Events** (stdout): `{"type": "message_update", ...}`, `{"type": "agent_end", ...}`, etc.
- **Responses**: `{"type": "response", "command": "prompt", "success": true}`

Key commands to support initially:
- `prompt` — send a task prompt
- `abort` — cancel current operation  
- `get_state` — check if streaming
- `steer` / `follow_up` — queue messages during streaming

Key events to handle:
- `message_update` (with `assistantMessageEvent.type` = `text_delta`)
- `tool_execution_start/update/end`
- `agent_start` / `agent_end`
- `extension_ui_request` (for extension dialogs)

## Go API Shape (approximate)
```go
type Bridge struct { ... }
func NewBridge() *Bridge
func (b *Bridge) SpawnSession(id string, workDir string, cfg *project.Config) (*Session, error)
func (b *Bridge) GetSession(id string) *Session
func (b *Bridge) KillSession(id string) error
func (b *Bridge) KillAll() error

type Session struct { ... }
func (s *Session) Prompt(message string) error
func (s *Session) Abort() error
func (s *Session) Events() <-chan Event
func (s *Session) IsStreaming() bool
func (s *Session) Wait() error  // wait for agent_end
```

## Dependencies
- Pi CLI must be installed (`pi --mode rpc`)
- No dependency on zmx or zellij

## Todo
- [ ] Define Session struct with stdin/stdout pipe management
- [ ] Implement JSON line reader/writer for pi RPC protocol
- [ ] Implement Bridge manager with session registry
- [ ] Handle pi process lifecycle (spawn, crash detection, kill)
- [ ] Handle pi config passthrough (provider, model, thinking from project.Config)
- [ ] Write tests with mock pi process
