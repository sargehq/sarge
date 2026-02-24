---
# main-u4b0
title: UpdateConfigFields silently fails when switching from default claude to pi
status: completed
type: bug
priority: normal
created_at: 2026-02-24T01:38:55Z
updated_at: 2026-02-24T01:53:13Z
parent: 8lbx
---

review-w-437.4 | internal/project/config.go UpdateConfigFields + cmd/config.go runConfig

In cmd/config.go:runConfig, the flow calls UpdateConfig(configPath, proj.Config) then UpdateConfigFields(configPath, agentType, ...). However, UpdateConfig (internal/project/config.go:551) treats commented-out sections (e.g., '# [agent]') as 'already existing' and skips re-adding them. When a user's config has the default claude setup, the [agent] section is commented out ('# [agent]'). UpdateConfigFields then looks only for active (uncommented) '[agent]' section headers and cannot find it — printing a warning and silently dropping the agent type change. A user switching from the claude default to 'pi' via 'sarge config' will not have their selection persisted to config.toml.

Fix: UpdateConfigFields (or the runConfig flow) needs to handle the case where the target section is currently commented out — either by activating/uncommentng it, or by regenerating the config section from scratch using the new values.

Fixed: UpdateConfigFields now handles commented-out [agent] sections — uncomments the header and inserts/updates the type field when switching from default claude to another agent type.
