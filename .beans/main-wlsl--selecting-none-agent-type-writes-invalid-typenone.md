---
# main-wlsl
title: Selecting 'none' agent type writes invalid type="none" to config.toml
status: completed
type: bug
priority: normal
created_at: 2026-02-24T01:39:09Z
updated_at: 2026-02-24T01:53:13Z
parent: 8lbx
---

review-w-437.4 | cmd/config.go:runConfig + internal/project/config.go:UpdateConfigFields

If a user has an existing active [agent] section (e.g., type = "pi") and runs 'sarge config' selecting 'none' for agent, UpdateConfigFields will write type = "none" into the [agent] section. The value "none" may not be a valid agent type at runtime (the valid values appear to be "claude" and "pi"). The intended semantics of 'none' is 'no coding agent' — the [agent] section should be commented out or removed rather than set to type = "none".

Fix: Handle agentType == "none" specially in runConfig or UpdateConfigFields — comment out or remove the [agent] section instead of writing type = "none".

Fixed: UpdateConfigFields now handles agentType=="none" by commenting out the active [agent] section and any type= line beneath it, instead of writing type="none".
