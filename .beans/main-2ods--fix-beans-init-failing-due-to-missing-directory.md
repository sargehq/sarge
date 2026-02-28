---
# main-2ods
title: Fix beans init failing due to missing directory
status: in-progress
type: bug
priority: high
created_at: 2026-02-28T01:34:12Z
updated_at: 2026-02-28T01:34:12Z
---

When running `sarge proj create` on a repo that doesn't have beans, the project-local beans init fails because the `.co/.beans` directory doesn't exist yet. The `beans.Init()` function sets `cmd.Dir = beansDir` but never creates the directory first, causing a 'chdir: no such file or directory' error.
