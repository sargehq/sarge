---
# main-clsn
title: RegenerateConfigWithSelections overwrites .mise.toml with no backup
status: completed
type: task
priority: normal
created_at: 2026-02-24T01:39:15Z
updated_at: 2026-02-24T02:09:10Z
parent: 8lbx
---

review-w-437.4 | internal/mise/template.go:RegenerateConfigWithSelections + cmd/config.go:runConfig

RegenerateConfigWithSelections force-overwrites .mise.toml with no backup. By contrast, UpdateConfig (internal/project/config.go:551) creates a .bak backup of config.toml before writing. Users who have manually customized their .mise.toml (e.g., added extra tools or tasks) will lose those customizations with no recovery path.

Fix: Create a .mise.toml.bak backup in RegenerateConfigWithSelections before overwriting, consistent with the UpdateConfig approach.

## Summary of Changes

Added backup creation to  in . Before overwriting , the function now reads any existing file and writes it to , consistent with the backup approach used by  in . Also added two tests in  to verify backup is created when a config exists and no backup is created when there is no existing config.
