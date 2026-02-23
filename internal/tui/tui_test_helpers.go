package tui

import "github.com/sargehq/sarge/internal/beans"

// testBeadItem creates a beadItem for testing with the given properties.
// deps are the IDs of issues this item depends on (for tree building tests).
func testBeadItem(id, title, status string, priority string, beadType string, deps ...string) beadItem {
	// Build Dependencies slice from dep IDs
	dependencies := make([]beans.Dependency, len(deps))
	for i, depID := range deps {
		dependencies[i] = beans.Dependency{
			BeanID:      id,
			BlockedByID: depID,
		}
	}

	return beadItem{
		BeanWithDeps: &beans.BeanWithDeps{
			Bean: &beans.Bean{
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
func testBeadItemWithOptions(id, title string, priority string, beadType string, isClosedParent bool, deps ...string) beadItem {
	item := testBeadItem(id, title, beans.StatusCompleted, priority, beadType, deps...)
	item.isClosedParent = isClosedParent
	return item
}
