package tui

import (
	"sort"
	"strings"

	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/ptysession"
)

// WorkTabType represents the type of a work tab.
type WorkTabType int

const (
	// WorkTabDefault is the always-present "Main" tab with a global pi session.
	WorkTabDefault WorkTabType = iota
	// WorkTabWork is a tab for an active work unit (has work details + sessions).
	WorkTabWork
	// WorkTabPlan is a tab for a bean planning session.
	WorkTabPlan
)

// SubTab represents one sub-tab within a work tab.
type SubTab struct {
	ID    string // PTY session ID (e.g. "task-w-5d0.2", "agent-w-5d0", "console-w-5d0")
	Label string // Display label (e.g. "task .2", "agent", "console")
	Type  string // "task", "agent", "console", "plan"
}

// WorkTab represents a single tab in the work tabs bar.
// It can be a default session, a work unit, or a standalone plan session.
type WorkTab struct {
	// ID is the unique identifier for this tab.
	// For default: "main"
	// For work: the work ID (e.g., "w-abc")
	// For plan: "plan-<beanID>"
	ID string

	// Type determines the tab behavior and rendering.
	Type WorkTabType

	// Label is the display name shown in the tab bar.
	Label string

	// --- Work tab fields ---

	// WorkID is set for WorkTabWork tabs.
	WorkID string

	// WorkProgress holds cached work data (tasks, beans, etc.) for work tabs.
	WorkProgress *progress.WorkProgress

	// ActiveSessionID is the PTY session ID currently displayed.
	// Empty means "auto" — pick the first available task or agent session.
	ActiveSessionID string

	// SessionMaximized is true when the session panel is zoomed to fill the content area.
	SessionMaximized bool

	// --- Plan tab fields ---

	// BeanID is set for WorkTabPlan tabs (the bean being planned).
	BeanID string
}

// AvailableSessions returns all sub-tabs for this work, discovered from the PTY manager.
// Returns them in a stable order: task sessions (sorted by ID), then agent, console, plan.
func (t *WorkTab) AvailableSessions(mgr *ptysession.Manager) []SubTab {
	if mgr == nil || t.Type != WorkTabWork {
		return nil
	}

	var tasks []SubTab
	var others []SubTab

	prefix := t.WorkID
	for id, state := range mgr.List() {
		if state == ptysession.SessionDead {
			continue
		}

		if strings.HasPrefix(id, "task-"+prefix+".") {
			// Extract task number suffix for label (e.g., "task-w-5d0.2" -> ".2")
			suffix := id[len("task-"+prefix):]
			tasks = append(tasks, SubTab{
				ID:    id,
				Label: "task" + suffix,
				Type:  "task",
			})
		} else if id == "agent-"+prefix {
			others = append(others, SubTab{
				ID:    id,
				Label: "agent",
				Type:  "agent",
			})
		} else if id == "console-"+prefix {
			others = append(others, SubTab{
				ID:    id,
				Label: "console",
				Type:  "console",
			})
		} else if id == "plan-"+prefix {
			others = append(others, SubTab{
				ID:    id,
				Label: "plan",
				Type:  "plan",
			})
		}
	}

	// Sort task sessions by ID for stable ordering
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	return append(tasks, others...)
}

// SessionID returns the PTY session ID to display for this tab.
func (t *WorkTab) SessionID() string {
	switch t.Type {
	case WorkTabDefault:
		return "main"
	case WorkTabWork:
		if t.ActiveSessionID != "" {
			return t.ActiveSessionID
		}
		// Default: return agent session ID (may not exist)
		return "agent-" + t.WorkID
	case WorkTabPlan:
		return "plan-" + t.BeanID
	}
	return ""
}

// ResolveSessionID picks the best available session to display.
// If ActiveSessionID is set and alive, keeps it. Otherwise picks the first available.
func (t *WorkTab) ResolveSessionID(mgr *ptysession.Manager) string {
	if t.Type != WorkTabWork {
		return t.SessionID()
	}

	// If explicitly set and still alive, use it
	if t.ActiveSessionID != "" {
		if s := mgr.Get(t.ActiveSessionID); s != nil && s.State() != ptysession.SessionDead {
			return t.ActiveSessionID
		}
	}

	// Auto-pick: first available from AvailableSessions
	subs := t.AvailableSessions(mgr)
	if len(subs) > 0 {
		t.ActiveSessionID = subs[0].ID
		return subs[0].ID
	}

	return ""
}

// ActiveSession returns the PTY session for the currently active sub-session,
// or nil if no session is running.
func (t *WorkTab) ActiveSession(mgr *ptysession.Manager) *ptysession.Session {
	if mgr == nil {
		return nil
	}
	id := t.ResolveSessionID(mgr)
	if id == "" {
		return nil
	}
	return mgr.Get(id)
}

// HasActiveSession returns true if the tab has a running PTY session.
func (t *WorkTab) HasActiveSession(mgr *ptysession.Manager) bool {
	s := t.ActiveSession(mgr)
	return s != nil && s.State() != ptysession.SessionDead
}

// CycleSubSession cycles to the next available sub-session within a work tab.
func (t *WorkTab) CycleSubSession(mgr *ptysession.Manager) {
	if t.Type != WorkTabWork || mgr == nil {
		return
	}
	subs := t.AvailableSessions(mgr)
	if len(subs) <= 1 {
		return
	}
	current := t.ResolveSessionID(mgr)
	for i, sub := range subs {
		if sub.ID == current {
			t.ActiveSessionID = subs[(i+1)%len(subs)].ID
			return
		}
	}
	t.ActiveSessionID = subs[0].ID
}

// SetSubSessionByIndex sets the active sub-session by 1-based index into AvailableSessions.
func (t *WorkTab) SetSubSessionByIndex(mgr *ptysession.Manager, index int) {
	if t.Type != WorkTabWork || mgr == nil {
		return
	}
	subs := t.AvailableSessions(mgr)
	if index < 1 || index > len(subs) {
		return
	}
	t.ActiveSessionID = subs[index-1].ID
}

// ActiveSubTabIndex returns the 0-based index of the active session in AvailableSessions.
// Returns -1 if not found.
func (t *WorkTab) ActiveSubTabIndex(mgr *ptysession.Manager) int {
	if mgr == nil {
		return -1
	}
	subs := t.AvailableSessions(mgr)
	current := t.ResolveSessionID(mgr)
	for i, sub := range subs {
		if sub.ID == current {
			return i
		}
	}
	return -1
}

// NewDefaultTab creates the always-present "Main" tab.
func NewDefaultTab() *WorkTab {
	return &WorkTab{
		ID:    "main",
		Type:  WorkTabDefault,
		Label: "Main",
	}
}

// NewWorkTab creates a tab for an active work unit.
func NewWorkTab(workID string, label string) *WorkTab {
	if label == "" {
		label = workID
	}
	return &WorkTab{
		ID:     workID,
		Type:   WorkTabWork,
		Label:  label,
		WorkID: workID,
	}
}

// NewPlanTab creates a tab for a standalone bean planning session.
func NewPlanTab(beanID string, label string) *WorkTab {
	if label == "" {
		label = "plan:" + beanID
	}
	return &WorkTab{
		ID:     "plan-" + beanID,
		Type:   WorkTabPlan,
		Label:  label,
		BeanID: beanID,
	}
}
