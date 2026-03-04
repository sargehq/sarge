---
# oise
title: Remove unused code in work package (orchestrator.go, tabs.go, tests)
status: completed
type: task
priority: normal
created_at: 2026-03-01T22:59:25Z
updated_at: 2026-03-02T21:38:29Z
parent: ir72
---

10 unused code lint errors: orchestrator.go - unused const controlPlaneTabName (line 20), unused funcs tabBelongsToWork (24), parseSessionType (43), sessionDisplayName (56), tabExists (132). tabs.go - unused funcs shellQuoteEnv (63), buildShellCommand (84), buildAgentCommand (146). destroy_integration_test.go:275 - unused boolPtr. terminate_tabs_test.go:44 - unused strPtr.

## Summary of Changes\n\nRemoved 10 unused code items:\n- orchestrator.go: const controlPlaneTabName, funcs tabBelongsToWork, parseSessionType, sessionDisplayName, tabExists\n- tabs.go: funcs shellQuoteEnv, buildShellCommand, buildAgentCommand (+ unused imports os, path/filepath, strings)\n- destroy_integration_test.go: func boolPtr\n- terminate_tabs_test.go: func strPtr
