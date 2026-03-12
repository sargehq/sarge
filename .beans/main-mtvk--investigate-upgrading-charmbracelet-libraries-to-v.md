---
# main-mtvk
title: Investigate upgrading Charmbracelet libraries to v2 to fix VT emulator flickering
status: completed
type: feature
priority: normal
created_at: 2026-03-12T01:07:04Z
updated_at: 2026-03-12T01:28:05Z
---

## Problem

The VT emulator output in the session panel continually scrolls and flickers. This is likely caused by Bubble Tea v1's string-based renderer struggling with full-screen ANSI output from the nested VT emulator — each PTY output triggers a full re-render cycle, and the string-based diff algorithm can't efficiently handle the dense ANSI escape sequences from `x/vt`'s `Render()`.

## Current Versions

- `bubbletea` v1.3.10
- `lipgloss` v1.1.1-pre (pinned commit)
- `bubbles` v0.21.1-pre (pinned commit)
- `huh` v0.8.0
- `x/vt` v0.0.0-20260302
- `x/ansi` v0.11.6

## Available v2 Versions

- `bubbletea/v2` v2.0.2
- `lipgloss/v2` v2.0.2
- `bubbles/v2` v2.0.0
- `huh` — needs v2-compatible version

## Why v2 Should Help

Bubble Tea v2 introduces a **cell-buffer based renderer** instead of string-based rendering. Key improvements:

1. **Differential cell updates** — only redraws changed cells, not full strings
2. **Native cell-buffer integration** — `x/vt` already uses `x/cellbuf` internally; v2 can consume cell buffers directly instead of round-tripping through ANSI strings
3. **Eliminates string-based diffing** — the v1 renderer's line-by-line string comparison is the root cause of flickering with dense ANSI content

## Scope of Upgrade

~24 Go files import Charm libraries. Key API changes:

- [x] Audit Bubble Tea v2 API changes (View signature, key handling, Cmd changes)
- [x] Audit Lipgloss v2 API changes (style API, rendering)
- [x] Audit Bubbles v2 API changes (spinner, viewport, etc.)
- [x] Check huh v2 compatibility
- [x] Check if x/vt Render() can return cell buffers directly for v2
- [x] Estimate effort and create implementation plan

## Summary of Changes

Successfully upgraded all Charmbracelet libraries from v1 to v2:

### Library Versions
- `bubbletea` v1.3.10 → `charm.land/bubbletea/v2` v2.0.2
- `lipgloss` v1.1.1-pre → `charm.land/lipgloss/v2` v2.0.2
- `bubbles` v0.21.1-pre → `charm.land/bubbles/v2` v2.0.0
- `huh` v0.8.0 → `charm.land/huh/v2` v2.0.3
- `bubblezone` v1.0.0 → `github.com/lrstanley/bubblezone/v2` v2.0.0
- `x/vt` updated to latest
- Go version bumped from 1.25.5 to 1.25.8

### Key Changes
1. **Import paths**: All `github.com/charmbracelet/*` → `charm.land/*/v2`
2. **View() signature**: `rootModel.View()` now returns `tea.View` with declarative `AltScreen` and `MouseMode`
3. **KeyPressMsg**: `tea.KeyMsg` → `tea.KeyPressMsg` with `Code`/`Mod`/`Text` fields instead of `Type`/`Runes`
4. **Mouse events**: Split `tea.MouseMsg` into typed `MouseClickMsg`/`MouseMotionMsg`/`MouseWheelMsg` with `.Mouse()` accessor
5. **keyMsgToBytes**: Rewritten for v2 Key API using `Code`/`Mod` instead of `Type` enum
6. **Viewport**: `New(w,h)` → `New(WithWidth(w), WithHeight(h))`, field access → setter methods
7. **Textinput**: `.Width = x` → `.SetWidth(x)`
8. **huh themes**: `ThemeCharm()` → `ThemeFunc(ThemeCharm)`
9. **Removed Kitty keyboard hack**: v2 handles Kitty protocol natively
10. **planModel no longer satisfies tea.Model**: Returns `*planModel` directly from Update()

## Recommendation

**Yes, upgrade** — the v2 cell-buffer renderer directly addresses the flickering issue. However, this is a non-trivial migration (~24 files) and should be planned as a focused effort.

## Investigation Findings

### Confirmed: v2 uses cell-buffer renderer
The v2 renderer (`cursedRenderer`) uses `ultraviolet.ScreenBuffer` (a cell buffer) internally. The render pipeline:
1. Parses the View's Content string into a `StyledString`
2. Draws it into a cell buffer (`ScreenBuffer`)
3. Uses `ultraviolet.TerminalRenderer` to diff against the previous cell buffer
4. Only emits ANSI sequences for changed cells
5. Has a built-in `viewEquals()` fast-path that skips rendering entirely when nothing changed
6. Caps at 60fps by default

This is a **fundamental improvement** over v1's line-by-line string diffing, which can't efficiently handle the dense ANSI output from the VT emulator.

### Import path change
v2 libraries have moved to `charm.land/` import paths:
- `charm.land/bubbletea/v2` (v2.0.2)
- `charm.land/lipgloss/v2` (v2.0.2)
- `charm.land/bubbles/v2` (v2.0.0)
- `charm.land/huh/v2` (v2.0.3)
- `x/vt` still at `github.com/charmbracelet/x/vt` (no charm.land migration yet)

### Key API changes
1. **View() returns `tea.View`** instead of `string` — use `tea.NewView(s)` to wrap strings
2. **KeyMsg is now an interface** — `KeyPressMsg` and `KeyReleaseMsg` are concrete types; pattern matching changes
3. **Key constants renamed** — e.g. `tea.KeyEnter` is now `uv.KeyEnter`, `KeyRunes` removed (use `Key.Text`)
4. **View struct** gains `AltScreen`, `Cursor`, `MouseMode`, `WindowTitle` fields (declarative)
5. **Update signature unchanged** — `Update(Msg) (Model, Cmd)`
6. **Init signature unchanged** — `Init() Cmd`

### Migration scope
- 24 Go files need import path changes
- All `View() string` methods → `View() tea.View`
- All `tea.KeyMsg` pattern matching needs updating for new key types
- `keyMsgToBytes()` in session panel needs rewrite for new Key API
- Lipgloss style API changes (v2 is largely compatible but import paths change)
- huh form usage needs updating to v2 API

### Estimate
Medium-large effort (~2-3 days). Recommend creating a feature branch and migrating file-by-file. The session panel + VT rendering code would benefit immediately.
