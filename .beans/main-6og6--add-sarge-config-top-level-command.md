---
# main-6og6
title: Add sarge config top-level command
status: completed
type: task
priority: normal
created_at: 2026-02-24T01:13:14Z
updated_at: 2026-02-24T01:34:00Z
parent: 8lbx
---

Add a new top-level 'sarge config' cobra command that re-runs project configuration interactively, regenerates .mise.toml, runs mise install, and updates config.toml.

New file: cmd/config.go

Command structure:
  var configCmd = &cobra.Command{
      Use:   "config",
      Short: "Reconfigure the current project",
      RunE:  runConfig,
  }

runConfig implementation:
1. Find project: project.Find(ctx, "") — errors if not in a project directory
2. Re-run tool selection: call promptToolSelections() (already in cmd/proj.go — make it accessible or move to a shared location within cmd package since both files are in package cmd)
3. Regenerate mise config: mise.RegenerateConfigWithSelections(proj.Root, selections) — from subtask main-9vuf
4. Run mise install: mise.Initialize(proj.Root) — runs trust + install + setup task if present
5. Merge new config sections: project.UpdateConfig(configPath, proj.Config) — configPath is filepath.Join(proj.Root, project.ConfigDir, project.ConfigFile)
6. Update agent/multiplexer fields: project.UpdateConfigFields(configPath, agentType, selections.MultiplexerType) — from subtask main-62ul
7. Print confirmation summary

Register in cmd/root.go init():
  rootCmd.AddCommand(configCmd)

Notes:
- promptToolSelections() is already defined in cmd/proj.go in the same 'cmd' package, so it's directly callable from cmd/config.go
- agentType comes back as the first return value of promptToolSelections()
- proj.Config is the loaded config (used as template data source for UpdateConfig)

Implemented: Added cmd/config.go with 'sarge config' top-level command. Re-runs promptToolSelections(), regenerates .mise.toml via mise.RegenerateConfigWithSelections, runs mise.Initialize, merges sections via project.UpdateConfig, and updates fields via project.UpdateConfigFields. Also added RegenerateConfigWithSelections to internal/mise/template.go (dependency from failed task w-437.2). Registered configCmd in cmd/root.go.
