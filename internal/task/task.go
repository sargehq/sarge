package task

//go:generate moq -stub -out task_mock.go . ComplexityEstimator Planner:PlannerMock

import (
	"context"

	"github.com/sargehq/sarge/internal/beans"
)

// Status constants for task tracking.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

// Task represents a virtual task - a group of beans to be processed together.
type Task struct {
	ID              string       // Unique task identifier
	BeanIDs         []string     // IDs of beans in this task
	Beans           []beans.Bean // Full bean information
	Complexity      int          // Sum of bean complexity scores
	EstimatedTokens int          // Sum of estimated tokens for all beans
	Status          string       // pending, processing, completed, failed
}

// Planner creates task groupings from a list of beans.
type Planner interface {
	// Plan analyzes beans and creates task assignments based on token budget.
	// The budget represents the target tokens per task (e.g., 120000 for 120K tokens).
	// Returns a list of tasks with beans grouped to respect dependencies and fit within budget.
	Plan(
		ctx context.Context,
		beanList []beans.Bean,
		dependencies map[string][]beans.Dependency,
		budget int,
	) ([]Task, error)
}

// ComplexityEstimator estimates the complexity of a bean.
type ComplexityEstimator interface {
	// Estimate returns a complexity score (1-10) and estimated context tokens for a bean.
	Estimate(ctx context.Context, bean beans.Bean) (score int, tokens int, err error)
}

// BeanComplexity holds complexity information for a single bean.
type BeanComplexity struct {
	BeanID          string
	ComplexityScore int // 1-10 scale
	EstimatedTokens int
}
