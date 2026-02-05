# CLI Reference

This document provides detailed documentation for all `sarge` CLI commands.

## Work Commands

### `sarge work create <bead-args...>`

Creates a new work unit from one or more beads.

```bash
sarge work create bead-1           # Single bead
sarge work create bead-1 bead-2    # Multiple beads
sarge work create epic-1           # Epic (includes all children)
sarge work create bead-1 --auto    # Automated workflow
```

This creates:
- A work directory (`w-abc/`)
- A git worktree with a generated branch (`w-abc/tree/`)
- A unique work ID using content-based hashing

If the bead is an epic, all child beads are automatically included.
Transitive dependencies are also included.

The branch name is generated from bead titles and you're prompted for confirmation.

| Flag | Description |
|------|-------------|
| `--auto` | Full automated workflow (implement, review/fix loop, PR) |

Base branch is configured in `config.toml` under `[repo] base_branch` (default: main).

### `sarge work add <bead-args...>`

Adds beads to an existing work.

```bash
sarge work add bead-4 bead-5           # In work directory
sarge work add bead-4 --work w-abc     # Explicit work ID
```

- Detects work from current directory or uses `--work` flag
- Expands epics to include all child beads
- Cannot add beads already assigned to a task

### `sarge work remove <bead-ids...>`

Removes beads from an existing work.

```bash
sarge work remove bead-4 bead-5        # In work directory
sarge work remove bead-4 --work w-abc  # Explicit work ID
```

- Detects work from current directory or uses `--work` flag
- Cannot remove beads already assigned to a pending/processing task

### `sarge work list`

Lists all work units with their status.

```bash
sarge work list
```

Shows ID, status, branch, and PR URL. Displays summary counts by status.

### `sarge work show [<id>]`

Shows detailed information about a work.

```bash
sarge work show          # Current directory
sarge work show w-abc    # Explicit ID
```

Displays status, branch, worktree path, PR URL. Lists associated beads and tasks with their status.

### `sarge work destroy <id>`

Destroys a work unit and its resources.

```bash
sarge work destroy w-abc
```

- Removes git worktree
- Deletes work subdirectory
- Updates database records
- Use with caution - destructive operation

### `sarge work restart [<id>]`

Restarts a failed work.

```bash
sarge work restart         # Current directory
sarge work restart w-abc   # Explicit ID
```

- Only works if work is in `failed` status
- Transitions work back to `processing`
- Orchestrator will resume processing pending tasks

### `sarge work complete [<id>]`

Explicitly marks an idle work as completed.

```bash
sarge work complete        # Current directory
sarge work complete w-abc  # Explicit ID
```

- Only works if work is in `idle` status
- Transitions work to `completed` (terminal state)
- Use when PR is merged or work is truly finished

### `sarge work pr [<id>]`

Creates a PR task for Claude to generate a pull request.

```bash
sarge work pr          # Current directory
sarge work pr w-abc    # Explicit ID
```

Work must be completed before creating PR. After creating the PR task, run `sarge run` to execute it.

### `sarge work review [<id>]`

Creates a review task to examine code changes.

```bash
sarge work review              # Current directory
sarge work review w-abc        # Explicit ID
sarge work review --auto       # Review-fix loop
```

| Flag | Description |
|------|-------------|
| `--auto` | Loop review/fix until clean (max 3 iterations) |

Claude examines the work's branch for quality and security issues and creates beads for issues found.

### `sarge work feedback [<id>]`

Processes PR feedback and creates beads from actionable items.

```bash
sarge work feedback                    # Current directory
sarge work feedback w-abc              # Explicit ID
sarge work feedback --dry-run          # Preview only
sarge work feedback --auto-add         # Add beads to work
sarge work feedback --min-priority 2   # Filter by priority
```

| Flag | Description |
|------|-------------|
| `--dry-run` | Preview what beads would be created |
| `--auto-add` | Automatically add beads to work |
| `--min-priority N` | Set minimum priority (0-4) |

