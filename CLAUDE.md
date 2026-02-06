# Sarge

Go CLI tool that orchestrates Claude Code to process issues, creating PRs for each.

## Build & Test

```bash
go build -o sarge .
go test ./...
```

## Project Structure

- `main.go` - CLI entry point (cobra)

### User-Facing Commands
- `cmd/proj.go` - Project management (create/destroy/status)
- `cmd/work.go` - Work management (create/list/show/destroy/pr/review)
- `cmd/work_automated.go` - Automated bead-to-PR workflow
- `cmd/work_feedback.go` - PR feedback processing (fetch feedback, create beads)
- `cmd/task.go` - Task management (list/show/delete/reset/set-review-epic)
- `cmd/run.go` - Run command (execute pending tasks or works)
- `cmd/list.go` - List command (list tracked beads)
- `cmd/status.go` - Status command (show bead tracking status)
- `cmd/poll.go` - Poll command (monitor progress with text output)
- `cmd/sync.go` - Sync command (pull from upstream)
- `cmd/linear.go` - Linear integration commands (import issues from Linear)

### Agent Commands (called by Claude/orchestration)
- `cmd/complete.go` - [Agent] Mark beads/tasks as done or failed
- `cmd/estimate.go` - [Agent] Report complexity estimates for beads

### Internal/Hidden Commands (spawned automatically)
- `cmd/orchestrate.go` - [Hidden] Execute tasks for a work unit (zellij tab)
- `cmd/control_plane.go` - [Hidden] Background task execution (zellij tab)

### Internal Packages
- `internal/beads/` - Beads database client (bd CLI wrapper)
- `internal/linear/` - Linear MCP client and import logic
- `internal/agents/` - Coding agent invocation (Claude, pi)
  - `internal/agents/claude/` - Claude Code agent templates and args
  - `internal/agents/pi/` - Pi agent templates and args
- `internal/db/` - SQLite tracking database
- `internal/task/` - Task planning and complexity estimation
- `internal/git/` - Git operations
- `internal/github/` - PR creation and merging (gh CLI)
- `internal/mise/` - Mise tool management
- `internal/project/` - Project discovery and configuration
- `internal/worktree/` - Git worktree operations
- `internal/logging/` - Structured logging using slog
- `internal/procmon/` - Database-backed process monitoring with heartbeats
- `internal/testutil/` - Shared test utilities and moq-generated mocks

## External Dependencies

Uses CLI tools: `bd`, `claude`, `gh`, `git`, `mise` (optional), `zellij`

**Important**: The beads (`bd`) version in `mise.toml` must stay aligned with the version in `internal/mise/template/mise.tmpl`. Sarge queries the beads database directly via sqlc and expects specific schema columns. Version mismatches cause errors like "no such column: owner".

## Context Usage

Functions that execute external commands or perform I/O should accept `context.Context` as their first parameter:

- Use `exec.CommandContext(ctx, ...)` instead of `exec.Command(...)` for shell commands
- Pass context through the call chain from CLI commands down to helper functions
- This enables proper cancellation and timeout handling

## Error Handling Best Practices

When handling errors in Go, use the standard `errors` package properly:

- Use `errors.Is(err, targetErr)` to check if an error matches a specific sentinel error
- Use `errors.As(err, &targetType)` to check and extract specific error types
- Never use type assertions like `err.(*exec.ExitError)` - use `errors.As` instead

Example:
```go
// Good - using errors.As
var exitErr *exec.ExitError
if errors.As(err, &exitErr) {
    // Handle exec.ExitError
    if exitErr.ExitCode() == 1 {
        // ...
    }
}

// Bad - using type assertion
if exitErr, ok := err.(*exec.ExitError); ok {  // Don't do this
    // ...
}
```

## Debug Logging

The project uses Go's `slog` for structured debug logging via `internal/logging`.

- Logs are written to `.co/debug.log` in JSON format (append mode)
- Logging is automatically initialized when a project is loaded
- Log file location: `<project-root>/.co/debug.log`

### Usage

