# Beans Dependencies

## Dependency Types

| Type | Meaning | Affects `beans list --ready`? |
|------|---------|-------------------------------|
| `blocked-by` | Bean A is blocked by bean B (A can't start until B is done) | **Yes** |
| `parent-child` | Hierarchical grouping (via `--parent`) | No |

**Only `blocked-by` dependencies affect `beans list --ready`.** Parent-child is informational grouping.

## Direction

The most important thing to remember:

```bash
beans update A --blocked-by B
```

This means: **A is blocked by B** (B must be done before A can start).

### Mental Model

Think of it as: "this bean (`A`) is blocked by that bean (`B`)"

```bash
# "Deploy" is blocked by "Write tests" — deploy can't start until tests are done
beans update deploy-123 --blocked-by write-tests-456

# "Step 2" is blocked by "Step 1"
beans update step2-id --blocked-by step1-id
```

### Verify

After adding, check with:
```bash
beans show <id>
```

Look for:
- **Blocked by** — beans this one is waiting for
- **Blocking** — beans waiting for this one

## Decision Tree: Which Relationship?

1. **Must B finish before A can start?** → `beans update A --blocked-by B`
2. **B is a subtask of A?** → `beans create "B" --parent A`
3. **A and B are related but independent?** → Add a shared tag with `--tag`
