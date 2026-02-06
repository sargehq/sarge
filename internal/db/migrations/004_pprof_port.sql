-- +up
-- Add pprof_port column to processes table
-- Stores the ephemeral port the process bound its pprof HTTP server to, or NULL if pprof is disabled
ALTER TABLE processes ADD COLUMN pprof_port INTEGER;

-- +down
-- SQLite doesn't support DROP COLUMN directly, but we document the intent
-- This requires recreating the table without the column
