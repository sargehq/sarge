package tui

import (
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

// WorkTabSubSession identifies which sub-session is active within a work tab.
type WorkTabSubSession int

const (
	// SubSessionAgent is the agent/chat pi session.
	SubSessionAgent WorkTabSubSession = iota
	// SubSessionConsole is the terminal/console pi session.
	SubSessionConsole
	// SubSessionPlan is a planning pi session within a work.
	SubSessionPlan
)

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

	// ActiveSubSession is which sub-session is currently shown (work tabs only).
	ActiveSubSession WorkTabSubSession

	// SessionMaximized is true when the session panel is zoomed to fill the content area.
	SessionMaximized bool

	// --- Plan tab fields ---

	// BeanID is set for WorkTabPlan tabs (the bean being planned).
	BeanID string

	// --- Session references ---
	// These are looked up from the PTY manager by convention:
	//   default:  session ID "main"
	//   agent:    session ID "agent-<workID>"
	//   console:  session ID "console-<workID>"
	//   plan:     session ID "plan-<beanID>"
}

// SessionID returns the PTY session ID for the currently active session in this tab.
func (t *WorkTab) SessionID() string {
	switch t.Type {
	case WorkTabDefault:
		return "main"
	case WorkTabWork:
		switch t.ActiveSubSession {
		case SubSessionAgent:
			return "agent-" + t.WorkID
		case SubSessionConsole:
			return "console-" + t.WorkID
		case SubSessionPlan:
			// Plan within a work — uses the root bean ID
			return "plan-" + t.WorkID
		}
		return "agent-" + t.WorkID
	case WorkTabPlan:
		return "plan-" + t.BeanID
	}
	return ""
}

// ActiveSession returns the PTY session for the currently active sub-session,
// or nil if no session is running.
func (t *WorkTab) ActiveSession(mgr *ptysession.Manager) *ptysession.Session {
	if mgr == nil {
		return nil
	}
	return mgr.Get(t.SessionID())
}

// HasActiveSession returns true if the tab has a running PTY session.
func (t *WorkTab) HasActiveSession(mgr *ptysession.Manager) bool {
	s := t.ActiveSession(mgr)
	return s != nil && s.State() != ptysession.SessionDead
}

// CycleSubSession cycles to the next sub-session within a work tab.
func (t *WorkTab) CycleSubSession() {
	if t.Type != WorkTabWork {
		return
	}
	switch t.ActiveSubSession {
	case SubSessionAgent:
		t.ActiveSubSession = SubSessionConsole
	case SubSessionConsole:
		t.ActiveSubSession = SubSessionPlan
	case SubSessionPlan:
		t.ActiveSubSession = SubSessionAgent
	}
}

// SetSubSession sets the active sub-session by index (1=agent, 2=console, 3=plan).
func (t *WorkTab) SetSubSession(index int) {
	if t.Type != WorkTabWork {
		return
	}
	switch index {
	case 1:
		t.ActiveSubSession = SubSessionAgent
	case 2:
		t.ActiveSubSession = SubSessionConsole
	case 3:
		t.ActiveSubSession = SubSessionPlan
	}
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
		ID:               workID,
		Type:             WorkTabWork,
		Label:            label,
		WorkID:           workID,
		ActiveSubSession: SubSessionAgent,
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
