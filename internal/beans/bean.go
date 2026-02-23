package beans

import (
	"time"
)

// Bean is the domain type representing an issue tracked by the beans CLI.
// Unlike the old Bead type, this maps directly to beans CLI/GraphQL JSON output
// with no sqlite dependency.
type Bean struct {
	ID        string
	Slug      string
	Path      string
	Title     string
	Body      string
	Status    string // todo, in-progress, completed, scrapped, draft
	Priority  string // critical, high, normal, low, deferred
	Type      string // task, bug, feature, epic, milestone
	Tags      []string
	ParentID  string
	CreatedAt time.Time
	UpdatedAt time.Time
	Etag      string
	IsEpic    bool // derived from Type == "epic"

	// Relationship IDs (populated from GraphQL/JSON)
	BlockedByIDs []string
	BlockingIDs  []string
}
