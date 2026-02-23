package tui

import "github.com/sargehq/sarge/internal/beans"

// testBeanItem creates a beanItem for testing with the given properties.
// deps are the IDs of issues this item depends on (for tree building tests).
func testBeanItem(id, title, status string, priority string, beanType string, deps ...string) beanItem {
	// Build Dependencies slice from dep IDs
	dependencies := make([]beans.Dependency, len(deps))
	for i, depID := range deps {
		dependencies[i] = beans.Dependency{
			BeanID:      id,
			BlockedByID: depID,
		}
	}

	return beanItem{
		BeanWithDeps: &beans.BeanWithDeps{
			Bean: &beans.Bean{
				ID:       id,
				Title:    title,
				Status:   status,
				Priority: priority,
				Type:     beanType,
			},
			Dependencies: dependencies,
		},
	}
}

// testBeanItemWithOptions creates a beanItem for testing with additional options.
// This is used specifically for creating closed items with the isClosedParent flag.
func testBeanItemWithOptions(id, title string, priority string, beanType string, isClosedParent bool, deps ...string) beanItem {
	item := testBeanItem(id, title, beans.StatusCompleted, priority, beanType, deps...)
	item.isClosedParent = isClosedParent
	return item
}
