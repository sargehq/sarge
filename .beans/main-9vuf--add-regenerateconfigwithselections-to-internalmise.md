---
# main-9vuf
title: Add RegenerateConfigWithSelections to internal/mise/template.go
status: completed
type: task
priority: normal
created_at: 2026-02-24T01:12:55Z
updated_at: 2026-02-24T01:35:30Z
parent: 8lbx
---

Add a new exported function RegenerateConfigWithSelections(dir string, selections ToolSelections) error to internal/mise/template.go that force-overwrites .mise.toml regardless of whether one already exists.

The existing GenerateConfigWithSelections already has the render+write logic but returns nil early if a config file is found (findConfigFile guard). The new function should skip that guard and always render the template to .mise.toml.

Implementation:
- In internal/mise/template.go, add RegenerateConfigWithSelections(dir string, selections ToolSelections) error
- Render miseTemplate using selections.toTemplateData() (identical to GenerateConfigWithSelections)
- Write result to filepath.Join(dir, ".mise.toml"), mode 0600, always overwriting
- No existence check — this is the force-regenerate path

Implemented: RegenerateConfigWithSelections added to internal/mise/template.go — force-overwrites .mise.toml without existence check, identical render logic to GenerateConfigWithSelections but skips findConfigFile guard.
