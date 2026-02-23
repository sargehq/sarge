package beans

// Bean status constants.
// These are the canonical status values used by the beans issue tracker.
const (
	StatusTodo       = "todo"        // Default status for new issues
	StatusInProgress = "in-progress" // Issue is being actively worked on
	StatusDraft      = "draft"       // Issue is not yet ready
	StatusCompleted  = "completed"   // Issue is completed or resolved
	StatusScrapped   = "scrapped"    // Issue was abandoned
)

// Priority constants for beans (string-based, not numeric).
const (
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityNormal   = "normal"
	PriorityLow      = "low"
	PriorityDeferred = "deferred"
)

// IsWorkableStatus returns true if the status indicates the bean can be worked on.
// This includes todo and in-progress statuses (not draft, completed, or scrapped).
func IsWorkableStatus(status string) bool {
	switch status {
	case StatusTodo, StatusInProgress, "":
		return true
	default:
		return false
	}
}

// IsTerminalStatus returns true if the status indicates the bean is done.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusCompleted, StatusScrapped:
		return true
	default:
		return false
	}
}

// PriorityFromInt converts legacy numeric priority (0-4) to beans string priority.
func PriorityFromInt(p int) string {
	switch p {
	case 0:
		return PriorityCritical
	case 1:
		return PriorityHigh
	case 2:
		return PriorityNormal
	case 3:
		return PriorityLow
	case 4:
		return PriorityDeferred
	default:
		return PriorityNormal
	}
}
