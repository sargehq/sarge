# Configuration Reference

Project configuration is stored in `.co/config.toml`.

## Full Example

```toml
[project]
  name = "my-project"
  created_at = 2024-01-15T10:00:00-05:00

[repo]
  type = "github"  # or "local"
  source = "https://github.com/user/repo"
  path = "main"
  base_branch = "main"

[hooks]
  env = [
    "CLAUDE_CODE_USE_VERTEX=1",
    "CLOUD_ML_REGION=us-east5",
    "MY_VAR=value"
  ]

[linear]
  api_key = "lin_api_..."

[claude]
  skip_permissions = true
  time_limit = 30
  task_timeout_minutes = 60

[workflow]
  max_review_iterations = 2

[scheduler]
  pr_feedback_interval_minutes = 5
  comment_resolution_interval_minutes = 5
  scheduler_poll_seconds = 1
  activity_update_seconds = 30

[multiplexer]
  type = "zmx"
  terminal = "ghostty -e zmx attach {session}"
  attach_orchestrator = false

[zellij]
  kill_tabs_on_destroy = true

[log_parser]
  use_agent = false
  model = "haiku"
```

## Section Reference

### `[project]`

Basic project metadata. Set automatically during `sarge proj create`.

| Key | Description |
|-----|-------------|
| `name` | Project name |
| `created_at` | Creation timestamp |

### `[repo]`

Repository configuration.

| Key | Description | Default |
|-----|-------------|---------|
| `type` | Repository type: `github` or `local` | - |
| `source` | GitHub URL or local path | - |
| `path` | Path to main worktree | `main` |
| `base_branch` | Default base branch for PRs | `main` |

### `[hooks]`

Environment configuration for Claude sessions.

| Key | Description |
|-----|-------------|
| `env` | Array of environment variables (supports `$VAR` expansion) |

Useful for:
- Configuring Claude Code to use Vertex AI
- Setting custom PATH for tools
- Any environment variables Claude needs

### `[linear]`

Linear integration settings.

| Key | Description |
|-----|-------------|
| `api_key` | Linear API key for `sarge linear import` |

### `[claude]`

Claude Code execution settings.

| Key | Description | Default |
|-----|-------------|---------|
| `skip_permissions` | Run with `--dangerously-skip-permissions` | `true` |
| `time_limit` | Maximum minutes per Claude session (0 = unlimited) | `0` |
| `task_timeout_minutes` | Maximum task execution time in minutes | `60` |

**Notes:**
- `skip_permissions`: Set to `false` to have Claude prompt for permission before running commands
- `time_limit`: Tasks exceeding this limit are terminated and marked as failed
- If `time_limit` is set and is less than `task_timeout_minutes`, `time_limit` takes precedence

### `[workflow]`

Automated workflow settings.

| Key | Description | Default |
|-----|-------------|---------|
| `max_review_iterations` | Maximum review/fix cycles in `--auto` mode | `2` |

### `[scheduler]`

Background task timing.

| Key | Description | Default |
|-----|-------------|---------|
| `pr_feedback_interval_minutes` | How often to check for PR feedback | `5` |
| `comment_resolution_interval_minutes` | How often to check for resolved feedback | `5` |
| `scheduler_poll_seconds` | Internal scheduler polling frequency | `1` |
| `activity_update_seconds` | Task activity timestamp update interval | `30` |

### `[log_parser]`

CI log analysis settings.

| Key | Description | Default |
|-----|-------------|---------|
| `use_agent` | Use the configured agent for log analysis instead of Go parser | `false` |
| `model` | Model to use for log analysis (passed through to agent) | (agent default) |

**When to use agent-based log parsing:**
- Polyglot projects with multiple languages
- Complex test frameworks (Jest, pytest, RSpec)
- Custom CI output formats
- When the native parser misses failures

**Cost/Performance:**
- Native parser: Zero cost, ~1ms per log
- Agent-based: Costs vary by provider and model. Typical ranges:
  - Small/fast models: ~$0.01 per log, ~2-5s
  - Mid-tier models: ~$0.03 per log, ~5-10s
  - Large/capable models: ~$0.15 per log, ~10-20s

### `[multiplexer]`

Terminal multiplexer configuration. Sarge supports two multiplexers: **zellij** (default) and **zmx**.

```toml
[multiplexer]
  type = "zmx"                                    # "zellij" (default) or "zmx"
  terminal = "ghostty -e zmx attach {session}"    # Terminal command template (zmx only)
  attach_orchestrator = false                      # Auto-attach orchestrator to terminal (zmx only)
```

| Key | Description | Default |
|-----|-------------|---------|
| `type` | Multiplexer backend: `zellij` or `zmx` | `zellij` |
| `terminal` | Terminal command template for zmx sessions. `{session}` is replaced with the session name. | `ghostty -e zmx attach {session}` |
| `attach_orchestrator` | Whether to open a terminal window for orchestrator sessions when spawned (zmx only) | `false` |

**zmx session naming convention:** Sessions are named `sarge-<project>.<tabname>` (e.g., `sarge-myproj.orch-w-abc`).

**Session types managed per work unit:**
- `orch-<workID>` — Work orchestrator
- `task-<workID>.*` — Task runners
- `console-<workID>` — Console shells
- `claude-<workID>` — Claude agent sessions
- `pi-<workID>` — Pi agent sessions

When a work unit is destroyed (`sarge work destroy`), all associated zmx sessions (or zellij tabs) are automatically killed. The orchestrator process receives SIGTERM first for clean shutdown, then all matching sessions are terminated. This behavior is controlled by the `[zellij] kill_tabs_on_destroy` setting.

### `[zellij]`

Zellij/multiplexer tab management configuration. These settings apply to both zellij and zmx multiplexers.

```toml
[zellij]
  kill_tabs_on_destroy = true   # Kill tabs/sessions when work is destroyed (default: true)
```

| Key | Description | Default |
|-----|-------------|---------|
| `kill_tabs_on_destroy` | Automatically kill all multiplexer tabs/sessions associated with a work unit when it is destroyed | `true` |

## Mise Setup Task

For JavaScript/Node.js projects, configure a mise `setup` task to install dependencies automatically.

Add to your project's `.mise.toml`:

```toml
# npm
[tasks]
setup = "npm install"

# pnpm
[tasks]
setup = "pnpm install"

# yarn
[tasks]
setup = "yarn install"
```

The setup task runs automatically during:
- Project creation (`sarge proj create`)
- Work creation (`sarge work create`)
