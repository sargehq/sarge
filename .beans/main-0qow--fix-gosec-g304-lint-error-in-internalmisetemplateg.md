---
# main-0qow
title: Fix gosec G304 lint error in internal/mise/template.go
status: todo
type: bug
priority: high
created_at: 2026-02-24T02:15:05Z
updated_at: 2026-02-24T02:15:05Z
parent: 8lbx
---

Lint error at internal/mise/template.go:83:22 - G304: Potential file inclusion via variable (gosec). The offending line is: 'if existing, err := os.ReadFile(configPath); err == nil {'. gosec flags this as a potential file inclusion via variable. Fix by adding a #nosec G304 comment if the path is trusted, or by validating/sanitizing configPath before use.
