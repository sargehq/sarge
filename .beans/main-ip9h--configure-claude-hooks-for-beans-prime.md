---
# main-ip9h
title: Configure Claude hooks for beans prime
status: todo
type: task
priority: low
created_at: 2026-02-23T02:07:21Z
updated_at: 2026-02-23T02:07:21Z
parent: main-5uxi
---

Optional: Add beans prime hooks to .claude/settings.json:
```json
{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "beans prime" }] }
    ],
    "PreCompact": [
      { "hooks": [{ "type": "command", "command": "beans prime" }] }
    ]
  }
}
```

## Files
- .claude/settings.json or .claude/settings.local.json
