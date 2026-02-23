# Agent Instructions

This project uses **beans** for issue tracking. Run `beans prime` to get started.

## Quick Reference

```bash
beans list            # List all beans
beans show <id>       # View issue details
beans update <id> -s in-progress  # Claim work
beans update <id> -s completed    # Complete work
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create beans for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Complete finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   git add -A
   git commit -m "..."
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

## Technical Constraints

### Ghostty AppleScript (macOS)

Ghostty has no CLI/IPC for opening tabs or sending text to a specific window. The only mechanism is AppleScript via System Events.

**Key limitation:** `keystroke` in System Events ALWAYS sends to the frontmost app, regardless of the `tell process` block. There is no way to send keystrokes to a non-frontmost window or process. Do not waste time searching for one.

**Current approach** (`internal/zmx/zmx.go` — `buildGhosttyTabAppleScript`):
- `click menu item "New Tab"` — process-targeted, safe even if Ghostty isn't frontmost
- `set frontmost to true` — brings Ghostty to front before typing
- `keystroke "zmx attach ..."` — types into the now-frontmost Ghostty tab

There is a small race window between `set frontmost to true` and the `keystroke` if the user alt-tabs at exactly the wrong moment, but this is inherent to AppleScript and cannot be fixed without Ghostty adding IPC support.
