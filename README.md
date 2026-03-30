# Sarge

Orchestrate code agents to process issues and create PRs. Includes a TUI for interactive management and CLI for scripting.

## Philosophy

Sarge is designed to manage an army of coding agents, turning your issue tracker into a PR factory.

### The Workflow

1. **Create or import issues** - Define work in your issue tracker (beans), or import from Linear
2. **Plan the implementation** - Use agents interactively to break down complex issues into actionable tasks
3. **Execute with a Work** - Create a work unit that represents a git worktree and feature branch
4. **Automatic execution** - Sarge orchestrates agents to solve all issues, commit changes, and push continuously
5. **Code review** - The agent automatically reviews work, creating fix issues for any problems found
6. **PR creation** - Once implementation and review pass, the agent creates a comprehensive PR
7. **Handle feedback** - CI failures and review comments automatically become new issues, which can be planned or added to the existing work
8. **Merge and cleanup** - After approval, merge the PR and destroy the work

### Design Principles

- **Autonomous execution** - Agents work independently, committing and pushing after each completed issue
- **Continuous progress** - Work is never lost; every bead completion is immediately saved
- **Feedback loops** - CI failures and review comments flow back as actionable issues
- **Human oversight** - You control when to create work, when to merge, and can intervene at any point
- **Isolation** - Each work unit has its own worktree, preventing conflicts between parallel efforts

### Agent Support

Sarge currently supports Claude Code & pi as its agent backend. The architecture is designed to be agent-agnostic, and other agentic coding tools could be supported in the future.

## Prerequisites

### Tools (installed via mise)

The following CLI tools are required but are **automatically installed by mise** when you run `mise install`. During project creation, sarge generates a `.mise.toml` based on your chosen agent:

| Tool | Purpose |
|------|---------|
| `beans` | Beans issue tracking |
| `claude` or `pi` | Coding agent (selected during project setup) |
| `gh` | GitHub CLI |
| `zellij` | Terminal multiplexer |

