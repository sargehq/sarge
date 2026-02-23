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

// Task represents a virtual task - a group of beads to be processed together.
type Task struct {
	ID              string       // Unique task identifier
	BeanIDs         []string     // IDs of beads in this task
	Beads           []beans.Bean // Full bead information
	Complexity      int          // Sum of bead complexity scores
	EstimatedTokens int          // Sum of estimated tokens for all beads
	Status          string       // pending, processing, completed, failed
}

// Planner creates task groupings from a list of beads.
type Planner interface {
	// Plan analyzes beads and creates task assignments based on token budget.
	// The budget represents the target tokens per task (e.g., 120000 for 120K tokens).
	// Returns a list of tasks with beads grouped to respect dependencies and fit within budget.
	Plan(
		ctx context.Context,
		beadList []beans.Bean,
		dependencies map[string][]beans.Dependency,
		budget int,
	) ([]Task, error)
}

// ComplexityEstimator estimates the complexity of a bead.
type ComplexityEstimator interface {
	// Estimate returns a complexity score (1-10) and estimated context tokens for a bead.
	Estimate(ctx context.Context, bead beans.Bean) (score int, tokens int, err error)
}

// BeadComplexity holds complexity information for a single bead.
type BeadComplexity struct {
	BeanID          string
	ComplexityScore int // 1-10 scale
	EstimatedTokens int
}
