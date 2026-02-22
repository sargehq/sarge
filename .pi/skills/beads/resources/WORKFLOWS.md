# Beads Workflows

## Session Start

```bash
bd prime                               # Get full context (especially after compaction)
bd ready                               # Find available work
bd show <id>                           # Review issue details and dependencies
bd update <id> --status in_progress    # Claim the work
```

If resuming work, check for in-progress items first:
```bash
bd list --status=in_progress           # Find your active work
bd show <id>                           # Read notes for context
```

## Session End (Mandatory Checklist)

```
[ ] 1. git status                      # Check what changed
[ ] 2. bd close <id> --reason "..."    # Close completed issues
[ ] 3. git add -A                      # Stage changes
[ ] 4. bd sync                         # Sync beads state
[ ] 5. git commit -m "..."             # Commit everything
[ ] 6. git push                        # Push to remote
```

**Work is NOT done until `git push` succeeds.**

## Epic Planning

```bash
# Create the epic
bd create "Epic title" --type epic -p 2 -d "Overview"

# Create child tasks under the epic
bd create "Step 1" --type task --parent <epic-id> -d "..."
bd create "Step 2" --type task --parent <epic-id> -d "..."
bd create "Step 3" --type task --parent <epic-id> -d "..."

# Add ordering dependencies between steps
bd dep add <step-2-id> <step-1-id>     # Step 2 depends on Step 1
bd dep add <step-3-id> <step-2-id>     # Step 3 depends on Step 2

# View the plan
bd children <epic-id>
bd dep tree <epic-id>
```

## Side Quest Handling

When you discover work while doing other work:

```bash
# 1. Create an issue for the discovery
bd create "Found: thing that needs fixing" --type task -p 3 -d "Details..."

# 2. Decide: blocker or defer?
# If it blocks current work:
bd dep add <current-work-id> <new-issue-id>

# If it can wait, just leave it for bd ready to surface later
```

## Compaction Recovery

After context compaction or starting a new session:

```bash
bd prime                               # Reload full workflow context
bd list --status=in_progress           # Find active work
bd show <id>                           # Read notes for detailed context
```

**Tip**: Always update notes before session end so the next session has context:
```bash
bd update <id> --append-notes "Progress: completed X, next step is Y"
```
