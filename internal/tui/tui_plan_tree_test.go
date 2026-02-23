package tui

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/require"
)

// TestBuildBeanTree_EpicHierarchy tests handling of epic (parent-child) relationships
func TestBuildBeanTree_EpicHierarchy(t *testing.T) {
	items := []beanItem{
		testBeanItem("epic-1", "Epic 1", beans.StatusTodo, beans.PriorityHigh, "epic"),
		testBeanItem("task-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task", "epic-1"),
		testBeanItem("task-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task", "epic-1"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	// Verify epic is root and tasks are children
	require.Len(t, result, 3)

	require.Equal(t, "epic-1", result[0].ID)
	require.Equal(t, 0, result[0].treeDepth, "expected epic-1 at root level")

	// Both tasks should be at depth 1
	require.Equal(t, 1, result[1].treeDepth, "expected task at depth 1")
	require.Equal(t, 1, result[2].treeDepth, "expected task at depth 1")
}

// TestBuildBeanTree_BlocksDependencies tests handling of "blocks" type dependencies
func TestBuildBeanTree_BlocksDependencies(t *testing.T) {
	items := []beanItem{
		testBeanItemBlocking("blocker", "Blocker", beans.PriorityHigh, "task"),
		testBeanItemBlocking("blocked", "Blocked", beans.PriorityNormal, "task", "blocker"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	require.Len(t, result, 2)

	// Blocker should be root, blocked should be child
	require.Equal(t, "blocker", result[0].ID)
	require.Equal(t, "blocked", result[1].ID)
	require.Equal(t, 1, result[1].treeDepth, "expected blocked at depth 1")
}

// TestBuildBeanTree_ClosedParentVisibility tests filtering of closed parents
func TestBuildBeanTree_ClosedParentVisibility(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeanItem("child", "Child", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	// Both parent and child should be visible since parent has visible child
	require.Len(t, result, 2, "expected both parent and child visible")
}

// TestBuildBeanTree_ClosedParentNoVisibleChildren tests filtering out closed parents without visible children
func TestBuildBeanTree_ClosedParentNoVisibleChildren(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
	}

	result := buildBeanTree(context.Background(), items, nil)

	// Parent should be filtered out since it has no visible children
	require.Empty(t, result, "expected closed parent without children to be filtered out")
}

// TestBuildBeanTree_MultiLevelNesting tests deep hierarchy
func TestBuildBeanTree_MultiLevelNesting(t *testing.T) {
	items := []beanItem{
		testBeanItem("level-0", "Level 0", beans.StatusTodo, beans.PriorityHigh, "task"),
		testBeanItem("level-1", "Level 1", beans.StatusTodo, beans.PriorityNormal, "task", "level-0"),
		testBeanItem("level-2", "Level 2", beans.StatusTodo, beans.PriorityLow, "task", "level-1"),
		testBeanItem("level-3", "Level 3", beans.StatusTodo, beans.PriorityDeferred, "task", "level-2"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	require.Len(t, result, 4)

	// Verify each level has correct depth
	expectedDepths := []int{0, 1, 2, 3}
	for i, item := range result {
		require.Equal(t, expectedDepths[i], item.treeDepth, "item %s has wrong depth", item.ID)
	}
}

// TestBuildBeanTree_MultipleRoots tests handling of multiple independent trees
func TestBuildBeanTree_MultipleRoots(t *testing.T) {
	items := []beanItem{
		testBeanItem("root-1", "Root 1", beans.StatusTodo, beans.PriorityHigh, "task"),
		testBeanItem("root-2", "Root 2", beans.StatusTodo, beans.PriorityNormal, "task"),
		testBeanItem("child-1", "Child 1", beans.StatusTodo, beans.PriorityLow, "task", "root-1"),
		testBeanItem("child-2", "Child 2", beans.StatusTodo, beans.PriorityDeferred, "task", "root-2"),
	}

	result := buildBeanTree(context.Background(), items, nil)

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

// TestBuildBeanTree_MixedTypes tests handling of different dependency types together
func TestBuildBeanTree_MixedTypes(t *testing.T) {
	items := []beanItem{
		testBeanItem("epic", "Epic", beans.StatusTodo, beans.PriorityHigh, "epic"),
		testBeanItem("task", "Task", beans.StatusTodo, beans.PriorityNormal, "task", "epic"),
		testBeanItem("bug", "Bug", beans.StatusTodo, beans.PriorityLow, "bug"),
		testBeanItem("feature", "Feature", beans.StatusTodo, beans.PriorityDeferred, "feature", "bug"),
	}

	result := buildBeanTree(context.Background(), items, nil)

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

// TestBuildBeanTree_CircularDependencies tests handling of circular dependency detection
func TestBuildBeanTree_CircularDependencies(t *testing.T) {
	// Use blocking deps for circular dependency (parent-child can't be circular)
	items := []beanItem{
		testBeanItemBlocking("item-1", "Item 1", beans.PriorityHigh, "task", "item-3"),
		testBeanItemBlocking("item-2", "Item 2", beans.PriorityNormal, "task", "item-1"),
		testBeanItemBlocking("item-3", "Item 3", beans.PriorityLow, "task", "item-2"),
	}

	// The function should handle this gracefully without infinite loop
	result := buildBeanTree(context.Background(), items, nil)

	// Should still produce all 3 items
	require.Len(t, result, 3, "expected 3 items despite circular dependency")
}

// TestBuildBeanTree_EmptyInput tests handling of empty input
func TestBuildBeanTree_EmptyInput(t *testing.T) {
	items := []beanItem{}
	result := buildBeanTree(context.Background(), items, nil)

	require.Empty(t, result, "expected empty result for empty input")
}

// TestBuildBeanTree_WithNilClient tests that the function works with nil client
func TestBuildBeanTree_WithNilClient(t *testing.T) {
	// With nil client, function uses dependencies already set on items
	items := []beanItem{
		testBeanItem("item-1", "Item 1", beans.StatusTodo, beans.PriorityHigh, "task"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	require.Len(t, result, 1)
}

// TestBuildBeanTree_ParentChildRelationship tests that parent-child relationships are preserved
func TestBuildBeanTree_ParentChildRelationship(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeanItem("child", "Child", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	// Parents should be fetched and visible when they have visible children
	result := buildBeanTree(context.Background(), items, nil)

	// Both parent and child should be visible
	require.Len(t, result, 2, "expected parent to be visible with open child")
}

// TestBuildBeanTree_ClosedParentWithClosedChildren tests that closed parents with only closed children are filtered out
func TestBuildBeanTree_ClosedParentWithClosedChildren(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeanItemWithOptions("child-1", "Child 1", beans.PriorityNormal, "task", false, "parent"),
		testBeanItemWithOptions("child-2", "Child 2", beans.PriorityNormal, "task", false, "parent"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	// All should be filtered out - parent has no open descendants
	require.Empty(t, result, "expected all closed items to be filtered out when no open descendants")
}

// TestBuildBeanTree_ClosedParentWithMixedChildren tests closed parent with both closed and open children
func TestBuildBeanTree_ClosedParentWithMixedChildren(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("parent", "Parent", beans.PriorityHigh, "epic", true),
		testBeanItemWithOptions("child-1", "Child 1", beans.PriorityNormal, "task", false, "parent"),
		testBeanItem("child-2", "Child 2", beans.StatusTodo, beans.PriorityNormal, "task", "parent"),
	}

	result := buildBeanTree(context.Background(), items, nil)

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

// TestBuildBeanTree_DeepClosedHierarchyWithOpenLeaf tests visibility propagates up through multiple closed levels
func TestBuildBeanTree_DeepClosedHierarchyWithOpenLeaf(t *testing.T) {
	items := []beanItem{
		testBeanItemWithOptions("grandparent", "Grandparent", beans.PriorityHigh, "epic", true),
		testBeanItemWithOptions("parent", "Parent", beans.PriorityNormal, "task", true, "grandparent"),
		testBeanItem("child", "Child", beans.StatusTodo, beans.PriorityLow, "task", "parent"),
	}

	result := buildBeanTree(context.Background(), items, nil)

	// All should be visible because the open leaf makes its ancestors visible
	require.Len(t, result, 3, "expected all visible due to open leaf")
}

// TestBuildBeanTree_PreserveIDsKeepsClosedRootIssue tests that preserveIDs prevents
// the visibility filter from removing a closed root issue
func TestBuildBeanTree_PreserveIDsKeepsClosedRootIssue(t *testing.T) {
	items := []beanItem{
		testBeanItem("root-issue", "Root Issue", beans.StatusCompleted, beans.PriorityHigh, "epic"),
		testBeanItem("child-1", "Child 1", beans.StatusCompleted, beans.PriorityNormal, "task", "root-issue"),
	}

	// Without preserveIDs, both closed items should be filtered out
	result := buildBeanTree(context.Background(), items, nil)
	require.Len(t, result, 0, "expected all filtered out without preserveIDs")

	// With preserveIDs, the root issue should be kept
	result = buildBeanTree(context.Background(), items, nil, "root-issue")
	ids := make([]string, len(result))
	for i, item := range result {
		ids[i] = item.ID
	}
	require.Contains(t, ids, "root-issue", "expected preserved root issue to be visible")
}

// TestBuildBeanTree_PreserveIDsEmptyStringIgnored tests that empty preserveIDs are ignored
func TestBuildBeanTree_PreserveIDsEmptyStringIgnored(t *testing.T) {
	items := []beanItem{
		testBeanItem("closed-item", "Closed", beans.StatusCompleted, beans.PriorityHigh, "task"),
	}

	// Empty string should be ignored, closed item should still be filtered
	result := buildBeanTree(context.Background(), items, nil, "")
	require.Len(t, result, 0, "expected empty preserveID to be ignored")
}
