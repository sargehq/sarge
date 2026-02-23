# Beans Workflows

## Session Start

```bash
beans prime                                # Get full context (especially after compaction)
beans list --status todo                   # Find available work
beans show <id>                            # Review issue details and dependencies
beans update <id> --status in-progress     # Claim the work
```

If resuming work, check for in-progress items first:
```bash
beans list --status in-progress            # Find your active work
beans show <id>                            # Read body for context
```

## Session End (Mandatory Checklist)

```
[ ] 1. git status                          # Check what changed
[ ] 2. beans update <id> --status completed  # Complete finished beans
[ ] 3. git add -A                          # Stage all changes (including bean files)
[ ] 4. git commit -m "..."                 # Commit everything
[ ] 5. git push                            # Push to remote
```

**Work is NOT done until `git push` succeeds.**

## Epic Planning

```bash
# Create the epic
beans create "Epic title" --type epic --priority normal --body "Overview"

# Create child tasks under the epic
beans create "Step 1" --type task --parent <epic-id> --body "..."
beans create "Step 2" --type task --parent <epic-id> --body "..."
beans create "Step 3" --type task --parent <epic-id> --body "..."

# Add ordering dependencies between steps
beans update <step-2-id> --blocked-by <step-1-id>    # Step 2 blocked by Step 1
beans update <step-3-id> --blocked-by <step-2-id>    # Step 3 blocked by Step 2

# View the plan
beans show <epic-id>
```

## Side Quest Handling

When you discover work while doing other work:

```bash
# 1. Create an issue for the discovery
beans create "Found: thing that needs fixing" --type task --priority low --body "Details..."

# 2. Decide: blocker or defer?
# If it blocks current work:
beans update <current-work-id> --blocked-by <new-issue-id>

# If it can wait, just leave it for later
```

## Compaction Recovery

After context compaction or starting a new session:

```bash
beans prime                                # Reload full workflow context
beans list --status in-progress            # Find active work
beans show <id>                            # Read body for detailed context
```

**Tip**: Always update the body before session end so the next session has context:
```bash
beans update <id> --body-append "Progress: completed X, next step is Y"
```
