---
name: beads
description: >
  Git-backed issue tracker (bd) for multi-session work with dependencies.
  Use when the project has a .beads directory, or when managing tasks,
  bugs, features, dependencies, and project work. Replaces TodoWrite
  and markdown-based tracking.
---

# Beads Issue Tracker

## When to Use

Use `bd` commands when:
- The project has a `.beads/` directory
- Work spans multiple sessions or has dependencies
- You need to track tasks, bugs, features, or epics

**Never** use TodoWrite, TaskCreate, or markdown files for task tracking when beads is available.

## Quick Start

```bash
bd prime              # Full AI-optimized workflow context (run after compaction/new session)
bd ready              # Find work with no blockers
bd show <id>          # View issue details
```

## Essential Commands

| Action | Command |
|--------|---------|
| Find work | `bd ready` |
| View issue | `bd show <id>` |
| Create issue | `bd create "Title" --type task -p 2 -d "description"` |
| Claim work | `bd update <id> --status in_progress` |
| Complete work | `bd close <id>` or `bd close <id> --reason "summary"` |
| Add dependency | `bd dep add <dependent> <prerequisite>` |
| View children | `bd children <id>` |
| Sync with git | `bd sync` |

For full command reference, see [resources/CLI_REFERENCE.md](resources/CLI_REFERENCE.md).

## Session Protocol

1. **Start**: `bd ready` → `bd show <id>` → `bd update <id> --status in_progress`
2. **Work**: Implement changes, update notes with `bd update <id> --notes "..."`
3. **Complete**: `bd close <id> --reason "summary"`
4. **Sync**: `bd sync` → `git add -A` → `git commit` → `git push`

See [resources/WORKFLOWS.md](resources/WORKFLOWS.md) for detailed workflows.

## ⚠️ Critical Warnings

- **Do NOT use `bd edit`** — opens $EDITOR, blocks agents. Use `bd update` with flags instead.
- **Priority is numeric 0–4** (0=critical, 4=backlog). NOT "high"/"medium"/"low".
- **Title is positional**: `bd create "My title"`, not just `--title` (though `--title` also works).
- **Dependency direction**: `bd dep add A B` means **A depends on B** (A is blocked by B).
- **Always `bd sync` before `git push`** to include beads changes in the commit.
- **Close with reason**: `bd close <id> --reason "what was done"` preserves context across sessions.

## Resources

- [CLI Reference](resources/CLI_REFERENCE.md) — Full command syntax
- [Workflows](resources/WORKFLOWS.md) — Session workflows and checklists
- [Dependencies](resources/DEPENDENCIES.md) — Dependency types and direction guide
- [Pitfalls](resources/PITFALLS.md) — Common agent mistakes and fixes
