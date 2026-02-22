# Common Agent Pitfalls with Beads

## ❌ Using `bd edit` (blocks agents)

**Problem**: `bd edit <id>` opens $EDITOR (vim/nano), which hangs the agent.

**Fix**: Use `bd update` with flags:
```bash
bd update <id> --description "new description"
bd update <id> --notes "working notes"
bd update <id> --title "new title"
bd update <id> --append-notes "additional context"
```

## ❌ String Priorities

**Problem**: Using `--priority high` or `--priority medium`.

**Fix**: Priority is numeric 0–4:
```bash
bd create "Title" -p 2        # Correct (medium)
bd create "Title" -p high     # WRONG — will error
```

## ❌ Dependency Direction Confusion

**Problem**: `bd dep add A B` is read as "A depends on B" but agents often reverse it.

**Fix**: Think "add dependency FROM A TO B" — A is waiting for B:
```bash
bd dep add <blocked-issue> <blocker-issue>
```

Verify with `bd show <id>` — check DEPENDS ON and BLOCKS sections.

## ❌ Forgetting `bd sync` Before Push

**Problem**: Beads state changes aren't included in git commits.

**Fix**: Always sync before committing:
```bash
bd sync
git add -A
git commit -m "..."
git push
```

## ❌ Using TodoWrite/Markdown Instead of Beads

**Problem**: Creating TODO.md or using TodoWrite when .beads/ exists.

**Fix**: Always use `bd create` for tracking work:
```bash
bd create "Fix the bug" --type bug -p 2 -d "Details..."
```

## ❌ Not Updating Notes for Session Survival

**Problem**: Context is lost after compaction or new session.

**Fix**: Update notes before ending a session:
```bash
bd update <id> --append-notes "Done: X, Y. Next: Z. Blocked by: W."
```

Notes survive compaction; your conversation history doesn't.

## ❌ Closing Without Reason

**Problem**: `bd close <id>` with no reason loses context about what was done.

**Fix**: Always include a reason:
```bash
bd close <id> --reason "Implemented feature X with tests, updated docs"
```
