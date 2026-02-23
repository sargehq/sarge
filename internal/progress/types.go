package progress

import (
	"github.com/sargehq/sarge/internal/db"
)

// WorkProgress holds progress info for a work unit.
type WorkProgress struct {
	Work                *db.Work
	Tasks               []*TaskProgress
	WorkBeans           []BeanProgress // all beans assigned to this work
	UnassignedBeans     []BeanProgress // beans in work but not assigned to any task
	UnassignedBeanCount int
	FeedbackCount       int      // count of unresolved PR feedback items
	FeedbackBeanIDs     []string // bean IDs from unassigned PR feedback

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
	Beans []BeanProgress
}

// BeanProgress holds progress info for a bean.
type BeanProgress struct {
	ID          string
	Status      string
	Title       string
	Description string
	BeanStatus  string // status from beans (todo/completed/etc)
	Priority    string
	IssueType   string
}
