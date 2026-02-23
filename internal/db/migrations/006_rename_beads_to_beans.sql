-- +up
-- Rename beads→beans in all tables and columns

-- Rename main tables
ALTER TABLE beads RENAME TO beans;
ALTER TABLE task_beads RENAME TO task_beans;
ALTER TABLE work_beads RENAME TO work_beans;

-- Rename bead_id columns to bean_id
ALTER TABLE task_beans RENAME COLUMN bead_id TO bean_id;
ALTER TABLE work_beans RENAME COLUMN bead_id TO bean_id;
ALTER TABLE complexity_cache RENAME COLUMN bead_id TO bean_id;
ALTER TABLE plan_sessions RENAME COLUMN bead_id TO bean_id;
ALTER TABLE pr_feedback RENAME COLUMN bead_id TO bean_id;

-- +down
-- Reverse the renames

ALTER TABLE pr_feedback RENAME COLUMN bean_id TO bead_id;
ALTER TABLE plan_sessions RENAME COLUMN bean_id TO bead_id;
ALTER TABLE complexity_cache RENAME COLUMN bean_id TO bead_id;
ALTER TABLE work_beans RENAME COLUMN bean_id TO bead_id;
ALTER TABLE task_beans RENAME COLUMN bean_id TO bead_id;

ALTER TABLE work_beans RENAME TO work_beads;
ALTER TABLE task_beans RENAME TO task_beads;
ALTER TABLE beans RENAME TO beads;
