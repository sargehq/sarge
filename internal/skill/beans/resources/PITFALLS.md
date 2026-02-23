# Common Agent Pitfalls with Beans

## ❌ Using Wrong Priority Format

**Problem**: Using numeric priorities like `--priority 2` or `--priority medium`.

**Fix**: Priority is a named string:
```bash
beans create "Title" --priority normal     # Correct
beans create "Title" --priority 2          # WRONG
beans create "Title" --priority medium     # WRONG
```

Valid values: `critical`, `high`, `normal`, `low`, `deferred`.

## ❌ Using Wrong Status Format

**Problem**: Using old status values like `open`, `in_progress`, `closed`.

**Fix**: Use beans status values (with hyphens, not underscores):
```bash
beans update <id> --status in-progress     # Correct
beans update <id> --status in_progress     # WRONG
beans update <id> --status completed       # Correct
beans update <id> --status closed          # WRONG
```

Valid values: `draft`, `todo`, `in-progress`, `completed`, `scrapped`.

## ❌ Dependency Direction Confusion

**Problem**: Getting the direction wrong when adding blockers.

**Fix**: `--blocked-by` means "this bean is blocked by that bean":
```bash
# A is blocked by B (B must finish first)
beans update A --blocked-by B
```

Verify with `beans show <id>` — check "Blocked by" and "Blocking" sections.

## ❌ Not Committing Bean Files

**Problem**: Bean changes aren't included in git commits.

**Fix**: Beans are markdown files in `.beans/` — always include them:
```bash
git add -A                    # Stages bean files too
git commit -m "..."
git push
```

No separate sync step needed — beans are plain files tracked by git.

## ❌ Using TodoWrite/Markdown Instead of Beans

**Problem**: Creating TODO.md or using TodoWrite when `.beans/` exists.

**Fix**: Always use `beans create` for tracking work:
```bash
beans create "Fix the bug" --type bug --priority high --body "Details..."
```

## ❌ Not Updating Body for Session Survival

**Problem**: Context is lost after compaction or new session.

**Fix**: Update the bean body before ending a session:
```bash
beans update <id> --body-append "Done: X, Y. Next: Z. Blocked by: W."
```

Bean bodies survive compaction; your conversation history doesn't.

## ❌ Completing Without Context

**Problem**: Marking a bean completed with no record of what was done.

**Fix**: Update the body before completing:
```bash
beans update <id> --body-append "Implemented feature X with tests, updated docs"
beans update <id> --status completed
```
