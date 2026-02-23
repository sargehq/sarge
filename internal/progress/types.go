package progress

import (
	"github.com/sargehq/sarge/internal/db"
)

// WorkProgress holds progress info for a work unit.
type WorkProgress struct {
	Work                *db.Work
	Tasks               []*TaskProgress
	WorkBeans           []BeanProgress // all beads assigned to this work
	UnassignedBeads     []BeanProgress // beads in work but not assigned to any task
	UnassignedBeadCount int
	FeedbackCount       int      // count of unresolved PR feedback items
	FeedbackBeanIDs     []string // bead IDs from unassigned PR feedback

	// PR status fields (populated from work record)
	CIStatus           string   // pending, success, failure
	ApprovalStatus     string   // pending, approved, changes_requested
	Approvers          []string // list of usernames who approved
	HasUnseenPRChanges bool     // true if there are unseen PR changes
	MergeableState     string   // CLEAN, DIRTY, BLOCKED, BEHIND, DRAFT, UNSTABLE, UNKNOWN
}

// TaskProgress holds progress info for a task.
type TaskProgress struct {
	Task  *db.Task
	Beads []BeanProgress
}

// BeanProgress holds progress info for a bead.
type BeanProgress struct {
	ID          string
	Status      string
	Title       string
	Description string
	BeanStatus  string // status from beads (open/closed)
	Priority    int
	IssueType   string
}
