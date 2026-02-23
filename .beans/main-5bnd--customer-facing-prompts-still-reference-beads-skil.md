---
# main-5bnd
title: Customer-facing prompts still reference 'beads' skill instead of 'beans'
status: completed
type: bug
priority: normal
created_at: 2026-02-23T19:09:01Z
updated_at: 2026-02-23T19:40:35Z
---

Two customer-facing docs still tell users to install/use the old 'beads' skill name, but the skill has been renamed to 'beans' and `.pi/skills/beads/` no longer exists (only `.pi/skills/beans/` does).

## Affected Files

**README.md** (lines 51–62) — "Beads Skill" setup section:
- Section header says "Beads Skill"
- Tells Claude Code users to run `/plugin marketplace add steveyegge/beads` and `/plugin install beads`
- Tells pi users the skill is at `.pi/skills/beads/` — this path does not exist

**docs/cli-reference.md** (line 454) — `sarge doctor` output description:
- "**Beads skill**: Ensures the coding agent has the beads skill installed (pi skill or Claude plugin)"

## Not Affected

All other 'beads' references in README.md, CLAUDE.md, and source code correctly refer to the `bd` CLI / beads issue tracker that sarge integrates with — those are accurate product names and should not change.
