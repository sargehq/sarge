package task_test

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDependencyGraph(t *testing.T) {
	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}

	dependencies := map[string][]beans.Dependency{
		"b": {{BeanID: "b", BlockedByID: "a"}},
		"c": {{BeanID: "c", BlockedByID: "b"}},
	}

	graph := task.BuildDependencyGraph(beadList, dependencies)

	// b depends on a
	require.Len(t, graph.DependsOn["b"], 1, "expected b to have 1 dependency")
	assert.Equal(t, "a", graph.DependsOn["b"][0], "expected b to depend on a")

	// c depends on b
	require.Len(t, graph.DependsOn["c"], 1, "expected c to have 1 dependency")
	assert.Equal(t, "b", graph.DependsOn["c"][0], "expected c to depend on b")

	// a blocks b (now called Dependents)
	require.Len(t, graph.Dependents["a"], 1, "expected a to have 1 dependent")
	assert.Equal(t, "b", graph.Dependents["a"][0], "expected a to have dependent b")
}

func TestBuildDependencyGraphIgnoresExternalDeps(t *testing.T) {
	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
	}

	// Dependencies on external beads (not in beads list) should be ignored
	dependencies := map[string][]beans.Dependency{
		"a": {{BeanID: "a", BlockedByID: "external"}},
	}

	graph := task.BuildDependencyGraph(beadList, dependencies)

	// External dependency should be filtered out since "external" is not in the beads list
	assert.Empty(t, graph.DependsOn["a"], "external dependency should be ignored")
	assert.Empty(t, graph.Dependents["external"], "external should not have dependents since it's not in beads")
}

func TestTopologicalSort(t *testing.T) {
	beadList := []beans.Bean{
		{ID: "c", Title: "C"},
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}

	dependencies := map[string][]beans.Dependency{
		"c": {{BeanID: "c", BlockedByID: "b"}},
		"b": {{BeanID: "b", BlockedByID: "a"}},
	}

	graph := task.BuildDependencyGraph(beadList, dependencies)
	sorted, err := task.TopologicalSort(graph, beadList)
	require.NoError(t, err, "TopologicalSort failed")

	// a should come before b, b should come before c
	positions := make(map[string]int)
	for i, bead := range sorted {
		positions[bead.ID] = i
	}

	assert.Less(t, positions["a"], positions["b"], "a should come before b")
	assert.Less(t, positions["b"], positions["c"], "b should come before c")
}

func TestTopologicalSortDetectsCycle(t *testing.T) {
	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}

	dependencies := map[string][]beans.Dependency{
		"a": {{BeanID: "a", BlockedByID: "b"}},
		"b": {{BeanID: "b", BlockedByID: "a"}},
	}

	graph := task.BuildDependencyGraph(beadList, dependencies)
	_, err := task.TopologicalSort(graph, beadList)
	assert.Error(t, err, "expected error for cycle detection")
}

func TestPlanSimple(t *testing.T) {
	ctx := context.Background()
	scores := map[string]int{"a": 3, "b": 3, "c": 3}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}

	dependencies := map[string][]beans.Dependency{}

	// Token budget of 10000 should fit all beads (3000+3000+3000=9000 tokens)
	tasks, err := planner.Plan(ctx, beadList, dependencies, 10000)
	require.NoError(t, err, "Plan failed")

	assert.Len(t, tasks, 1, "expected 1 task")
	assert.Len(t, tasks[0].BeanIDs, 3, "expected 3 beads in task")
}

func TestPlanSplitByBudget(t *testing.T) {
	ctx := context.Background()
	scores := map[string]int{"a": 5, "b": 5, "c": 5}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}

	dependencies := map[string][]beans.Dependency{}

	// Token budget of 7000 should split into multiple tasks (each bead is 5000 tokens)
	tasks, err := planner.Plan(ctx, beadList, dependencies, 7000)
	require.NoError(t, err, "Plan failed")

	assert.GreaterOrEqual(t, len(tasks), 2, "expected at least 2 tasks")

	// Verify all beads are assigned
	totalBeads := 0
	for _, t := range tasks {
		totalBeads += len(t.BeanIDs)
	}
	assert.Equal(t, 3, totalBeads, "expected 3 total beads")
}

func TestPlanRespectsDependencies(t *testing.T) {
	ctx := context.Background()
	scores := map[string]int{"a": 3, "b": 3}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}

	dependencies := map[string][]beans.Dependency{
		"b": {{BeanID: "b", BlockedByID: "a"}},
	}

	// Small token budget to force multiple tasks (each bead is 3000 tokens)
	tasks, err := planner.Plan(ctx, beadList, dependencies, 4000)
	require.NoError(t, err, "Plan failed")

	// Find which tasks contain a and b
	taskForBead := make(map[string]int)
	for i, t := range tasks {
		for _, id := range t.BeanIDs {
			taskForBead[id] = i
		}
	}

	// b depends on a, so a's task index must be <= b's task index
	assert.LessOrEqual(t, taskForBead["a"], taskForBead["b"], "dependency violated: a should be in same or earlier task than b")
}

