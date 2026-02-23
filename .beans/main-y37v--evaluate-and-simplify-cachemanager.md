---
# main-y37v
title: Evaluate and simplify cachemanager
status: completed
type: task
priority: normal
created_at: 2026-02-23T02:06:09Z
updated_at: 2026-02-23T02:14:58Z
parent: main-1icu
---

Beans GraphQL/CLI reads are file-based and fast. Evaluate whether the sqlite query caching layer is still needed.
- If beans CLI is fast enough: delete cachemanager/
- If caching still needed: adapt to cache CLI/GraphQL results instead of sqlite queries

## Files
- cachemanager/