```go
import "github.com/sargehq/sarge/internal/logging"

// Simple logging
logging.Debug("operation started", "key", value)
logging.Info("status update", "count", 42)
logging.Warn("potential issue", "error", err)
logging.Error("operation failed", "error", err, "context", ctx)

// Get logger for custom use
logger := logging.Logger()
logger.With("component", "beads").Debug("message")
```

### Viewing Logs

```bash
# View recent logs
tail -f .co/debug.log

# Pretty-print JSON logs
cat .co/debug.log | jq .
```

## Mock Generation

The project uses [moq](https://github.com/matryer/moq) for generating test mocks. Mocks are stored in `internal/testutil/` and use the function-field pattern for easy customization per-test.

### Installing moq

moq is installed automatically via mise:

```bash
mise install  # Installs all tools including moq
```

The tool is defined in `mise.toml`:
```toml
"go:github.com/matryer/moq" = "latest"
```

### Regenerating Mocks

After modifying interfaces or adding new `//go:generate` directives:

```bash
mise run generate
```

This runs `go generate ./...` to regenerate all mocks.

### Adding a New Mock

1. Add a `//go:generate` directive to the interface file:
   ```go
   //go:generate moq -stub -out mypkg_mock.go . InterfaceName:InterfaceNameMock
   ```

2. Run `mise run generate` to create the mock

3. Use the mock in tests:
   ```go
   mock := &mypkg.InterfaceNameMock{
       MethodNameFunc: func(ctx context.Context, arg string) error {
           return nil
       },
   }
   ```

### Available Mocks

Mocks are generated in their respective package directories:
- `internal/git/git_mock.go` - Git CLI operations (`GitOperationsMock`)
- `internal/worktree/worktree_mock.go` - Git worktree operations (`WorktreeOperationsMock`)
- `internal/mise/mise_mock.go` - Mise tool operations (`MiseOperationsMock`)
- `internal/zellij/zellij_mock.go` - Zellij session management (`SessionManagerMock`, `SessionMock`)
- `internal/beads/beads_mock.go` - Beads CLI and reader interfaces (`BeadsCLIMock`, `BeadsReaderMock`)
- `internal/github/github_mock.go` - GitHub API client (`GitHubClientMock`)
- `internal/agents/agents_mock.go` - Agent runner and agent (`AgentRunnerMock`, `AgentMock`)
- `internal/process/process_mock.go` - Process lister/killer (`ProcessListerMock`, `ProcessKillerMock`)
- `internal/task/task_mock.go` - Complexity estimator (`ComplexityEstimatorMock`)
- `internal/linear/linear_mock.go` - Linear API client (`LinearClientMock`)
- `internal/feedback/feedback_mock.go` - PR feedback processor (`FeedbackProcessorMock`)
- `internal/control/control_mock_test.go` - Orchestrator spawner, work destroyer (test-local mocks to avoid import cycle)

### Testing Best Practices

**Configuring mock behavior per-test:**
```go
mock := &git.GitOperationsMock{
    BranchExistsFunc: func(ctx context.Context, repoPath, branchName string) bool {
        return branchName == "main"  // Returns true only for "main"
    },
}
```

**Tracking and verifying calls:**
```go
mock := &git.GitOperationsMock{
    FetchPRRefFunc: func(ctx context.Context, repoPath string, prNumber int, localBranch string) error {
        return nil
    },
}

_ = mock.FetchPRRef(ctx, "/repo", 123, "pr-123")

// Verify call count
calls := mock.FetchPRRefCalls()
if len(calls) != 1 {
    t.Errorf("expected 1 call, got %d", len(calls))
}

// Verify call arguments
if calls[0].PrNumber != 123 {
    t.Errorf("expected prNumber 123, got %d", calls[0].PrNumber)
}
```

**Nil functions return zero values:**
```go
mock := &git.GitOperationsMock{}  // No functions set

// Returns false (zero value for bool) when BranchExistsFunc is nil
mock.BranchExists(ctx, "/repo", "any")  // returns false

// Returns nil, nil when ListBranchesFunc is nil
branches, err := mock.ListBranches(ctx, "/repo")  // branches=nil, err=nil
```

**Compile-time interface verification:**
```go
// Ensure mock implements the interface at compile time
var _ git.Operations = (*git.GitOperationsMock)(nil)
```

## Database Migrations

The project uses a SQLite database (`tracking.db`) with schema migrations.

### Creating a New Migration

1. Create a new SQL file in `internal/db/migrations/` with sequential numbering:
   ```
   internal/db/migrations/005_your_migration_name.sql
   ```

2. Structure the migration with `+up` and `+down` sections:
   ```sql
   -- +up
   -- SQL commands to apply the migration
   CREATE TABLE example (...);

   -- +down
   -- SQL commands to reverse the migration
   DROP TABLE IF EXISTS example;
   ```

3. Migrations run automatically when the database is accessed
4. The `+down` section is critical for rollback capability

### Regenerating SQLC Code

After modifying SQL queries in `internal/db/queries/` or changing the schema:

```bash
mise run sqlc-generate
```

This regenerates the Go code in `internal/db/sqlc/` from the SQL query definitions.

## PR Feedback Integration

The system integrates with GitHub to automatically process PR feedback and create actionable beads.

### Architecture

#### Database Schema

The `pr_feedback` table stores feedback items from GitHub:

```sql
CREATE TABLE pr_feedback (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL,
    pr_url TEXT NOT NULL,
    source TEXT NOT NULL,      -- e.g., "CI: Test Suite", "Review: User123"
    source_id TEXT,             -- GitHub comment ID for resolution
    title TEXT NOT NULL,
    description TEXT,
    feedback_type TEXT NOT NULL, -- test, build, lint, review, security
    priority INTEGER NOT NULL,   -- 0-4 (0=critical, 4=backlog)
    bead_id TEXT,               -- Created bead ID
    processed BOOLEAN DEFAULT FALSE,
    resolved BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

#### GitHub API Client

Located in `internal/github/`, the integration provides:

- `FetchAndStoreFeedback()`: Fetches PR status checks, workflow runs, comments
- `CreateBeadFromFeedback()`: Creates beads using the bd CLI

Feedback types processed:
- **CI/Build failures**: Failed status checks and workflow runs
- **Test failures**: Extracted from CI logs using pattern matching
- **Lint errors**: Code style and quality issues
- **Review comments**: Actionable feedback from code reviews
- **Security issues**: Vulnerabilities and dependency issues

#### Control Plane Architecture

PR feedback polling is handled by the control plane (`cmd/control_plane.go`), not the orchestrator.
The control plane manages all scheduled background tasks via a database-driven event loop:

- **PR Feedback Task**: Polls GitHub for new PR comments, CI failures, and reviews
- **Comment Resolution Task**: Posts resolution comments when beads are closed

Feedback polling is scheduled when a PR is first created (via `sarge complete --pr <url>`),
not when work goes idle. This ensures feedback is monitored immediately, even while
other tasks are still running.

#### Feedback Processing Flow

1. PR task completes → `sarge complete --pr <url>` sets PR URL and schedules feedback polling
2. Control plane executes scheduled feedback tasks
3. `sarge work feedback` command fetches and processes feedback:
   - Queries GitHub API for PR data
   - Filters actionable items based on rules
   - Stores in pr_feedback table
   - Creates beads under work's root issue
   - Optionally adds beads to work for immediate execution
4. Claude addresses feedback beads in subsequent tasks
5. When bead is closed, resolution comment is posted to GitHub

### Commands

#### `sarge work feedback [<work-id>]`

Processes PR feedback for a work unit:

Options:
- `--dry-run`: Preview without creating beads
- `--auto-add`: Automatically add created beads to work
- `--min-priority N`: Set minimum priority for created beads (0-4)

The command:
1. Fetches PR status, comments, and workflow runs from GitHub
2. Filters for actionable feedback based on configured rules
3. Creates beads for each feedback item
4. Stores feedback in database for tracking
5. Optionally adds beads to work for immediate execution

### TUI Integration

The TUI (`sarge`) provides:
- F5 key binding for manual feedback polling
- Visual feedback indicator showing polling status
- Automatic refresh when new beads are created from feedback

### Log Parser Configuration

The system can analyze CI logs to extract specific failures using two approaches:

#### Native Parser (Default)

The built-in Go parser handles common patterns:
- Go test failures (standard `go test` and `gotestsum` output)
- Lint errors (golangci-lint, eslint style)
- Compilation errors with file:line:column format

This is fast, free, and works offline but only supports recognized patterns.

#### Claude-Based Parser

When enabled, Claude analyzes CI logs directly and creates beads for each failure found. This approach:
- Handles any programming language or test framework
- Provides detailed error context and file locations
- Creates beads with appropriate priorities (P0-P3)
- Works with complex, multi-line error messages

Enable in `.co/config.toml`:

```toml
[log_parser]
# Use Claude for log analysis instead of the Go-based parser
use_claude = true

# Model for log analysis: "haiku", "sonnet", or "opus"
# - haiku: Fastest and cheapest, good for most logs
# - sonnet: More detailed analysis for complex failures
# - opus: Most capable, use for difficult debugging
model = "haiku"
```

#### When to Use Each

| Scenario | Recommended Parser |
|----------|-------------------|
| Go/TypeScript projects with standard tooling | Native (default) |
| Polyglot monorepos with multiple languages | Claude |
| Complex test frameworks (Jest, pytest, RSpec) | Claude |
| Custom CI output formats | Claude |
| Cost-sensitive environments | Native |
| Offline development | Native |

#### Cost/Performance Tradeoffs

- **Native parser**: Zero cost, ~1ms per log
- **Claude haiku**: ~$0.01 per log, ~2-5s
- **Claude sonnet**: ~$0.03 per log, ~5-10s
- **Claude opus**: ~$0.15 per log, ~10-20s

For most projects, the native parser is sufficient. Enable Claude parsing when you need multi-language support or the native parser misses failures in your CI output.

## PR Requirements

**NEVER push directly to main.** All changes must go through a PR.

### Correct PR Workflow

When implementing changes:
1. Create a feature branch: `git checkout -b feature-name`
2. Make and commit changes on the feature branch
3. Push the feature branch: `git push -u origin feature-name`
4. Create a PR: `gh pr create --title "..." --body "..."`
5. Merge the PR: `gh pr merge --squash`
6. Delete the branch: `git branch -d feature-name`

All PRs must be squash merged.

**NEVER use `git commit --amend`.** Always create new commits. If you need to fix something, make a new commit - the squash merge will clean up the history.

## Project Model

All commands require a project context. Projects are created with `sarge proj create` and have this structure:

```
<project-dir>/
├── .co/
│   ├── config.toml      # Project configuration
│   ├── tracking.db      # SQLite coordination database
│   └── .beads/          # Beads (if project-local)
├── main/                # Symlink (local) or clone (GitHub)
│   └── .beads/          # Beads (if repo has beads)
└── <work-id>/           # Work subdirectory (e.g., w-abc, w-xyz)
    └── tree/            # Git worktree for this work
```

Beads location is determined at project creation:
- **Repo beads** (`main/.beads/`): Used when the repository already has beads initialized. Git hooks are installed for sync.
- **Project-local beads** (`.co/.beads/`): Used when the repository doesn't have beads. No hooks or sync (standalone).

## Work Concept (3-Tier Hierarchy)

The system uses a 3-tier hierarchy: **Work → Tasks → Beads**

- **Work**: Controls git worktree, feature branch, and zellij tab
- **Tasks**: Run as Claude Code sessions sequentially within work's tab
- **Beads**: Individual issues solved within tasks

## Work Status State Machine

Works have the following status states:

```
┌─────────┐         ┌────────────┐         ┌──────┐         ┌───────────┐
│ pending │ ──────► │ processing │ ──────► │ idle │ ──────► │ completed │
└─────────┘         └────────────┘         └──────┘         └───────────┘
                          │ ▲                  │ ▲
                          │ │                  │ │
                          │ └──────────────────┘ │
                          │  (new task starts)   │
                          ▼                      │
                     ┌────────┐                  │
                     │ failed │ ◄────────────────┘
                     └────────┘
                          │
                          └──► processing (co work restart)

                     ┌────────┐
                     │ merged │ ◄── (auto-detected when PR merged on GitHub)
                     └────────┘
                  (from idle or processing)
```

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

## Workflow

Two-phase workflow: **work** → **run**.

1. Create project: `sarge proj create <dir> <repo>`
   - Initializes beads: `bd init` and `bd hooks install`
   - If mise enabled (`.mise.toml` or `.tool-versions`): runs `mise trust`, `mise install`
   - If mise `setup` task defined: runs `mise run setup` (use for npm/pnpm install)

2. Create work unit from a bead: `sarge work create <bead-id>`
   - Auto-generates work ID (w-abc, w-xyz, etc.)
   - Generates branch name from bead titles, prompts for confirmation
   - Expands epics to include all child beads with transitive dependencies
   - Creates subdirectory with git worktree: `<project>/<work-id>/tree/`
   - Use `--auto` for full automated workflow (implement, review/fix loop, PR)

3. Execute work: `sarge run`
   - From work directory: `cd <work-id> && sarge run`
   - Or explicitly: `sarge run --work <work-id>`
   - Automatically creates tasks from unassigned work beads
   - Use `--plan` for LLM complexity estimation to auto-group beads
   - Use `--auto` for full automated workflow (implement, review/fix loop, PR)
   - Executes all tasks in the work sequentially:
     - Creates single zellij tab for the work
     - Tasks run in sequence within the work's tab
     - All tasks share the same worktree
     - Claude Code implements changes in the work's worktree
     - Closes beads with `bd close <id> --reason "..."`
   - Worktree persists (managed at work level, not task level)

Zellij sessions are named `sarge-<project-name>` for isolation between projects.

## Work Commands

### `sarge work create <bead-id>`
Creates a new work unit from a bead:
- Generates branch name from bead titles, prompts for confirmation
- Expands epics to include all child beads with transitive dependencies
- Auto-generates work ID (w-abc, w-xyz, etc.)
- Creates subdirectory: `<project>/<work-id>/`
- Creates git worktree: `<project>/<work-id>/tree/`
- Pushes branch and sets upstream
- Spawns orchestrator in zellij tab
- Initializes mise if configured
- Base branch comes from project config (`[repo] base_branch`, default: main)
- Use `--auto` for full automated workflow (implement, review/fix loop, PR)

### `sarge work add <bead-ids...>`
Adds beads to an existing work:
- Syntax: `sarge work add bead-4 bead-5 [--work=<id>]`
- Detects work from current directory or uses `--work` flag
- Expands epics to include all child beads
- Cannot add beads already assigned to a task

### `sarge work remove <bead-ids...>`
Removes beads from an existing work:
- Syntax: `sarge work remove bead-4 bead-5 [--work=<id>]`
- Detects work from current directory or uses `--work` flag
- Cannot remove beads already assigned to a pending/processing task

### `sarge work list`
Lists all work units with their status:
- Shows ID, status, branch, and PR URL
- Displays summary counts by status

### `sarge work show [<id>]`
Shows detailed information about a work:
- If no ID provided, detects from current directory
- Displays status, branch, worktree path, PR URL
- Lists associated beads and tasks with their status

### `sarge work destroy <id>`
Destroys a work unit and its resources:
- Removes git worktree
- Deletes work subdirectory
- Updates database records
- Use with caution - destructive operation

### `sarge work restart [<id>]`
Restarts a failed work:
- Only works if work is in `failed` status
- Transitions work back to `processing`
- Orchestrator will resume processing pending tasks
- Use after fixing the issue that caused the failure (e.g., reset/delete failed task)

### `sarge work complete [<id>]`
Explicitly marks an idle work as completed:
- Only works if work is in `idle` status
- Transitions work to `completed` (terminal state)
- Use when PR is merged or work is truly finished

## Task Commands

### `sarge task list`
Lists all tasks with their status:
- Shows ID, status, type, budget, creation time, and associated beads
- Filter by status: `--status pending|processing|completed|failed`
- Filter by type: `--type estimate|implement`

### `sarge task show <id>`
Shows detailed information about a task:
- Displays status, type, budget, timestamps
- Lists associated beads and their completion status
- Shows worktree path, zellij session/pane if applicable

### `sarge task delete <id>...`
Deletes one or more tasks from the database:
- Removes task and all associated records
- Accepts multiple task IDs

### `sarge task reset <id>`
Resets a failed or stuck task to pending:
- Changes task status from processing/failed back to pending
- Resets all bead statuses for the task
- Use when a task gets stuck or needs to be rerun

### `sarge task set-review-epic <epic-id>`
Associates a review epic with a review task:
- Sets the review_epic_id metadata on a review task
- Task is auto-detected from CO_TASK_ID env var or current processing review task
- Use `--task` flag for explicit specification

## Additional User Commands

### `sarge list`
Lists tracked beads in the database:
- Filter by status with `--status` (pending, processing, completed, failed)
- Shows ID, status, title, and PR URL

### `sarge status [bead-id]`
Shows bead tracking status:
- With a bead ID: shows detailed status including zellij session/pane info
- Without ID: shows all beads currently processing with their session/pane

### `sarge poll [work-id|task-id]`
Monitors work/task progress with simple text output:
- Without arguments: monitors all active works or detected work from directory
- With work ID: monitors that work's tasks
- With task ID: monitors that specific task
- Use `--interval` to set polling interval (default: 2s)
- For interactive TUI with management features, use `sarge` instead

### `sarge sync`
Pulls from upstream in all repositories:
- Runs git pull in each worktree (main and all work worktrees)
- Skips internal beads worktrees

### `sarge work pr [<id>]`
Creates a PR task for Claude to generate a pull request:
- Work must be completed before creating PR
- If no ID provided, uses work from current directory

### `sarge work review [<id>]`
Creates a review task to examine code changes:
- Claude examines the work's branch for quality and security issues
- Creates review task with sequential numeric ID (w-xxx.5, w-xxx.6, etc.)
- Use `--auto` for review-fix loop until clean (max 3 iterations)

## Agent Commands

These commands are called by Claude Code or the orchestration system during task execution. They are not intended for direct user invocation. In help output, they are prefixed with `[Agent]`.

### `sarge complete <bead-id|task-id>`
Marks a bead or task as completed (or failed with --error):
- Called by Claude Code when work is done
- Supports both bead IDs and task IDs (task IDs contain dots like "w-xxx.1")
- Use `--error "message"` to mark a task as failed
- Use `--pr "url"` to associate a PR URL with completion

### `sarge estimate <bead-id>`
Reports complexity estimate for a bead:
- Called by Claude Code during estimation tasks
- Required flags: `--score` (1-10) and `--tokens` (5000-50000)
- Optional: `--task` to specify the task ID

## Internal Commands (Hidden)

These commands are hidden from help output and spawned automatically by the orchestration system. Users should never need to run these directly.

### `sarge orchestrate`
Executes tasks for a work unit:
- Polls for ready tasks and executes them sequentially
- Runs in a zellij tab, spawned automatically when work is created
- Monitors task completion and handles post-execution workflows

### `sarge control`
Runs the control plane for background task execution:
- Long-lived process that watches for scheduled tasks across all works
- Executes tasks asynchronously with retry support
- Spawned automatically in a zellij tab named "control"
- Handles task types: CreateWorktree, SpawnOrchestrator, DestroyWorktree, PRFeedback, CommentResolution, GitPush, GitHubComment, GitHubResolveThread
- Uses database change events for reactive execution (with 30s periodic fallback)
- Managed via `SpawnControlPlane()` and `EnsureControlPlane()` functions
