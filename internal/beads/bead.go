package beads

import (
	"time"
)

// Bead represents an issue from the beads issue tracker.
type Bead struct {
	ID                 string
	Title              string
	Description        string
	Design             string
	AcceptanceCriteria string
	Notes              string
	Status             string
	Priority           int
	Type               string // issue_type in the database
	Assignee           string
	EstimatedMinutes   int
	CreatedAt          time.Time
	CreatedBy          string
	Owner              string
	UpdatedAt          time.Time
	ClosedAt           time.Time
	CloseReason        string
	ExternalRef        string
	IsEpic             bool // derived from issue_type == "epic"
}
