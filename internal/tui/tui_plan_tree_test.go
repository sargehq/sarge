package tui

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/require"
)

// TestBuildBeadTree_EpicHierarchy tests handling of epic (parent-child) relationships
func TestBuildBeadTree_EpicHierarchy(t *testing.T) {
	items := []beadItem{
		testBeadItem("epic-1", "Epic 1", beans.StatusTodo, beans.PriorityHigh, "epic"),
		testBeadItem("task-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task", "epic-1"),
		testBeadItem("task-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task", "epic-1"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// Verify epic is root and tasks are children
	require.Len(t, result, 3)

	require.Equal(t, "epic-1", result[0].ID)
	require.Equal(t, 0, result[0].treeDepth, "expected epic-1 at root level")

	// Both tasks should be at depth 1
	require.Equal(t, 1, result[1].treeDepth, "expected task at depth 1")
	require.Equal(t, 1, result[2].treeDepth, "expected task at depth 1")
}

// TestBuildBeadTree_BlocksDependencies tests handling of "blocks" type dependencies
func TestBuildBeadTree_BlocksDependencies(t *testing.T) {
	items := []beadItem{
		testBeadItem("blocker", "Blocker", beans.StatusTodo, beans.PriorityHigh, "task"),
		testBeadItem("blocked", "Blocked", beans.StatusTodo, beans.PriorityNormal, "task", "blocker"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	require.Len(t, result, 2)

	// Blocker should be root, blocked should be child
	require.Equal(t, "blocker", result[0].ID)
	require.Equal(t, "blocked", result[1].ID)
	require.Equal(t, 1, result[1].treeDepth, "expected blocked at depth 1")
}

// TestBuildBeadTree_ClosedParentVisibility tests filtering of closed parents
func TestBuildBeadTree_ClosedParentVisibility(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeadItem("child", "Child", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// Both parent and child should be visible since parent has visible child
	require.Len(t, result, 2, "expected both parent and child visible")
}

// TestBuildBeadTree_ClosedParentNoVisibleChildren tests filtering out closed parents without visible children
func TestBuildBeadTree_ClosedParentNoVisibleChildren(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// Parent should be filtered out since it has no visible children
	require.Empty(t, result, "expected closed parent without children to be filtered out")
}

// TestBuildBeadTree_MultiLevelNesting tests deep hierarchy
func TestBuildBeadTree_MultiLevelNesting(t *testing.T) {
	items := []beadItem{
		testBeadItem("level-0", "Level 0", beans.StatusTodo, beans.PriorityHigh, "task"),
		testBeadItem("level-1", "Level 1", beans.StatusTodo, beans.PriorityNormal, "task", "level-0"),
		testBeadItem("level-2", "Level 2", beans.StatusTodo, beans.PriorityLow, "task", "level-1"),
		testBeadItem("level-3", "Level 3", beans.StatusTodo, beans.PriorityDeferred, "task", "level-2"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	require.Len(t, result, 4)

	// Verify each level has correct depth
	expectedDepths := []int{0, 1, 2, 3}
	for i, item := range result {
		require.Equal(t, expectedDepths[i], item.treeDepth, "item %s has wrong depth", item.ID)
	}
}

// TestBuildBeadTree_MultipleRoots tests handling of multiple independent trees
func TestBuildBeadTree_MultipleRoots(t *testing.T) {
	items := []beadItem{
		testBeadItem("root-1", "Root 1", beans.StatusTodo, beans.PriorityHigh, "task"),
		testBeadItem("root-2", "Root 2", beans.StatusTodo, beans.PriorityNormal, "task"),
		testBeadItem("child-1", "Child 1", beans.StatusTodo, beans.PriorityLow, "task", "root-1"),
		testBeadItem("child-2", "Child 2", beans.StatusTodo, beans.PriorityDeferred, "task", "root-2"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	require.Len(t, result, 4)

	// Count roots (depth 0)
	rootCount := 0
	for _, item := range result {
		if item.treeDepth == 0 {
			rootCount++
		}
	}

	require.Equal(t, 2, rootCount, "expected 2 roots")
}

// TestBuildBeadTree_MixedTypes tests handling of different dependency types together
func TestBuildBeadTree_MixedTypes(t *testing.T) {
	items := []beadItem{
		testBeadItem("epic", "Epic", beans.StatusTodo, beans.PriorityHigh, "epic"),
		testBeadItem("task", "Task", beans.StatusTodo, beans.PriorityNormal, "task", "epic"),
		testBeadItem("bug", "Bug", beans.StatusTodo, beans.PriorityLow, "bug"),
		testBeadItem("feature", "Feature", beans.StatusTodo, beans.PriorityDeferred, "feature", "bug"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	require.Len(t, result, 4)

	// Verify mixed types are handled correctly
	rootTypes := make(map[string]bool)
	for _, item := range result {
		if item.treeDepth == 0 {
			rootTypes[item.Type] = true
		}
	}

	require.GreaterOrEqual(t, len(rootTypes), 2, "expected multiple types at root level")
}

// TestBuildBeadTree_CircularDependencies tests handling of circular dependency detection
func TestBuildBeadTree_CircularDependencies(t *testing.T) {
	items := []beadItem{
		testBeadItem("item-1", "Item 1", beans.StatusTodo, beans.PriorityHigh, "task", "item-3"),
		testBeadItem("item-2", "Item 2", beans.StatusTodo, beans.PriorityNormal, "task", "item-1"),
		testBeadItem("item-3", "Item 3", beans.StatusTodo, beans.PriorityLow, "task", "item-2"),
	}

	// The function should handle this gracefully without infinite loop
	result := buildBeadTree(context.Background(), items, nil)

	// Should still produce all 3 items
	require.Len(t, result, 3, "expected 3 items despite circular dependency")
}

// TestBuildBeadTree_EmptyInput tests handling of empty input
func TestBuildBeadTree_EmptyInput(t *testing.T) {
	items := []beadItem{}
	result := buildBeadTree(context.Background(), items, nil)

	require.Empty(t, result, "expected empty result for empty input")
}

// TestBuildBeadTree_WithNilClient tests that the function works with nil client
func TestBuildBeadTree_WithNilClient(t *testing.T) {
	// With nil client, function uses dependencies already set on items
	items := []beadItem{
		testBeadItem("item-1", "Item 1", beans.StatusTodo, beans.PriorityHigh, "task"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	require.Len(t, result, 1)
}

// TestBuildBeadTree_ParentChildRelationship tests that parent-child relationships are preserved
func TestBuildBeadTree_ParentChildRelationship(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeadItem("child", "Child", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	// Parents should be fetched and visible when they have visible children
	result := buildBeadTree(context.Background(), items, nil)

	// Both parent and child should be visible
	require.Len(t, result, 2, "expected parent to be visible with open child")
}

// TestBuildBeadTree_ClosedParentWithClosedChildren tests that closed parents with only closed children are filtered out
func TestBuildBeadTree_ClosedParentWithClosedChildren(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeadItemWithOptions("child-1", "Child 1", beans.PriorityNormal, "task", false, "parent"),
		testBeadItemWithOptions("child-2", "Child 2", beans.PriorityNormal, "task", false, "parent"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// All should be filtered out - parent has no open descendants
	require.Empty(t, result, "expected all closed items to be filtered out when no open descendants")
}

// TestBuildBeadTree_ClosedParentWithMixedChildren tests closed parent with both closed and open children
func TestBuildBeadTree_ClosedParentWithMixedChildren(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeadItemWithOptions("child-1", "Child 1", beans.PriorityNormal, "task", false, "parent"),
		testBeadItem("child-2", "Child 2", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// Parent and open child should be visible, closed child should be filtered out
	require.Len(t, result, 2, "expected parent and open child visible")
	ids := make([]string, len(result))
	for i, item := range result {
		ids[i] = item.ID
	}
	require.Contains(t, ids, "parent", "expected parent visible")
	require.Contains(t, ids, "child-2", "expected open child visible")
	require.NotContains(t, ids, "child-1", "expected closed child filtered out")
}

// TestBuildBeadTree_DeepClosedHierarchyWithOpenLeaf tests visibility propagates up through multiple closed levels
func TestBuildBeadTree_DeepClosedHierarchyWithOpenLeaf(t *testing.T) {
	items := []beadItem{
		testBeadItemWithOptions("grandparent", "Grandparent", beans.PriorityHigh, "epic", true),
		testBeadItemWithOptions("parent", "Parent", beans.PriorityNormal, "task", true, "grandparent"),
		testBeadItem("child", "Child", beans.StatusTodo, beans.PriorityLow, "task", "parent"),
	}

	result := buildBeadTree(context.Background(), items, nil)

	// All should be visible because the open leaf makes its ancestors visible
	require.Len(t, result, 3, "expected all visible due to open leaf")
}

// TestBuildBeadTree_PreserveIDsKeepsClosedRootIssue tests that preserveIDs prevents
// the visibility filter from removing a closed root issue
func TestBuildBeadTree_PreserveIDsKeepsClosedRootIssue(t *testing.T) {
	items := []beadItem{
		testBeadItem("root-issue", "Root Issue", beans.StatusCompleted, beans.PriorityHigh, "epic"),
		testBeadItem("child-1", "Child 1", beans.StatusCompleted, beans.PriorityNormal, "task", "root-issue"),
	}

	// Without preserveIDs, both closed items should be filtered out
	result := buildBeadTree(context.Background(), items, nil)
	require.Len(t, result, 0, "expected all filtered out without preserveIDs")

	// With preserveIDs, the root issue should be kept
	result = buildBeadTree(context.Background(), items, nil, "root-issue")
	ids := make([]string, len(result))
	for i, item := range result {
		ids[i] = item.ID
	}
	require.Contains(t, ids, "root-issue", "expected preserved root issue to be visible")
}

// TestBuildBeadTree_PreserveIDsEmptyStringIgnored tests that empty preserveIDs are ignored
func TestBuildBeadTree_PreserveIDsEmptyStringIgnored(t *testing.T) {
	items := []beadItem{
		testBeadItem("closed-item", "Closed", beans.StatusCompleted, beans.PriorityHigh, "task"),
	}

	// Empty string should be ignored, closed item should still be filtered
	result := buildBeadTree(context.Background(), items, nil, "")
	require.Len(t, result, 0, "expected empty preserveID to be ignored")
}
