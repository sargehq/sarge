---
# main-lbo4
title: 'Debug: Details panel height mismatch with Issues panel'
status: completed
type: bug
priority: normal
created_at: 2026-03-13T03:59:20Z
updated_at: 2026-03-13T04:05:33Z
---

The Details panel (right column) renders taller than the Issues panel (left column) in the two-column layout. Add debug logging to understand why.

## Summary of Changes\n\nDiagnosed via debug logging that lipgloss `.Height()` only sets minimum height, not maximum. The Details panel content was wrapping inside its borders, pushing rendered height from 28 to 30 lines while Issues stayed at 28.\n\nFix: Added `.MaxHeight(contentHeight)` to both panels' lipgloss styles. Removed the manual overflow truncation hack from Issues panel since MaxHeight handles it properly.