func TestPlanEmpty(t *testing.T) {
	ctx := context.Background()
	estimator := &task.ComplexityEstimatorMock{}
	planner := task.NewDefaultPlanner(estimator)

	dependencies := map[string][]beans.Dependency{}

	tasks, err := planner.Plan(ctx, nil, dependencies, 10000)
	require.NoError(t, err, "Plan failed")

	assert.Empty(t, tasks, "expected no tasks for empty input")
}

func TestPlanFirstFitDecreasing(t *testing.T) {
	ctx := context.Background()
	// Larger beads are assigned first (by token estimate)
	scores := map[string]int{"small": 2, "medium": 4, "large": 6}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "small", Title: "Small"},
		{ID: "medium", Title: "Medium"},
		{ID: "large", Title: "Large"},
	}

	dependencies := map[string][]beans.Dependency{}

	// Token budget of 10000: large (6000) goes first, then small (2000) fits, medium (4000) won't fit
	// So we expect: task1=[large, small], task2=[medium]
	tasks, err := planner.Plan(ctx, beadList, dependencies, 10000)
	require.NoError(t, err, "Plan failed")

	assert.Len(t, tasks, 2, "expected 2 tasks")
}

func TestPlanChainDependencySplitAcrossTasks(t *testing.T) {
	ctx := context.Background()
	// Small budget to force each bead into separate task
	scores := map[string]int{"a": 5, "b": 5, "c": 5}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
	}

	// Chain: c depends on b, b depends on a
	dependencies := map[string][]beans.Dependency{
		"b": {{BeanID: "b", BlockedByID: "a"}},
		"c": {{BeanID: "c", BlockedByID: "b"}},
	}

	// Budget of 6000 allows one bead per task (each is 5000 tokens)
	tasks, err := planner.Plan(ctx, beadList, dependencies, 6000)
	require.NoError(t, err, "Plan failed")
	require.Len(t, tasks, 3, "expected 3 tasks for 3 beads with chain dependency")

	// Find which task contains each bead
	taskForBead := make(map[string]int)
	for i, t := range tasks {
		for _, id := range t.BeanIDs {
			taskForBead[id] = i
		}
	}

	// Verify chain ordering: task(a) < task(b) < task(c)
	assert.Less(t, taskForBead["a"], taskForBead["b"], "a should be in earlier task than b")
	assert.Less(t, taskForBead["b"], taskForBead["c"], "b should be in earlier task than c")
}

func TestPlanDiamondDependencySplitAcrossTasks(t *testing.T) {
	ctx := context.Background()
	scores := map[string]int{"a": 5, "b": 5, "c": 5, "d": 5}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	// Diamond: a and b are independent, c depends on both, d depends on c
	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
		{ID: "c", Title: "C"},
		{ID: "d", Title: "D"},
	}

	dependencies := map[string][]beans.Dependency{
		"c": {
			{BeanID: "c", BlockedByID: "a"},
			{BeanID: "c", BlockedByID: "b"},
		},
		"d": {{BeanID: "d", BlockedByID: "c"}},
	}

	// Budget of 6000 allows one bead per task
	tasks, err := planner.Plan(ctx, beadList, dependencies, 6000)
	require.NoError(t, err, "Plan failed")
	require.Len(t, tasks, 4, "expected 4 tasks for 4 beads with diamond dependency")

	// Find which task contains each bead
	taskForBead := make(map[string]int)
	for i, t := range tasks {
		for _, id := range t.BeanIDs {
			taskForBead[id] = i
		}
	}

	// c depends on both a and b, so task(c) > task(a) and task(c) > task(b)
	assert.Less(t, taskForBead["a"], taskForBead["c"], "a should be in earlier task than c")
	assert.Less(t, taskForBead["b"], taskForBead["c"], "b should be in earlier task than c")
	// d depends on c
	assert.Less(t, taskForBead["c"], taskForBead["d"], "c should be in earlier task than d")
}

func TestPlanSameTaskDependenciesNoSelfDep(t *testing.T) {
	ctx := context.Background()
	scores := map[string]int{"a": 2, "b": 2}
	estimator := &task.ComplexityEstimatorMock{
		EstimateFunc: func(ctx context.Context, bead beans.Bean) (int, int, error) {
			score := scores[bead.ID]
			if score == 0 {
				score = 5 // default
			}
			return score, score * 1000, nil
		},
	}
	planner := task.NewDefaultPlanner(estimator)

	beadList := []beans.Bean{
		{ID: "a", Title: "A"},
		{ID: "b", Title: "B"},
	}

	// b depends on a
	dependencies := map[string][]beans.Dependency{
		"b": {{BeanID: "b", BlockedByID: "a"}},
	}

	// Large budget to fit both beads in same task (each is 2000 tokens)
	tasks, err := planner.Plan(ctx, beadList, dependencies, 10000)
	require.NoError(t, err, "Plan failed")
	require.Len(t, tasks, 1, "expected 1 task with both beads")

	// Both beads should be in the same task
	taskForBead := make(map[string]int)
	for i, t := range tasks {
		for _, id := range t.BeanIDs {
			taskForBead[id] = i
		}
	}

	assert.Equal(t, taskForBead["a"], taskForBead["b"], "a and b should be in the same task")
}
