---
# main-s8ng
title: Fix beans-prime extension running on every prompt
status: completed
type: bug
priority: normal
created_at: 2026-02-25T05:28:43Z
updated_at: 2026-02-25T05:28:47Z
---

The beans-prime.ts pi extension uses before_agent_start which fires on every user prompt, causing beans prime to run repeatedly. Added a hasPrimed guard so it only runs once per session.

## Summary of Changes\n\nAdded a `hasPrimed` guard to `internal/agentsetup/extensions/beans-prime.ts` so `beans prime` only runs once per session (on the first `before_agent_start` event), not on every user prompt.
