---
# main-62ul
title: Add UpdateConfigFields to internal/project/config.go
status: completed
type: task
priority: normal
created_at: 2026-02-24T01:13:04Z
updated_at: 2026-02-24T01:29:57Z
parent: 8lbx
---

Add UpdateConfigFields(configPath string, agentType string, multiplexerType string) error to internal/project/config.go. This function updates the agent.type and multiplexer.type values in an existing config.toml in-place, preserving all user comments, other fields, and other sections.

Implementation approach (line-level section-aware replacement):
- Read the file as a string, split into lines
- Walk lines tracking current section (update currentSection whenever a '[section]' header is encountered)
- When currentSection == 'agent' and a line matches 'type = "..."', replace with 'type = "<agentType>"'
- When currentSection == 'multiplexer' and a line matches 'type = "..."', replace with 'type = "<multiplexerType>"'
- Rejoin and write back to configPath
- Only update lines that start with 'type =' (after TrimSpace) to avoid matching commented lines
- Return error if either section is not found (warn but don't fail — the section may be missing on older configs)

Implemented: Added UpdateConfigFields(configPath, agentType, multiplexerType) to internal/project/config.go. Uses line-level section-aware replacement preserving comments and indentation; warns but does not fail on missing sections.
