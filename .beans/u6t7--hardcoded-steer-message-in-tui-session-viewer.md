---
# u6t7
title: Hardcoded steer message in TUI session viewer
status: completed
type: task
priority: normal
tags:
    - review-w-2fo.14
created_at: 2026-03-01T22:54:09Z
updated_at: 2026-03-02T21:49:02Z
parent: ir72
---

In internal/tui/tui_plan.go:1216, the steer action sends a hardcoded 'Please adjust your approach' message instead of getting input from the user. There is a TODO comment acknowledging this. The steer action should use the text input to get the user's steer message, similar to how SessionPanelActionPrompt uses GetPendingPrompt().

## Summary of Changes\n\nReplaced hardcoded steer message with user input:\n- Added steerMode flag to SessionPanel for tracking steer input state\n- ctrl+s now enters input mode with steer placeholder instead of immediately sending\n- Added GetPendingSteer() method to retrieve and clear steer input\n- Updated tui_plan.go handler to use GetPendingSteer() instead of hardcoded string\n- Steer messages are displayed in the session output with ⟳ prefix
