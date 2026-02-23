# Beads CLI Reference

## Finding Work

```bash
bd ready                          # Issues ready to work (no blockers)
bd list --status=open             # All open issues
bd list --status=in_progress      # Active work
bd blocked                        # All blocked issues
bd search "query"                 # Search issues by text
bd show <id>                      # Detailed view with dependencies
bd children <id>                  # List child issues
bd stats                          # Project statistics
```

## Creating Issues

```bash
bd create "Title" --type task -p 2 -d "Description"
bd create "Title" --type bug -p 1 --parent <epic-id>
bd create "Title" --type feature -p 2 --deps "blocks:<id>"
```

**Flags:**
- `--type` / `-t`: `task` | `bug` | `feature` | `chore` | `epic` (default: `task`)
- `--priority` / `-p`: `0`–`4` or `P0`–`P4` (0=critical, 2=medium, 4=backlog; default: `2`)
- `--description` / `-d`: Issue description
- `--parent`: Parent issue ID for hierarchical child
- `--assignee` / `-a`: Assign to someone
- `--deps`: Dependencies in format `type:id` or just `id`
- `--notes`: Working notes
- `--design`: Design notes

## Updating Issues

```bash
bd update <id> --status in_progress    # Claim work
bd update <id> --status open           # Unclaim
bd update <id> --title "New title"     # Change title
bd update <id> --description "..."     # Change description
bd update <id> --notes "..."           # Set working notes
bd update <id> --append-notes "..."    # Append to notes
bd update <id> --priority 1            # Change priority
bd update <id> --assignee "name"       # Assign
```

**⚠️ Do NOT use `bd edit`** — it opens $EDITOR and blocks agents.

## Closing Issues

```bash
bd close <id>                          # Mark complete
bd close <id> --reason "summary"       # Close with explanation
bd close <id1> <id2> <id3>             # Close multiple at once
bd reopen <id>                         # Reopen a closed issue
```

## Dependencies

```bash
bd dep add <issue> <depends-on>        # issue depends on depends-on
bd dep remove <issue> <depends-on>     # Remove dependency
bd dep tree <id>                       # Show dependency tree
```

**Direction**: `bd dep add A B` → A depends on B (A is blocked until B is done).

## Labels & Comments

```bash
bd label add <id> "label-name"         # Add label
bd label remove <id> "label-name"      # Remove label
bd comment <id> "Comment text"         # Add comment
```

## Sync & Health

```bash
bd sync                                # Sync beads with git
bd sync --status                       # Check sync status
bd doctor                              # Diagnose issues
bd prime                               # Output AI workflow context
```

## Issue Types

| Type | Use For |
|------|---------|
| `task` | General work items |
| `bug` | Defects to fix |
| `feature` | New functionality |
| `chore` | Maintenance, cleanup |
| `epic` | Large work with children |

## Priority Scale

| Priority | Meaning |
|----------|---------|
| `0` / `P0` | Critical — drop everything |
| `1` / `P1` | High — do soon |
| `2` / `P2` | Medium — default |
| `3` / `P3` | Low — when time allows |
| `4` / `P4` | Backlog — someday |
