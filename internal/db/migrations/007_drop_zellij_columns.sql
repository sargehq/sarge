-- +migrate Up
-- Drop zellij-specific columns from works and beans tables.
-- These columns were used for zellij terminal multiplexer integration
-- which has been replaced by the bridge architecture.

-- SQLite doesn't support DROP COLUMN before 3.35.0, so we recreate the tables.

-- Recreate works table without zellij columns
CREATE TABLE works_new (
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

INSERT INTO works_new (id, status, name, worktree_path, branch_name, base_branch,
    root_issue_id, pr_url, error_message, started_at, completed_at, created_at,
    auto, ci_status, approval_status, approvers, last_pr_poll_at,
    has_unseen_pr_changes, pr_state, mergeable_state)
SELECT id, status, name, worktree_path, branch_name, base_branch,
    root_issue_id, pr_url, error_message, started_at, completed_at, created_at,
    auto, ci_status, approval_status, approvers, last_pr_poll_at,
    has_unseen_pr_changes, pr_state, mergeable_state
FROM works;

DROP TABLE works;
ALTER TABLE works_new RENAME TO works;

CREATE INDEX idx_works_status ON works(status);
CREATE INDEX idx_works_root_issue_id ON works(root_issue_id);

-- Recreate beans table without zellij columns
CREATE TABLE beans_new (
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

INSERT INTO beans_new (id, status, title, pr_url, error_message, worktree_path,
    started_at, completed_at, created_at, updated_at)
SELECT id, status, title, pr_url, error_message, worktree_path,
    started_at, completed_at, created_at, updated_at
FROM beans;

DROP TABLE beans;
ALTER TABLE beans_new RENAME TO beans;

CREATE INDEX idx_beans_status ON beans(status);

-- Rename zellij_session to session_name in plan_sessions
CREATE TABLE plan_sessions_new (
    bean_id TEXT PRIMARY KEY,
    session_name TEXT NOT NULL,
    tab_name TEXT NOT NULL,
    pid INTEGER NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO plan_sessions_new (bean_id, session_name, tab_name, pid, started_at)
SELECT bean_id, zellij_session, tab_name, pid, started_at
FROM plan_sessions;

DROP TABLE plan_sessions;
ALTER TABLE plan_sessions_new RENAME TO plan_sessions;

CREATE INDEX idx_plan_sessions_session_name ON plan_sessions(session_name);

-- +migrate Down
-- Recreate works with zellij columns
CREATE TABLE works_old (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    name TEXT NOT NULL DEFAULT '',
    zellij_session TEXT NOT NULL DEFAULT '',
    zellij_tab TEXT NOT NULL DEFAULT '',
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

INSERT INTO works_old (id, status, name, worktree_path, branch_name, base_branch,
    root_issue_id, pr_url, error_message, started_at, completed_at, created_at,
    auto, ci_status, approval_status, approvers, last_pr_poll_at,
    has_unseen_pr_changes, pr_state, mergeable_state)
SELECT id, status, name, worktree_path, branch_name, base_branch,
    root_issue_id, pr_url, error_message, started_at, completed_at, created_at,
    auto, ci_status, approval_status, approvers, last_pr_poll_at,
    has_unseen_pr_changes, pr_state, mergeable_state
FROM works;

DROP TABLE works;
ALTER TABLE works_old RENAME TO works;

CREATE INDEX idx_works_status ON works(status);
CREATE INDEX idx_works_root_issue_id ON works(root_issue_id);

-- Recreate beans with zellij columns
CREATE TABLE beans_old (
    id TEXT PRIMARY KEY,
    status TEXT NOT NULL DEFAULT 'pending',
    title TEXT NOT NULL DEFAULT '',
    pr_url TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    zellij_session TEXT NOT NULL DEFAULT '',
    zellij_pane TEXT NOT NULL DEFAULT '',
    worktree_path TEXT NOT NULL DEFAULT '',
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO beans_old (id, status, title, pr_url, error_message, worktree_path,
    started_at, completed_at, created_at, updated_at)
SELECT id, status, title, pr_url, error_message, worktree_path,
    started_at, completed_at, created_at, updated_at
FROM beans;

DROP TABLE beans;
ALTER TABLE beans_old RENAME TO beans;

CREATE INDEX idx_beans_status ON beans(status);

-- Recreate plan_sessions with zellij_session
CREATE TABLE plan_sessions_old (
    bean_id TEXT PRIMARY KEY,
    zellij_session TEXT NOT NULL,
    tab_name TEXT NOT NULL,
    pid INTEGER NOT NULL,
    started_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO plan_sessions_old (bean_id, zellij_session, tab_name, pid, started_at)
SELECT bean_id, session_name, tab_name, pid, started_at
FROM plan_sessions;

DROP TABLE plan_sessions;
ALTER TABLE plan_sessions_old RENAME TO plan_sessions;

CREATE INDEX idx_plan_sessions_zellij_session ON plan_sessions(zellij_session);
