# Beans CLI Reference

## Finding Work

```bash
beans list                             # All beans
beans list --status todo               # Beans ready to start
beans list --status in-progress        # Active work
beans list --ready                     # Beans with no unresolved blockers
beans show <id>                        # Detailed view with dependencies
beans prime                            # Full AI workflow context
```

## Creating Issues

```bash
beans create "Title" --type task --priority normal
beans create "Title" --type bug --priority high --parent <epic-id>
beans create "Title" --type feature --priority normal --body "Description"
```

**Flags:**
- `--type` / `-t`: `task` | `bug` | `feature` | `epic` | `milestone` (default: `task`)
- `--priority`: `critical` | `high` | `normal` | `low` | `deferred` (default: `normal`)
- `--body`: Issue description / body text
- `--parent`: Parent issue ID for hierarchical child
- `--tag`: Add a tag (can be repeated)
- `--blocked-by`: ID of a bean that blocks this one

## Updating Issues

```bash
beans update <id> --status in-progress     # Claim work
beans update <id> --status todo            # Unclaim
beans update <id> --title "New title"      # Change title
beans update <id> --body "..."             # Change body
beans update <id> --body-append "..."      # Append to body
beans update <id> --priority high          # Change priority
beans update <id> --tag "label"            # Add a tag
beans update <id> --blocked-by <other-id>  # Add a blocker
```

## Completing Issues

```bash
beans update <id> --status completed       # Mark complete
beans update <id> --status scrapped        # Abandon
beans update <id> --status todo            # Reopen
```

## Dependencies

```bash
beans update <id> --blocked-by <blocker-id>   # id is blocked by blocker-id
```

**Direction**: `beans update A --blocked-by B` → A depends on B (A is blocked until B is done).

## Archive & Validation

```bash
beans archive                          # Archive completed/scrapped beans
beans check                            # Validate beans integrity
```

## Issue Types

| Type | Use For |
|------|---------|
| `task` | General work items |
| `bug` | Defects to fix |
| `feature` | New functionality |
| `epic` | Large work with children |
| `milestone` | High-level goals |

## Priority Scale

| Priority | Meaning |
|----------|---------|
| `critical` | Drop everything |
| `high` | Do soon |
| `normal` | Default |
| `low` | When time allows |
| `deferred` | Someday |

## Status Values

| Status | Meaning |
|--------|---------|
| `draft` | Not yet ready |
| `todo` | Ready to start (default) |
| `in-progress` | Actively being worked on |
| `completed` | Done |
| `scrapped` | Abandoned |
