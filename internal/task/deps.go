package task

import (
	"fmt"

	"github.com/sargehq/sarge/internal/beads"
)

// DependencyGraph represents bead dependencies.
type DependencyGraph struct {
	// DependsOn maps bead ID to IDs it depends on
	DependsOn map[string][]string
	// Dependents maps bead ID to IDs that depend on it (renamed from BlockedBy)
	Dependents map[string][]string
}

// BuildDependencyGraph creates a dependency graph from beads and their dependencies.
func BuildDependencyGraph(
	beadList []beads.Bead,
	dependencies map[string][]beads.Dependency,
) *DependencyGraph {
	graph := &DependencyGraph{
		DependsOn:  make(map[string][]string),
		Dependents: make(map[string][]string),
	}

	// Create set of valid bead IDs
	validIDs := make(map[string]bool)
	for _, bead := range beadList {
		validIDs[bead.ID] = true
		// Initialize empty slices for this bead
		graph.DependsOn[bead.ID] = []string{}
		graph.Dependents[bead.ID] = []string{}
	}

	// Build dependency relationships from the dependencies map
	for beanID, deps := range dependencies {
		for _, dep := range deps {
			// Only add dependencies that are in our bead set
			if validIDs[dep.DependsOnID] {
				if dep.Type == "blocks" || dep.Type == "blocked_by" {
					graph.DependsOn[beanID] = append(graph.DependsOn[beanID], dep.DependsOnID)
					graph.Dependents[dep.DependsOnID] = append(graph.Dependents[dep.DependsOnID], beanID)
				}
			}
		}
	}

	return graph
}

// TopologicalSort returns beads in dependency order (dependencies before dependents).
func TopologicalSort(graph *DependencyGraph, beadList []beads.Bead) ([]beads.Bead, error) {
	beadMap := make(map[string]beads.Bead)
	for _, bead := range beadList {
		beadMap[bead.ID] = bead
	}

	// Kahn's algorithm
	inDegree := make(map[string]int)
	for _, bead := range beadList {
		inDegree[bead.ID] = len(graph.DependsOn[bead.ID])
	}

	// Start with beads that have no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var result []beads.Bead
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]

		result = append(result, beadMap[id])

		// Reduce in-degree of dependents
		for _, dependent := range graph.Dependents[id] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for cycles
	if len(result) != len(beadList) {
		return nil, fmt.Errorf("dependency cycle detected")
	}

	return result, nil
}