The feedback system processes:
- **CI/Build Failures**: Failed status checks and workflow runs
- **Test Failures**: Extracts specific test failures from logs
- **Lint Errors**: Code style and quality issues
- **Review Comments**: Actionable feedback from code reviews
- **Security Issues**: Vulnerabilities and security concerns

## Run Command

### `sarge run`

Executes pending tasks for a work unit.

```bash
sarge run                      # Current work directory
sarge run --work w-abc         # Explicit work ID
sarge run --plan               # LLM complexity grouping
sarge run --auto               # Full automated workflow
sarge run --dry-run            # Preview execution plan
```

| Flag | Short | Description |
|------|-------|-------------|
| `--limit` | `-n` | Maximum number of tasks to process (0 = unlimited) |
| `--dry-run` | | Show execution plan without running |
| `--plan` | | Use LLM complexity estimation to auto-group beads |
| `--auto` | | Full automated workflow (implement, review/fix loop, PR) |
| `--project` | | Specify project directory (default: auto-detect from cwd) |
| `--work` | | Specify work ID (default: auto-detect from current directory) |

## Task Commands

### `sarge task list`

Lists all tasks with their status.

```bash
sarge task list                    # All tasks
sarge task list --status pending   # Filter by status
sarge task list --type estimate    # Filter by type
```

| Flag | Description |
|------|-------------|
| `--status` | Filter: pending, processing, completed, failed |
| `--type` | Filter: estimate, implement |

### `sarge task show <id>`

Shows detailed information about a task.

```bash
sarge task show w-abc.1
```

Displays status, type, budget, timestamps. Lists associated beads and their completion status.

### `sarge task delete <id>...`

Deletes one or more tasks from the database.

```bash
sarge task delete w-abc.1
sarge task delete w-abc.1 w-abc.2  # Multiple tasks
```

### `sarge task reset <id>`

Resets a failed or stuck task to pending.

```bash
sarge task reset w-abc.1
```

Changes task status from processing/failed back to pending. Resets all bead statuses for the task.

### `sarge task set-review-epic <epic-id>`

Associates a review epic with a review task.

```bash
sarge task set-review-epic epic-1
sarge task set-review-epic epic-1 --task w-abc.2
```

Task is auto-detected from CO_TASK_ID env var or current processing review task.

## Monitoring Commands

### `sarge`

When run without a subcommand, sarge launches the interactive TUI for managing works and beads (lazygit-style).

```bash
sarge
```

Features:
- Three-panel drill-down: Beads → Works → Tasks
- Create/destroy works, run tasks
- Bead filtering (ready/open/closed), search, multi-select
- Keyboard shortcuts for all operations (press `?` for help)
- F5 to poll PR feedback on-demand

### `sarge poll [work-id|task-id]`

Monitor work/task progress with text output.

```bash
sarge poll             # All active works
sarge poll w-abc       # Specific work
sarge poll w-abc.1     # Specific task
sarge poll --interval 5s
```

| Flag | Description |
|------|-------------|
| `--interval` | Polling interval (default: 2s) |

## Other Commands

### `sarge status [bead-id]`

Shows bead tracking status.

```bash
sarge status           # All processing beads
sarge status bead-1    # Specific bead
```

### `sarge list`

Lists tracked beads in the database.

```bash
sarge list
sarge list --status pending
sarge list --status completed
```

| Flag | Description |
|------|-------------|
| `--status` | Filter: pending, processing, completed, failed |

### `sarge sync`

Pulls from upstream in all repositories.

```bash
sarge sync
```

Runs git pull in each worktree (main and all work worktrees).

## Linear Integration

### `sarge linear import <issues...>`

Import issues from Linear into the beads issue tracker.

