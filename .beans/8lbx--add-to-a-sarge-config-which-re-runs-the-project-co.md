---
# 8lbx
title: Add to a sarge config which re-runs the project configuration.
status: completed
type: feature
priority: normal
created_at: 2026-02-24T01:09:09Z
updated_at: 2026-02-24T03:49:41Z
---

I want it re-run the config, and then generate a new mise configuration, and alters the existing config.yaml against t?hat config.

Implemented: cmd/config.go added with 'sarge config' command that re-runs tool selection, regenerates .mise.toml via RegenerateConfigWithSelections, runs mise install, merges new config.toml sections via UpdateConfig, and updates agent.type/multiplexer.type via UpdateConfigFields. Command registered in cmd/root.go.
