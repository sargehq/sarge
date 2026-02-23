---
# main-oibi
title: Update file watcher for .beans/ markdown files
status: completed
type: task
priority: high
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:14:58Z
parent: main-1icu
---

Watch .beans/ directory for markdown file changes instead of sqlite DB changes.
- Use fsnotify on .beans/*.md files
- Trigger events on file create/modify/delete
- Map to same event types consumers expect

## Files
- watcher/watcher.go
- watcher/watcher_test.go
