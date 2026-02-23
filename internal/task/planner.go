package task

import (
	"context"
	"fmt"

	"github.com/sargehq/sarge/internal/beads"
)

// DefaultPlanner implements the Planner interface using bin-packing.
type DefaultPlanner struct {
	estimator ComplexityEstimator
}

// NewDefaultPlanner creates a new planner with the given complexity estimator.
func NewDefaultPlanner(estimator ComplexityEstimator) *DefaultPlanner {
	return &DefaultPlanner{estimator: estimator}
}

// beadEstimate holds both complexity score and token estimate for a bead.
type beadEstimate struct {
	score  int
	tokens int
}

// Plan creates task assignments from beads using bin-packing algorithm.
// The budget represents the target tokens per task (e.g., 120000 for 120K tokens).
func (p *DefaultPlanner) Plan(
	ctx context.Context,
	beadList []beads.Bead,
	dependencies map[string][]beads.Dependency,
	budget int,
) ([]Task, error) {
	if len(beadList) == 0 {
		return nil, nil
	}

	// Build dependency graph
	graph := BuildDependencyGraph(beadList, dependencies)

	// Get topologically sorted beads (respecting dependencies)
	sorted, err := TopologicalSort(graph, beadList)
	if err != nil {
		return nil, fmt.Errorf("failed to sort beads: %w", err)
	}

	// Estimate complexity and tokens for each bead
	estimates := make(map[string]beadEstimate)
	for _, bead := range sorted {
		score, tokens, err := p.estimator.Estimate(ctx, bead)
		if err != nil {
			return nil, fmt.Errorf("failed to estimate complexity for %s: %w", bead.ID, err)
		}
		estimates[bead.ID] = beadEstimate{score: score, tokens: tokens}
	}

	// Process beads in topological order to ensure dependencies are assigned first.
	// Use token estimates only for selecting which existing task to place a bead in.
	tasks := binPackBeads(sorted, estimates, graph, budget)

	return tasks, nil
}

// binPackBeads assigns beads to tasks using bin-packing algorithm.
// Beads are processed in topological order (dependencies first) to ensure
// task indices respect the dependency graph. Token estimates are used only
// for selecting which existing task to place a bead in.
func binPackBeads(sortedBeads []beads.Bead, estimates map[string]beadEstimate, graph *DependencyGraph, budget int) []Task {
	var tasks []Task
	assigned := make(map[string]int) // bead ID -> task index

	for _, bead := range sortedBeads {
		est := estimates[bead.ID]
		taskIdx := findBestTask(bead.ID, est.tokens, tasks, assigned, graph, budget)

		if taskIdx == -1 {
			// Create new task
			taskIdx = len(tasks)
			tasks = append(tasks, Task{
				ID:              fmt.Sprintf("task-%d", taskIdx+1),
				BeanIDs:         []string{},
				Beads:           []beads.Bead{},
				Complexity:      0,
				EstimatedTokens: 0,
				Status:          StatusPending,
			})
		}

		// Add bead to task
		tasks[taskIdx].BeanIDs = append(tasks[taskIdx].BeanIDs, bead.ID)
		tasks[taskIdx].Beads = append(tasks[taskIdx].Beads, bead)
		tasks[taskIdx].Complexity += est.score
		tasks[taskIdx].EstimatedTokens += est.tokens
		assigned[bead.ID] = taskIdx
	}

	return tasks
}

// findBestTask finds the best task for a bead, or -1 if a new task is needed.
// The tokens parameter is the estimated token usage for this bead.
// The budget is the maximum tokens per task.
func findBestTask(beanID string, tokens int, tasks []Task, assigned map[string]int, graph *DependencyGraph, budget int) int {
	bestIdx := -1
	bestFit := budget + 1 // Initialize to impossible value

	for i := range tasks {
		// Check if adding this bead would exceed token budget
		newTokens := tasks[i].EstimatedTokens + tokens
		if newTokens > budget {
			continue
		}

		// Check dependency constraints:
		// All dependencies must either be:
		// 1. In a previous task (already completed)
		// 2. In the same task
		if !canAddToTask(beanID, i, assigned, graph) {
			continue
		}

		// Best-fit: prefer the task with least remaining space
		remaining := budget - newTokens
		if remaining < bestFit {
			bestIdx = i
			bestFit = remaining
		}
	}

	return bestIdx
}

// canAddToTask checks if a bead can be added to a task based on dependency constraints.
func canAddToTask(beanID string, taskIdx int, assigned map[string]int, graph *DependencyGraph) bool {
	// Check that all dependencies are either in an earlier task or the same task
	for _, depID := range graph.DependsOn[beanID] {
		depTaskIdx, ok := assigned[depID]
		if !ok {
			// Dependency not yet assigned - can't add this bead yet
			return false
		}
		if depTaskIdx > taskIdx {
			// Dependency is in a later task - would create cycle
			return false
		}
	}
	return true
}
