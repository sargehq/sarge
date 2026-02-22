-- +up
-- Add task_number column to tasks table for reliable numeric ordering.
-- The number is extracted from the composite string ID (e.g., "work-1.14" -> 14).
ALTER TABLE tasks ADD COLUMN task_number INTEGER NOT NULL DEFAULT 0;

-- Populate from existing IDs: extract the number after the last '.'
UPDATE tasks SET task_number = CAST(SUBSTR(id, INSTR(id, '.') + 1) AS INTEGER)
WHERE INSTR(id, '.') > 0;

-- +down
-- SQLite doesn't support DROP COLUMN directly, but we document the intent