```bash
# Import single issue
sarge linear import ENG-123
sarge linear import https://linear.app/company/issue/ENG-123/title

# Import multiple issues
sarge linear import ENG-123 ENG-124 ENG-125

# Import with dependencies
sarge linear import ENG-123 --create-deps --max-dep-depth=2

# Update existing bead
sarge linear import ENG-123 --update

# Preview
sarge linear import ENG-123 --dry-run
```

| Flag | Description |
|------|-------------|
| `--api-key` | Linear API key (or use `[linear] api_key` in config.toml) |
| `--create-deps` | Import blocking issues as dependencies |
| `--max-dep-depth` | Maximum depth for dependency import (default: 1) |
| `--update` | Update existing beads if already imported |
| `--dry-run` | Preview import without creating beads |
| `--status-filter` | Only import issues matching status |
| `--priority-filter` | Only import issues matching priority |
| `--assignee-filter` | Only import issues matching assignee |

Linear metadata (ID, URL, assignee, labels) is preserved in the imported bead.

## Agent Commands

These commands are called by Claude Code during task execution. Not intended for direct user invocation.

### `sarge complete <bead-id|task-id>`

Marks a bead or task as completed (or failed with --error).

```bash
sarge complete bead-1
sarge complete w-abc.1
sarge complete w-abc.1 --error "Build failed"
sarge complete w-abc.1 --pr "https://github.com/user/repo/pull/123"
```

| Flag | Description |
|------|-------------|
| `--error` | Mark as failed with error message |
| `--pr` | Associate a PR URL with completion |

### `sarge estimate <bead-id>`

Reports complexity estimate for a bead.

```bash
sarge estimate bead-1 --score 5 --tokens 15000
sarge estimate bead-1 --score 5 --tokens 15000 --task w-abc.1
```

| Flag | Description |
|------|-------------|
| `--score` | Complexity score (1-10) |
| `--tokens` | Estimated tokens (5000-50000) |
| `--task` | Task ID (optional) |

## Work Status States

Works have the following status states:

| Status | Meaning |
|--------|---------|
| `pending` | Work created, no tasks started yet |
| `processing` | At least one task is running |
| `idle` | All tasks done, waiting for more work (e.g., PR feedback) |
| `completed` | Truly finished - explicitly closed by user |
| `failed` | A task failed - requires user intervention |
| `merged` | PR was merged on GitHub (auto-detected) |

**Key behaviors:**
- When all tasks complete successfully → work transitions to `idle` (not `completed`)
- When a task fails → work transitions to `failed` and orchestrator halts
- When new tasks are added to an idle work → work resumes to `processing`
- When PR is merged on GitHub → work automatically transitions to `merged`
- User must explicitly run `sarge work complete` to mark work as truly done
- User must run `sarge work restart` to resume a failed work after fixing issues

## ID Generation

CO uses a hierarchical ID system:

- **Work IDs**: Content-based hash (e.g., `w-8xa`)
  - Generated from branch name + project + timestamp
  - 3-8 character base36 hash
  - Collision-resistant with automatic lengthening

- **Task IDs**: Hierarchical format (e.g., `w-8xa.1`, `w-8xa.2`)
  - Format: `<work-id>.<sequence>`
  - Sequential numbering within each work

- **Bead IDs**: Managed by beads system (e.g., `ac-pjw`)
  - Project-specific prefixes
  - Content-based hashing similar to works

## Task Dependencies

Task dependencies are derived automatically from bead dependencies:
- If bead A depends on bead B, and they're in different tasks, task(A) depends on task(B)
- `sarge run` executes tasks in the correct dependency order
- Cycles are detected and reported as errors

## Error Handling and Retries

When a task fails:
- The task is automatically marked as failed in the database
- Claude can signal failure using `sarge complete <task-id> --error "message"`
- To retry a failed task:
  ```bash
  sarge task reset <task-id>    # Reset task status to pending
  sarge run                     # Retry the task
  ```
- On retry, Claude only processes incomplete beads (already completed beads are skipped)
