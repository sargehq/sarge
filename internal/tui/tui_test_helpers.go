package tui

import "github.com/sargehq/sarge/internal/beans"

// testBeanItem creates a beanItem for testing with the given properties.
// deps are the IDs of issues this item depends on (for tree building tests).
// The first dep is used as ParentID if provided, matching how beans models
// parent-child relationships.
func testBeanItem(id, title, status string, priority string, beanType string, deps ...string) beanItem {
	b := &beans.Bean{
		ID:       id,
		Title:    title,
		Status:   status,
		Priority: priority,
		Type:     beanType,
	}

	// First dep is the parent (parent-child relationship)
	if len(deps) > 0 {
		b.ParentID = deps[0]
	}

	// Additional deps beyond the first are blocking dependencies
	var dependencies []beans.Dependency
	if len(deps) > 1 {
		for _, depID := range deps[1:] {
			dependencies = append(dependencies, beans.Dependency{
				BeanID:      id,
				BlockedByID: depID,
			})
		}
	}

	return beanItem{
		BeanWithDeps: &beans.BeanWithDeps{
			Bean:         b,
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

// testBeanItemBlocking creates a beanItem with only blocking dependencies (no parent).
// Use this when testing blocking-only relationships (not parent-child).
// Status is always beans.StatusTodo; pass a different status via testBeanItem if needed.
func testBeanItemBlocking(id, title string, priority string, beanType string, blockedBy ...string) beanItem {
	var dependencies []beans.Dependency
	for _, depID := range blockedBy {
		dependencies = append(dependencies, beans.Dependency{
			BeanID:      id,
			BlockedByID: depID,
		})
	}

	return beanItem{
		BeanWithDeps: &beans.BeanWithDeps{
			Bean: &beans.Bean{
				ID:       id,
				Title:    title,
				Status:   beans.StatusTodo,
				Priority: priority,
				Type:     beanType,
			},
			Dependencies: dependencies,
		},
	}
}
