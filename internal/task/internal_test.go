package task

import (
	"testing"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanAddToTask(t *testing.T) {
	graph := &DependencyGraph{
		DependsOn: map[string][]string{
			"b": {"a"},
		},
		Dependents: map[string][]string{
			"a": {"b"},
		},
	}

	assigned := map[string]int{
		"a": 0, // a is in task 0
	}

	// b depends on a which is in task 0, so b can be added to task 0 or later
	assert.True(t, canAddToTask("b", 0, assigned, graph), "b should be addable to task 0 (same as dependency)")
	assert.True(t, canAddToTask("b", 1, assigned, graph), "b should be addable to task 1 (after dependency)")

	// Can't add b to task before a
	assigned["a"] = 1
	assert.False(t, canAddToTask("b", 0, assigned, graph), "b should not be addable to task 0 (before dependency in task 1)")
}

// ComputeInterTaskDeps is a helper function that mimics the inter-task dependency
// computation logic from handlePostEstimation. Used for testing.
func ComputeInterTaskDeps(tasks []Task, dependencies map[string][]beads.Dependency) map[int][]int {
	// Build beadID → task index mapping
	beadToTask := make(map[string]int)
	for i, t := range tasks {
		for _, beadID := range t.BeadIDs {
			beadToTask[beadID] = i
		}
	}

	// Compute inter-task dependencies
	// Returns map of taskIdx → list of task indices it depends on
	interTaskDeps := make(map[int]map[int]bool)
	for beadID, deps := range dependencies {
		taskIdx, ok := beadToTask[beadID]
		if !ok {
			continue
		}
		for _, dep := range deps {
			depTaskIdx, ok := beadToTask[dep.DependsOnID]
			if !ok {
				continue
			}
			if taskIdx == depTaskIdx {
				continue // same task, no inter-task dependency
			}
			if interTaskDeps[taskIdx] == nil {
				interTaskDeps[taskIdx] = make(map[int]bool)
			}
			interTaskDeps[taskIdx][depTaskIdx] = true
		}
	}

	// Convert to slice representation
	result := make(map[int][]int)
	for taskIdx, depSet := range interTaskDeps {
		for depIdx := range depSet {
			result[taskIdx] = append(result[taskIdx], depIdx)
		}
	}
	return result
}

func TestComputeInterTaskDepsChain(t *testing.T) {
	// Chain: c depends on b, b depends on a - each in separate task
	tasks := []Task{
		{ID: "task-1", BeadIDs: []string{"a"}},
		{ID: "task-2", BeadIDs: []string{"b"}},
		{ID: "task-3", BeadIDs: []string{"c"}},
	}

	dependencies := map[string][]beads.Dependency{
		"b": {{IssueID: "b", DependsOnID: "a", Type: "blocks"}},
		"c": {{IssueID: "c", DependsOnID: "b", Type: "blocks"}},
	}

	interDeps := ComputeInterTaskDeps(tasks, dependencies)

	// task-2 (index 1) should depend on task-1 (index 0)
	require.Contains(t, interDeps, 1, "task-2 should have dependencies")
	assert.Contains(t, interDeps[1], 0, "task-2 should depend on task-1")

	// task-3 (index 2) should depend on task-2 (index 1)
	require.Contains(t, interDeps, 2, "task-3 should have dependencies")
	assert.Contains(t, interDeps[2], 1, "task-3 should depend on task-2")

	// task-1 (index 0) should have no dependencies
	assert.NotContains(t, interDeps, 0, "task-1 should have no inter-task dependencies")
}

func TestComputeInterTaskDepsDiamond(t *testing.T) {
	// Diamond: a and b independent, c depends on both, d depends on c
	tasks := []Task{
		{ID: "task-1", BeadIDs: []string{"a"}},
		{ID: "task-2", BeadIDs: []string{"b"}},
		{ID: "task-3", BeadIDs: []string{"c"}},
		{ID: "task-4", BeadIDs: []string{"d"}},
	}

	dependencies := map[string][]beads.Dependency{
		"c": {
			{IssueID: "c", DependsOnID: "a", Type: "blocks"},
			{IssueID: "c", DependsOnID: "b", Type: "blocks"},
		},
		"d": {{IssueID: "d", DependsOnID: "c", Type: "blocks"}},
	}

	interDeps := ComputeInterTaskDeps(tasks, dependencies)

	// task-3 (index 2) should depend on both task-1 (index 0) and task-2 (index 1)
	require.Contains(t, interDeps, 2, "task-3 should have dependencies")
	assert.Contains(t, interDeps[2], 0, "task-3 should depend on task-1")
	assert.Contains(t, interDeps[2], 1, "task-3 should depend on task-2")

	// task-4 (index 3) should depend on task-3 (index 2)
	require.Contains(t, interDeps, 3, "task-4 should have dependencies")
	assert.Contains(t, interDeps[3], 2, "task-4 should depend on task-3")

	// task-1 and task-2 should have no dependencies
	assert.NotContains(t, interDeps, 0, "task-1 should have no inter-task dependencies")
	assert.NotContains(t, interDeps, 1, "task-2 should have no inter-task dependencies")
}

func TestComputeInterTaskDepsSameTaskNoDeps(t *testing.T) {
	// Both beads in same task, b depends on a
	tasks := []Task{
		{ID: "task-1", BeadIDs: []string{"a", "b"}},
	}

	dependencies := map[string][]beads.Dependency{
		"b": {{IssueID: "b", DependsOnID: "a", Type: "blocks"}},
	}

	interDeps := ComputeInterTaskDeps(tasks, dependencies)

	// No inter-task dependencies since both beads are in the same task
	assert.Empty(t, interDeps, "same-task dependencies should not create inter-task deps")
}
