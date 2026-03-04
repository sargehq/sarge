-- Schema for co project tracking database
-- Reference schema for sqlc code generation
-- The actual source of truth is in internal/db/migrations/

-- Works table: tracks work units (groups of tasks)
CREATE TABLE works (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    name TEXT NOT NULL DEFAULT '',
    worktree_path TEXT NOT NULL DEFAULT '',
    branch_name TEXT NOT NULL DEFAULT '',
    base_branch TEXT NOT NULL DEFAULT 'main',
    root_issue_id TEXT NOT NULL DEFAULT '',
    pr_url TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    auto BOOLEAN NOT NULL DEFAULT FALSE,
    ci_status TEXT NOT NULL DEFAULT 'pending',
    approval_status TEXT NOT NULL DEFAULT 'pending',
    approvers TEXT NOT NULL DEFAULT '[]',
    last_pr_poll_at DATETIME,
    has_unseen_pr_changes BOOLEAN NOT NULL DEFAULT FALSE,
    pr_state TEXT NOT NULL DEFAULT '',
    mergeable_state TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_works_status ON works(status);
CREATE INDEX idx_works_root_issue_id ON works(root_issue_id);

-- Beans table: tracks individual beans
CREATE TABLE beans (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    title TEXT NOT NULL DEFAULT '',
    pr_url TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    worktree_path TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_beans_status ON beans(status);

-- Tasks table: tracks virtual tasks (groups of beans)
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    task_type TEXT NOT NULL DEFAULT 'implement',
    complexity_budget INT NOT NULL DEFAULT 0,
    actual_complexity INT NOT NULL DEFAULT 0,
    work_id TEXT NOT NULL DEFAULT '' REFERENCES works(id),
    worktree_path TEXT NOT NULL DEFAULT '',
    pr_url TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    spawned_at DATETIME,
    spawn_status TEXT NOT NULL DEFAULT '',
    last_activity DATETIME,
    task_number INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_work_id ON tasks(work_id);

-- Task-beans junction table: links tasks to their beans
CREATE TABLE task_beans (
    task_id TEXT NOT NULL,
    bean_id TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    PRIMARY KEY (task_id, bean_id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_task_beans_task_id ON task_beans(task_id);
CREATE INDEX idx_task_beans_bean_id ON task_beans(bean_id);

-- Complexity cache: stores LLM complexity estimates
CREATE TABLE complexity_cache (
    bean_id TEXT PRIMARY KEY,
    description_hash TEXT NOT NULL,
    complexity_score INT NOT NULL,
    estimated_tokens INT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_complexity_cache_hash ON complexity_cache(description_hash);

-- Work-tasks junction table: links works to their tasks
CREATE TABLE work_tasks (
    work_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (work_id, task_id),
    FOREIGN KEY (work_id) REFERENCES works(id),
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX idx_work_tasks_work_id ON work_tasks(work_id);
CREATE INDEX idx_work_tasks_task_id ON work_tasks(task_id);

-- Work task counters: tracks next task number for each work
CREATE TABLE work_task_counters (
    work_id TEXT PRIMARY KEY,
    next_task_num INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
);

-- Task metadata table: stores key-value metadata on tasks
CREATE TABLE task_metadata (
    task_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, key),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX idx_task_metadata_task_id ON task_metadata(task_id);
CREATE INDEX idx_task_metadata_key ON task_metadata(key);

-- Task dependencies table: tracks dependencies between tasks
-- A task can depend on multiple other tasks, and those must complete before it can run
CREATE TABLE task_dependencies (
    task_id TEXT NOT NULL,
    depends_on_task_id TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, depends_on_task_id),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (depends_on_task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX idx_task_dependencies_task_id ON task_dependencies(task_id);
CREATE INDEX idx_task_dependencies_depends_on ON task_dependencies(depends_on_task_id);

-- Work beans table: beans assigned to work
CREATE TABLE work_beans (
    work_id TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    bean_id TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,  -- ordering within work
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (work_id, bean_id)
);

CREATE INDEX idx_work_beans_work_id ON work_beans(work_id);
CREATE INDEX idx_work_beans_bean_id ON work_beans(bean_id);

-- Schema migrations table: tracks applied database migrations
CREATE TABLE schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    name TEXT NOT NULL DEFAULT '',
    down_sql TEXT NOT NULL DEFAULT ''
);

-- Plan sessions table: tracks running plan mode Claude sessions per bean
CREATE TABLE plan_sessions (
    bean_id TEXT PRIMARY KEY,
    session_name TEXT NOT NULL,
    tab_name TEXT NOT NULL,
    pid INTEGER NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_plan_sessions_session_name ON plan_sessions(session_name);

-- PR Feedback table: tracks feedback from PRs (comments, CI failures, etc.)
CREATE TABLE pr_feedback (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL,
    pr_url TEXT NOT NULL,
    feedback_type TEXT NOT NULL, -- ci_failure, test_failure, lint_error, build_error, review_comment, security_issue, general
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    source_url TEXT,
    source_id TEXT, -- GitHub comment/check ID for resolution tracking
    source_type TEXT, -- Structured type: ci, workflow, review_comment, issue_comment
    source_name TEXT, -- Human-readable name (check name, workflow name, reviewer username)
    context TEXT, -- JSON: structured context data (FeedbackContext)
    priority INTEGER NOT NULL DEFAULT 2, -- 0-4 (0=critical, 4=backlog)
    bean_id TEXT, -- ID of the bean created from this feedback (null if not created yet)
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_at DATETIME, -- When the feedback was processed to create a bean
    resolved_at DATETIME, -- When the feedback was resolved on GitHub
    FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
);

CREATE INDEX idx_pr_feedback_work_id ON pr_feedback(work_id);
CREATE INDEX idx_pr_feedback_bean_id ON pr_feedback(bean_id);
CREATE INDEX idx_pr_feedback_processed ON pr_feedback(processed_at);
CREATE INDEX idx_pr_feedback_resolution ON pr_feedback(bean_id, resolved_at);
CREATE INDEX idx_pr_feedback_source_type ON pr_feedback(source_type);

-- Scheduler table: manages scheduled tasks like PR feedback checks
CREATE TABLE scheduler (
    id TEXT PRIMARY KEY,
    work_id TEXT NOT NULL,
    task_type TEXT NOT NULL, -- 'pr_feedback', 'comment_resolution', 'git_push', 'github_comment', 'github_resolve_thread', etc.
    scheduled_at DATETIME NOT NULL,
    executed_at DATETIME,
    status TEXT NOT NULL DEFAULT 'pending', -- 'pending', 'executing', 'completed', 'failed'
    error_message TEXT,
    metadata TEXT NOT NULL DEFAULT '{}', -- JSON metadata
    attempt_count INTEGER NOT NULL DEFAULT 0, -- Number of attempts made
    max_attempts INTEGER NOT NULL DEFAULT 5, -- Maximum retry attempts
    idempotency_key TEXT, -- Unique key for deduplication
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE CASCADE
);

CREATE INDEX idx_scheduler_work_id ON scheduler(work_id);
CREATE INDEX idx_scheduler_status ON scheduler(status);
CREATE INDEX idx_scheduler_scheduled_at ON scheduler(scheduled_at);
CREATE INDEX idx_scheduler_task_type ON scheduler(work_id, task_type);
CREATE UNIQUE INDEX idx_scheduler_idempotency_key ON scheduler(idempotency_key) WHERE idempotency_key IS NOT NULL;

-- Processes table: tracks running orchestrator and control plane processes with heartbeats
-- Enables detection of hung processes (alive but not making progress) via stale heartbeats
CREATE TABLE processes (
    id TEXT PRIMARY KEY,
    process_type TEXT NOT NULL,           -- 'orchestrator' or 'control_plane'
    work_id TEXT,                          -- work ID for orchestrators (NULL for control plane)
    pid INTEGER NOT NULL,                  -- OS process ID
    hostname TEXT NOT NULL DEFAULT '',     -- machine hostname
    heartbeat DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    pprof_port INTEGER                    -- ephemeral port for pprof HTTP server (NULL if disabled)
);

-- Index for looking up by type
CREATE INDEX idx_processes_type ON processes(process_type);

-- Index for looking up orchestrators by work_id
CREATE INDEX idx_processes_work_id ON processes(work_id);

-- Index for finding stale heartbeats
CREATE INDEX idx_processes_heartbeat ON processes(heartbeat);

-- Unique partial index: only one orchestrator per work
CREATE UNIQUE INDEX idx_processes_unique_orchestrator ON processes(work_id)
    WHERE process_type = 'orchestrator' AND work_id IS NOT NULL;

-- Unique partial index: only one control plane per project
CREATE UNIQUE INDEX idx_processes_unique_control_plane ON processes(process_type)
    WHERE process_type = 'control_plane';
