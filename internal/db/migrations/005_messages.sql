-- +up
CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL,
    text TEXT NOT NULL,
    work_id TEXT,
    task_id TEXT,
    bead_id TEXT,
    event_type TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (work_id) REFERENCES works(id) ON DELETE SET NULL
);
CREATE INDEX idx_messages_created_at ON messages(created_at);
CREATE INDEX idx_messages_work_id ON messages(work_id);
CREATE INDEX idx_messages_event_type ON messages(event_type);

-- +down
DROP INDEX IF EXISTS idx_messages_event_type;
DROP INDEX IF EXISTS idx_messages_work_id;
DROP INDEX IF EXISTS idx_messages_created_at;
DROP TABLE IF EXISTS messages;
