---
# main-5p02
title: No unit tests for UpdateConfigFields
status: completed
type: task
priority: normal
created_at: 2026-02-24T01:39:03Z
updated_at: 2026-02-24T01:53:17Z
parent: 8lbx
---

review-w-437.4 | internal/project/config.go:490-548 (UpdateConfigFields) and internal/project/config_test.go

UpdateConfigFields has non-trivial section-aware line-replacement logic but zero test coverage. The existing config_test.go has good coverage of UpdateConfig, but nothing for UpdateConfigFields. Tests should cover: (1) updating agent.type in an active section, (2) updating multiplexer.type in an active section, (3) both sections missing (warning, no failure), (4) commented-out sections (the critical bug scenario above), (5) preserving indentation and unrelated fields.

Added 6 unit tests for UpdateConfigFields in config_test.go covering: active section updates, both sections missing (no failure), commented-out section activation, none agent type (section commented out), and indentation/field preservation.
