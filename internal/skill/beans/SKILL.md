---
name: beans
description: >
  Markdown-file issue tracker (beans) for multi-session work with dependencies.
  Use when the project has a .beans/ directory or .beans.yml config, or when
  managing tasks, bugs, features, dependencies, and project work. Replaces
  TodoWrite and markdown-based tracking.
---

# Beans Issue Tracker

## When to Use

Use `beans` commands when:
- The project has a `.beans/` directory or `.beans.yml`
- Work spans multiple sessions or has dependencies
- You need to track tasks, bugs, features, or epics

**Never** use TodoWrite, TaskCreate, or markdown files for task tracking when beans is available.

## Quick Start

```bash
beans prime           # Full AI-optimized workflow context (run after compaction/new session)
beans list            # List all beans
beans show <id>       # View issue details
```

## Essential Commands

| Action | Command |
|--------|---------|
| List all | `beans list` |
| View issue | `beans show <id>` |
| Create issue | `beans create "Title" --type task --priority normal` |
| Claim work | `beans update <id> --status in-progress` |
| Complete work | `beans update <id> --status completed` |
| Add blocker | `beans update <id> --blocked-by <other-id>` |
| View children | `beans show <id>` (shows children in output) |
| Archive done | `beans archive` |
| Validate | `beans check` |

For full command reference, see [resources/CLI_REFERENCE.md](resources/CLI_REFERENCE.md).

## Session Protocol

1. **Start**: `beans list` → `beans show <id>` → `beans update <id> --status in-progress`
2. **Work**: Implement changes, update bean with `beans update <id>` flags
3. **Complete**: `beans update <id> --status completed`
4. **Commit**: Include both code changes AND bean file(s) in the commit, then `git push`

See [resources/WORKFLOWS.md](resources/WORKFLOWS.md) for detailed workflows.

## ⚠️ Critical Warnings

- **Beans are markdown files** in `.beans/` — they are tracked in git alongside code.
- **Always commit bean files** with your code changes.
- **Use `beans prime`** at session start for full context.
- **Priority is a string**: `critical`, `high`, `normal`, `low`, `deferred`. NOT numeric.
- **Status values**: `draft`, `todo`, `in-progress`, `completed`, `scrapped`.
- **Title is positional**: `beans create "My title"`, not just flags.
- **Dependency direction**: `beans update A --blocked-by B` means **A is blocked by B**.

## Resources

- [CLI Reference](resources/CLI_REFERENCE.md) — Full command syntax
- [Workflows](resources/WORKFLOWS.md) — Session workflows and checklists
- [Dependencies](resources/DEPENDENCIES.md) — Dependency types and direction guide
- [Pitfalls](resources/PITFALLS.md) — Common agent mistakes and fixes
