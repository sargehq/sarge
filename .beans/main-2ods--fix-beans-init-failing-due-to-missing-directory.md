---
# main-2ods
title: Fix beans init failing due to missing directory
status: completed
type: bug
priority: high
created_at: 2026-02-28T01:34:12Z
updated_at: 2026-02-28T01:37:14Z
---

When running `sarge proj create` on a repo that doesn't have beans, the project-local beans init fails because the `.co/.beans` directory doesn't exist yet. The `beans.Init()` function sets `cmd.Dir = beansDir` but never creates the directory first, causing a 'chdir: no such file or directory' error.

## Summary of Changes

Added `os.MkdirAll(beansDir, 0o755)` call in `beans.Init()` (internal/beans/cli.go) to ensure the target directory exists before running the `beans init` CLI command. This fixes the 'chdir: no such file or directory' error when creating projects with project-local beans.

## Additional Issue

`beansCommand()` in `internal/beans/cli.go` invokes `beans` directly via `exec.CommandContext(ctx, "beans", ...)` rather than through `mise exec -- beans`. During `proj create`, beans is installed by mise, but the Go process won't find it unless it's already on the global PATH. The `Init()` function should use `mise exec` to invoke beans through the mise-managed environment.

## Todo
- [x] Create the beans directory before running beans init
- [x] Make `beans.Init()` use mise exec to invoke the mise-installed beans binary
