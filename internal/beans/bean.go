package beans

import (
	"time"
)

// Bean is the domain type representing an issue tracked by the beans CLI.
// Unlike the old Bean type, this maps directly to beans CLI/GraphQL JSON output
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

// HasTag returns true if the bean has the given tag.
func (b *Bean) HasTag(tag string) bool {
	for _, t := range b.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// HasTagValue checks if a slice of tags contains the given tag.
func HasTagValue(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}
