# Beads Dependencies

## Dependency Types

| Type | Meaning | Affects `bd ready`? |
|------|---------|---------------------|
| `blocks` | Issue A blocks issue B (B can't start until A is done) | **Yes** |
| `related` | Issues are related but independent | No |
| `parent-child` | Hierarchical grouping (via `--parent`) | No |
| `discovered-from` | Issue was found while working on another | No |

**Only `blocks` dependencies affect `bd ready`.** Other types are informational.

## Direction Trap

The most common mistake with dependencies:

```bash
bd dep add A B
```

This means: **A depends on B** (A is blocked by B, B must be done first).

### Mental Model

Think of it as: `bd dep add <the-blocked-one> <the-blocker>`

```bash
# "Deploy" depends on "Write tests" — deploy is blocked until tests are done
bd dep add deploy-123 write-tests-456

# "Step 2" depends on "Step 1"
bd dep add step2-id step1-id
```

### Verify

After adding, check with:
```bash
bd show <id>
```

Look for:
- **DEPENDS ON** — issues this one is waiting for
- **BLOCKS** — issues waiting for this one

## Decision Tree: Which Dependency Type?

1. **Must B finish before A can start?** → `bd dep add A B` (blocks)
2. **A and B are related but independent?** → `bd dep add A B --type related`
3. **B is a subtask of A?** → `bd create "B" --parent A` (parent-child)
4. **Found B while working on A?** → `bd dep add B A --type discovered-from`
