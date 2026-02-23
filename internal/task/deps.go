package task

import (
	"fmt"

	"github.com/sargehq/sarge/internal/beans"
)

// DependencyGraph represents bean dependencies.
type DependencyGraph struct {
	// DependsOn maps bean ID to IDs it depends on
	DependsOn map[string][]string
	// Dependents maps bean ID to IDs that depend on it (renamed from BlockedBy)
	Dependents map[string][]string
}

// BuildDependencyGraph creates a dependency graph from beans and their dependencies.
func BuildDependencyGraph(
	beanList []beans.Bean,
	dependencies map[string][]beans.Dependency,
) *DependencyGraph {
	graph := &DependencyGraph{
		DependsOn:  make(map[string][]string),
		Dependents: make(map[string][]string),
	}

	// Create set of valid bean IDs
	validIDs := make(map[string]bool)
	for _, bean := range beanList {
		validIDs[bean.ID] = true
		// Initialize empty slices for this bean
		graph.DependsOn[bean.ID] = []string{}
		graph.Dependents[bean.ID] = []string{}
	}

	// Build dependency relationships from the dependencies map
	for beanID, deps := range dependencies {
		for _, dep := range deps {
			// Only add dependencies that are in our bean set
			if validIDs[dep.BlockedByID] {
				graph.DependsOn[beanID] = append(graph.DependsOn[beanID], dep.BlockedByID)
				graph.Dependents[dep.BlockedByID] = append(graph.Dependents[dep.BlockedByID], beanID)
			}
		}
	}

	return graph
}

// TopologicalSort returns beans in dependency order (dependencies before dependents).
func TopologicalSort(graph *DependencyGraph, beanList []beans.Bean) ([]beans.Bean, error) {
	beanMap := make(map[string]beans.Bean)
	for _, bean := range beanList {
		beanMap[bean.ID] = bean
	}

	// Kahn's algorithm
	inDegree := make(map[string]int)
	for _, bean := range beanList {
		inDegree[bean.ID] = len(graph.DependsOn[bean.ID])
	}

	// Start with beans that have no dependencies
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var result []beans.Bean
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]

		result = append(result, beanMap[id])

		// Reduce in-degree of dependents
		for _, dependent := range graph.Dependents[id] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check for cycles
	if len(result) != len(beanList) {
		return nil, fmt.Errorf("dependency cycle detected")
	}

	return result, nil
}
