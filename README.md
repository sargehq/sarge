# Sarge

Orchestrate Claude Code to process issues and create PRs. Includes a TUI for interactive management and CLI for scripting.

## Philosophy

Sarge is designed to manage an army of Claude agents, turning your issue tracker into a PR factory.

### The Workflow

1. **Create or import issues** - Define work in your issue tracker (beads), or import from Linear
2. **Plan the implementation** - Use Claude Code interactively to break down complex issues into actionable tasks
3. **Execute with a Work** - Create a work unit that represents a git worktree and feature branch
4. **Automatic execution** - Sarge orchestrates Claude to solve all issues, commit changes, and push continuously
5. **Code review** - Claude automatically reviews its own work, creating fix issues for any problems found
6. **PR creation** - Once implementation and review pass, Claude creates a comprehensive PR
7. **Handle feedback** - CI failures and review comments automatically become new issues, which can be planned or added to the existing work
8. **Merge and cleanup** - After approval, merge the PR and destroy the work

### Design Principles

- **Autonomous execution** - Agents work independently, committing and pushing after each completed issue
- **Continuous progress** - Work is never lost; every bead completion is immediately saved
- **Feedback loops** - CI failures and review comments flow back as actionable issues
- **Human oversight** - You control when to create work, when to merge, and can intervene at any point
- **Isolation** - Each work unit has its own worktree, preventing conflicts between parallel efforts

### Agent Support

Sarge currently supports Claude Code as its agent backend. The architecture is designed to be agent-agnostic, and other agentic coding tools could be supported in the future.

## Prerequisites

### Tools (installed via mise)

The following CLI tools are required but are **automatically installed by mise** when you run `mise install`:

| Tool | Purpose |
|------|---------|
| `bd` | Beads issue tracking |
| `claude` | Claude Code CLI |
| `gh` | GitHub CLI |
| `zellij` | Terminal multiplexer |

You only need `git` (usually pre-installed) and [mise](https://mise.jdx.dev/) itself:

```bash
curl https://mise.run | sh
```

### Claude Beads Skill

After mise installs the tools, you must install the beads skill for Claude Code:

```bash
claude /plugin marketplace add steveyegge/beads
claude /plugin install beads
```

This enables Claude to interact with the beads issue tracker.

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

### 1. Create a Project

```bash
# From a GitHub repository
sarge proj create ~/myproject https://github.com/user/repo

# From a local repository
sarge proj create ~/myproject ~/path/to/repo

cd ~/myproject
```

This clones the repo (or symlinks a local one), installs tools via mise, and initializes the beads issue tracker.

### 2. Create Issues

Create issues in the beads tracker for sarge to work on:

```bash
cd main
bd create --title "Add user authentication" --type feature
bd create --title "Fix login page crash" --type bug --priority 1
bd ready   # See what's available
cd ..
```

Or import from Linear: `sarge linear import ENG-123`

### 3. Choose Your Interface

Sarge provides two ways to interact with your project:

#### Option A: TUI (Recommended)

The interactive terminal UI provides a lazygit-style interface:

```bash
sarge
```

Features:
- Three-panel drill-down: Beads → Works → Tasks
- Create/destroy works, run tasks, monitor progress
- Bead filtering, search, multi-select
- Press `?` for keyboard shortcuts

#### Option B: CLI

Use individual commands for scripting or when you prefer the command line:

```bash
# Create a work unit from a bead (creates worktree + feature branch)
sarge work create bead-1

# Navigate to the work directory
cd w-abc

# Execute tasks
sarge run

# Or skip the manual steps - full automation in one command
sarge work create bead-1 --auto
```

### 4. Monitor and Merge

```bash
# Check on progress
sarge poll

# Once work is idle (all tasks finished), review the PR
sarge work pr          # Creates PR via Claude
sarge run              # Executes the PR task

# After PR approval
sarge work complete    # Mark work as done
sarge work destroy w-abc
```

## Project Commands

These commands must be used via CLI (not available in TUI):

| Command | Description |
|---------|-------------|
| `sarge proj create <dir> <repo>` | Create a new project (local path or GitHub URL) |
| `sarge proj destroy [--force]` | Remove project and all worktrees |
| `sarge proj status` | Show project info, worktrees, and task status |

### Project Structure

```
<project-dir>/
├── .co/
│   ├── config.toml      # Project configuration
│   └── tracking.db      # SQLite coordination database
├── main/                # Symlink to local repo OR clone from GitHub
│   └── .beads/          # Beads issue tracker
├── w-8xa/               # Work unit directory
│   └── tree/            # Git worktree for feature branch
└── ...
```

## Concepts

### Why Beads?

Sarge uses [Beads](https://github.com/steveyegge/beads), a distributed git-backed issue tracker designed specifically for AI coding agents. Traditional markdown plans lack the sophistication needed for complex, multi-step workflows. Beads provides:

- **Dependency tracking** - Agents understand task relationships and what's ready to work on
- **Git-native persistence** - Tasks stored as JSONL in `.beads/`, versioned alongside code
- **Collision-free IDs** - Hash-based IDs eliminate merge conflicts in multi-branch scenarios
- **Semantic compaction** - Completed tasks are summarized to conserve AI context windows

**You rarely need to use beads directly.** Claude Code (with the beads skill) and the TUI handle all issue management. The `bd` CLI is available if you need it, but most users interact with beads through `sarge` or let Claude manage issues automatically.

### Three-Tier Hierarchy

- **Work**: A feature branch with its own worktree, groups related tasks (ID: `w-8xa`)
- **Tasks**: Units of Claude execution within a work (ID: `w-8xa.1`, `w-8xa.2`)
- **Beads**: Individual issues from the beads tracker (ID: `ac-pjw`)

### Automated Workflow

Use `--auto` for a fully automated workflow:

```bash
sarge work create bead-1 bead-2 --auto
```

This mode:
1. Creates work unit and tasks from beads
2. Executes all implementation tasks
3. Runs review/fix loop until code is clean
4. Creates PR automatically
5. Returns PR URL when complete

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

### "not in a project directory"

All commands must be run from within a project:
```bash
sarge proj create ~/myproject ~/path/to/repo
cd ~/myproject
```

### "bd: command not found" or "gh: command not found" or "zellij: command not found"

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

### No beads found

Create work items in your project's main repo:
```bash
cd ~/myproject/main
bd create --title "Your task" --type task
bd ready  # View available beads
```

## License

Apache 2.0 with Commons Clause. See [LICENSE](LICENSE) for details.
