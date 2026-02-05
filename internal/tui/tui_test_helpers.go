package tui

import "github.com/sargehq/sarge/internal/beads"

// testBeadItem creates a beadItem for testing with the given properties.
// deps are the IDs of issues this item depends on (for tree building tests).
func testBeadItem(id, title, status string, priority int, beadType string, deps ...string) beadItem {
	// Build Dependencies slice from dep IDs
	dependencies := make([]beads.Dependency, len(deps))
	for i, depID := range deps {
		dependencies[i] = beads.Dependency{
			IssueID:     id,
			DependsOnID: depID,
			Type:        "blocks", // Default to blocks for tree building
		}
	}

	return beadItem{
		BeadWithDeps: &beads.BeadWithDeps{
			Bead: &beads.Bead{
				ID:       id,
				Title:    title,
				Status:   status,
				Priority: priority,
				Type:     beadType,
			},
			Dependencies: dependencies,
		},
	}
}

// testBeadItemWithOptions creates a beadItem for testing with additional options.
// This is used specifically for creating closed items with the isClosedParent flag.
func testBeadItemWithOptions(id, title string, priority int, beadType string, isClosedParent bool, deps ...string) beadItem {
	item := testBeadItem(id, title, "closed", priority, beadType, deps...)
	item.isClosedParent = isClosedParent
	return item
}
