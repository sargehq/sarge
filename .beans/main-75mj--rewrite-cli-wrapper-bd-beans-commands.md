---
# main-75mj
title: Rewrite CLI wrapper (bd → beans commands)
status: completed
type: task
priority: critical
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:14:58Z
parent: main-1icu
---

Replace all bd exec calls with beans exec calls:
- beans create --json (parse JSON output instead of text)
- beans update --json
- beans graphql --json for complex operations
- Delete/close/reopen via beans update --status
- AddComment → beans update --body-append
- AddLabels → beans update --tag
- AddDependency → beans update --blocking/--blocked-by
- SetExternalRef → store in bean body or tags
- Init → beans init
- InstallHooks → not needed (beans uses plain files)

## Files
- cli.go → rewrite all methods
- Remove bdCommand helper (no more BEADS_DIR env var)
