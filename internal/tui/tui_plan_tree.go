package tui

import (
	"context"
	"sort"

	"github.com/sargehq/sarge/internal/beans"
)

// buildBeadTree takes a flat list of beads and organizes them into a tree
// based on dependency relationships. Returns the items in tree order with
// treeDepth set for each item.
// Optional preserveIDs specifies bead IDs that should always be kept in results
// regardless of the closed-item visibility filter (e.g., root issues).
func buildBeadTree(ctx context.Context, items []beadItem, client *beans.Client, preserveIDs ...string) []beadItem {
	if len(items) == 0 {
		return items
	}

	// Build a map of ID -> beadItem for quick lookup
	itemMap := make(map[string]*beadItem)
	for i := range items {
		itemMap[items[i].ID] = &items[i]
	}

	// Collect all issue IDs (used in getBlockingDepIDs closure below)
	_ = len(items) // issueIDs would be used for dependency lookups if needed

	// Helper to extract blocking dependency IDs from a beadItem
	getBlockingDepIDs := func(item *beadItem) []string {
		if item.BeanWithDeps == nil {
			return nil
		}
		depIDs := make([]string, 0, len(item.Dependencies))
		for _, dep := range item.Dependencies {
			if dep.BlockedByID != "" {
				depIDs = append(depIDs, dep.BlockedByID)
			}
		}
		return depIDs
	}

	// Identify and fetch missing parent beads (dependencies not in our item list)
	// to preserve tree structure. Loop until no more missing parents are found
	// to handle multiple levels of closed ancestors.
	if client != nil {
		fetchedParents := make(map[string]bool)
		for {
			missingParentIDs := make([]string, 0)
			for i := range items {
				for _, depID := range getBlockingDepIDs(&items[i]) {
					if _, exists := itemMap[depID]; !exists && !fetchedParents[depID] {
						missingParentIDs = append(missingParentIDs, depID)
						fetchedParents[depID] = true
					}
				}
			}

			if len(missingParentIDs) == 0 {
				break
			}

			// Fetch missing parents in a single query
			parentResult, err := client.GetBeansWithDeps(ctx, missingParentIDs)
			if err == nil {
				// Add missing parents to items
				for _, parentID := range missingParentIDs {
					if beadWithDeps := parentResult.GetBean(parentID); beadWithDeps != nil {
						parentBead := &beadItem{
							BeanWithDeps:   beadWithDeps,
							isClosedParent: true,
						}
						items = append(items, *parentBead)
						itemMap[parentBead.ID] = &items[len(items)-1]
					}
				}
			} else {
				break
			}
		}

		// Rebuild itemMap to fix stale pointers.
		itemMap = make(map[string]*beadItem)
		for i := range items {
			itemMap[items[i].ID] = &items[i]
		}
	}

	// Build parent -> children map (issues that block -> issues they block)
	// If A blocks B, then B depends on A, so A is parent, B is child
	childrenMap := make(map[string][]string)
	for i := range items {
		for _, depID := range getBlockingDepIDs(&items[i]) {
			// This item depends on depID, so depID is the parent
			childrenMap[depID] = append(childrenMap[depID], items[i].ID)
		}
	}

	// Store children in each item
	for i := range items {
		items[i].children = childrenMap[items[i].ID]
	}

	// Find root nodes (items with no visible dependencies within our set)
	// A bead is a root if it has no dependencies, OR if none of its dependencies
	// are in our visible set (e.g., all dependencies were deleted or unavailable)
	roots := []string{}
	for i := range items {
		hasVisibleDep := false
		for _, depID := range getBlockingDepIDs(&items[i]) {
			if _, exists := itemMap[depID]; exists {
				hasVisibleDep = true
				break
			}
		}
		if !hasVisibleDep {
			roots = append(roots, items[i].ID)
		}
	}

	// Sort roots: closed parents first (so their open children appear under them),
	// then by priority, then by ID
	sort.Slice(roots, func(i, j int) bool {
		a, b := itemMap[roots[i]], itemMap[roots[j]]
		// Closed parents come first
		if a.isClosedParent != b.isClosedParent {
			return a.isClosedParent
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return compareIDsNatural(a.ID, b.ID)
	})

	// DFS to build tree order
	var result []beadItem
	visited := make(map[string]bool)

	// ancestorPattern tracks the prefix pattern for ancestor continuation lines.
	// Each character represents one depth level:
	// - "│" means the ancestor at that level has more siblings (needs continuation line)
	// - " " means the ancestor at that level is the last child (no continuation needed)
	var visit func(id string, depth int, ancestorPattern string, isLast bool)
	visit = func(id string, depth int, ancestorPattern string, isLast bool) {
		if visited[id] {
			return
		}
		visited[id] = true

		item, ok := itemMap[id]
		if !ok {
			return
		}

		item.treeDepth = depth
		item.isLastChild = isLast

		// Build the tree prefix pattern for this item
		if depth > 0 {
			// Start with ancestor continuation pattern (each character becomes "│ " or "  ")
			var prefix string
			for _, c := range ancestorPattern {
				if c == '│' {
					prefix += "│ "
				} else {
					prefix += "  "
				}
			}
			// Add the connector for this item
			if isLast {
				prefix += "└─"
			} else {
				prefix += "├─"
			}
			item.treePrefixPattern = prefix
		}

		result = append(result, *item)

		// Sort children by priority
		childIDs := childrenMap[id]
		sort.Slice(childIDs, func(i, j int) bool {
			a, b := itemMap[childIDs[i]], itemMap[childIDs[j]]
			if a == nil || b == nil {
				return compareIDsNatural(childIDs[i], childIDs[j])
			}
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return compareIDsNatural(a.ID, b.ID)
		})

		// Compute the ancestor pattern for children
		// If this item is the last child, its continuation is " " (no vertical line)
		// Otherwise, it's "│" (vertical line for siblings below)
		var childAncestorPattern string
		if depth == 0 {
			// Root nodes don't add to ancestor pattern
			childAncestorPattern = ancestorPattern
		} else if isLast {
			childAncestorPattern = ancestorPattern + " "
		} else {
			childAncestorPattern = ancestorPattern + "│"
		}

		for idx, childID := range childIDs {
			isLastChild := idx == len(childIDs)-1
			visit(childID, depth+1, childAncestorPattern, isLastChild)
		}
	}

	// Visit all roots
	for idx, rootID := range roots {
		isLastRoot := idx == len(roots)-1
		visit(rootID, 0, "", isLastRoot)
	}

	// Add any orphaned items (not reachable from roots)
	for i := range items {
		if !visited[items[i].ID] {
			items[i].treeDepth = 0
			result = append(result, items[i])
		}
	}

	// Filter out closed items that have no open descendants.
	// Build a map of ID -> item for quick lookup
	resultMap := make(map[string]*beadItem)
	for i := range result {
		resultMap[result[i].ID] = &result[i]
	}

	// Build set of preserved IDs that should always be visible
	preserveSet := make(map[string]bool, len(preserveIDs))
	for _, id := range preserveIDs {
		if id != "" {
			preserveSet[id] = true
		}
	}

	// Compute visibility recursively: an item is visible if:
	// 1. It's not closed, OR
	// 2. It's closed but has at least one visible descendant, OR
	// 3. It's in the preserveSet (e.g., root issue that should always be shown)
	// We use memoization to avoid recomputing visibility for the same item.
	visibilityCache := make(map[string]bool)
	var isVisible func(id string) bool
	isVisible = func(id string) bool {
		// Check cache first
		if cached, ok := visibilityCache[id]; ok {
			return cached
		}

		item, exists := resultMap[id]
		if !exists {
			visibilityCache[id] = false
			return false
		}

		// Preserved items are always visible (e.g., root issue)
		if preserveSet[id] {
			visibilityCache[id] = true
			return true
		}

		// Non-closed items are always visible
		if item.Status != beans.StatusCompleted {
			visibilityCache[id] = true
			return true
		}

		// For closed items, check if any child is visible (recursively)
		if children, ok := childrenMap[id]; ok {
			for _, childID := range children {
				if isVisible(childID) {
					visibilityCache[id] = true
					return true
				}
			}
		}

		visibilityCache[id] = false
		return false
	}

	// Filter based on computed visibility
	var filtered []beadItem
	for _, item := range result {
		if isVisible(item.ID) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}