You only need `git` (usually pre-installed) and [mise](https://mise.jdx.dev/) itself:

```bash
curl https://mise.run | sh
```

### Beans Skill

Install beans:
```
brew install hmans/beans/beans
beans init
```

Your coding agent needs a beans skill to interact with the issue tracker.

**Claude Code**: There's no beans skill yet. The temporary solution is to add this to your `.claude/settings.json`:
```
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "beans prime" }] }
    ],
    "PreCompact": [
      { "hooks": [{ "type": "command", "command": "beans prime" }] }
    ]
  }
}
```

**pi**: The beans skill is included in `.pi/skills/beans/` and is automatically available — no extra setup needed.

### Terminal Font (for zellij)

Zellij uses a nerd font for icons. Install one and configure your terminal to use it:

**macOS:**
```bash
brew install font-hack-nerd-font
```

Then update your terminal preferences to use "Hack Nerd Font" or "Hack Nerd Font Mono".

**Linux (Debian/Ubuntu):**
```bash
mkdir -p ~/.local/share/fonts
cd ~/.local/share/fonts
curl -fLO https://github.com/ryanoasis/nerd-fonts/releases/latest/download/Hack.zip
unzip Hack.zip -d Hack
rm Hack.zip
fc-cache -fv
```

Then configure your terminal emulator to use "Hack Nerd Font" or "Hack Nerd Font Mono".

**Linux (Arch):**
```bash
pacman -S ttf-hack-nerd
```

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/sargehq/sarge/main/scripts/install.sh | bash
```

This downloads the latest release for your platform. Alternatively:

```bash
# With Go 1.25+
go install github.com/sargehq/sarge@latest
```

For more information, visit [sargehq.dev](https://sargehq.dev).

## Quick Start

```bash
# Create a project from a GitHub repo (or local path)
sarge proj create ~/myproject https://github.com/user/repo

# Enter the project directory and launch the TUI
cd ~/myproject
sarge
```

The TUI provides a lazygit-style interface for managing your entire workflow:

- **Create issues** or import from Linear
- **Create works** from issues (worktree + feature branch)
- **Run tasks** and monitor agent progress
- **Review, create PRs, and merge** when ready
- Press `?` for keyboard shortcuts

### CLI Alternative

All TUI actions are also available as CLI commands for scripting:

```bash
cd main
beans create --title "Add user authentication" --type feature
sarge linear import ENG-123       # Or import from Linear
cd ..

sarge work create bead-1          # Create work from a bead
sarge work create bead-1 --auto   # Full automated workflow
sarge poll                        # Monitor progress
sarge work pr                     # Create PR
sarge work complete               # Mark done
sarge work destroy w-abc          # Clean up
```

## Project Commands

These commands must be used via CLI (not available in TUI):

| Command | Description |
|---------|-------------|
| `sarge proj create <dir> <repo>` | Create a new project (local path or GitHub URL) |
| `sarge proj destroy [--force]` | Remove project and all worktrees |
| `sarge proj status` | Show project info, worktrees, and task status |
| `sarge doctor [--dry-run]` | Check and fix project health (config, mise, agent skill) |

### Project Structure

```
<project-dir>/
├── .sarge/
│   ├── config.toml      # Project configuration
│   └── tracking.db      # SQLite coordination database
├── main/                # Symlink to local repo OR clone from GitHub
│   └── .beans/          # beans issue tracker
├── w-8xa/               # Work unit directory
│   └── tree/            # Git worktree for feature branch
└── ...
```

## Concepts

### Why beans?

Sarge uses [Beans](https://github.com/hmans/beans), a distributed git-backed issue tracker designed specifically for AI coding agents. Traditional markdown plans lack the sophistication needed for complex, multi-step workflows. Beans provides:

- **Dependency tracking** - Agents understand task relationships and what's ready to work on
- **Git-native persistence** - Tasks stored as JSONL in `.beans/`, versioned alongside code
- **Collision-free IDs** - Hash-based IDs eliminate merge conflicts in multi-branch scenarios
- **Semantic compaction** - Completed tasks are summarized to conserve AI context windows

**You rarely need to use beans directly.** The coding agent and the TUI handle all issue management. The `beans` CLI is available if you need it, but most users interact with beans through `sarge` or let the agent manage issues automatically.

### Three-Tier Hierarchy

- **Work**: A feature branch with its own worktree, groups related tasks (ID: `w-8xa`)
- **Tasks**: Units of agent execution within a work (ID: `w-8xa.1`, `w-8xa.2`)
- **beans**: Individual issues from the beans tracker (ID: `ac-pjw`)

### Automated Workflow

Use `--auto` for a fully automated workflow:

```bash
sarge work create bead-1 --auto
```

This mode:
1. Creates work unit and tasks from the bead
2. Executes all implementation tasks
3. Runs review/fix loop until code is clean
4. Creates PR automatically

Monitor progress by switching to the zellij session. To include additional beans, use `sarge work add`.

## Documentation

- [CLI Reference](docs/cli-reference.md) - Complete command documentation
- [Configuration](docs/configuration.md) - Project configuration options

## Development

### Setup

```bash
mise install && lefthook install
```

### Run Tests

```bash
go test ./...
```

### Build

```bash
go build -o sarge .
```

## Troubleshooting

### General Health Check

Run `sarge doctor` to automatically detect and fix common project issues:
```bash
sarge doctor             # Check and fix
sarge doctor --dry-run   # Preview changes
```

### "not in a project directory"

All commands must be run from within a project:
```bash
sarge proj create ~/myproject ~/path/to/repo
cd ~/myproject
```

### "beans: command not found" or "gh: command not found" or "zellij: command not found"

These tools are installed by mise:
```bash
mise install
```

If mise isn't installed, see [mise.jdx.dev](https://mise.jdx.dev/).

### "not logged into any GitHub hosts"

Authenticate with GitHub:
```bash
gh auth login
```

### No beans found

Create work items in your project's main repo:
```bash
cd ~/myproject/main
beans create --title "Your task" --type task
beans ready  # View available beans
```

## License

Apache 2.0 with Commons Clause. See [LICENSE](LICENSE) for details.
