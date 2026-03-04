package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
)

// LLMEstimator uses the coding agent via estimate tasks to estimate bean complexity.
type LLMEstimator struct {
	database    *db.DB
	workDir     string
	projectName string
	workID      string // Work context for estimation tasks
}

// NewLLMEstimator creates a new LLM-based complexity estimator.
func NewLLMEstimator(database *db.DB, workDir, projectName, workID string) *LLMEstimator {
	return &LLMEstimator{
		database:    database,
		workDir:     workDir,
		projectName: projectName,
		workID:      workID,
	}
}

// Estimate returns a complexity score (1-10) and estimated context tokens for a bean.
// Results are cached based on the description hash.
// Returns (0, 0, nil) if the bean needs estimation but an estimation task was spawned.
func (e *LLMEstimator) Estimate(ctx context.Context, bean beans.Bean) (score int, tokens int, err error) {
	// Calculate description hash for caching
	fullDescription := bean.Title + "\n" + bean.Body
	descHash := db.HashDescription(fullDescription)

	// Check cache first
	if e.database != nil {
		score, tokens, found, err := e.database.GetCachedComplexity(ctx, bean.ID, descHash)
		if err == nil && found {
			return score, tokens, nil
		}
	}

	// For single estimates, run a batch of one (never force)
	result, err := e.EstimateBatch(ctx, []beans.Bean{bean}, false)
	if err != nil {
		return 0, 0, err
	}

	// If a task was spawned, return zeros (estimation in progress)
	if result.TaskSpawned {
		return 0, 0, nil
	}

	// Retrieve the cached result
	score, tokens, found, err := e.database.GetCachedComplexity(ctx, bean.ID, descHash)
	if err != nil || !found {
		return 0, 0, fmt.Errorf("failed to retrieve estimate after batch: %w", err)
	}

	return score, tokens, nil
}

// EstimationResult contains the result of an estimation attempt.
type EstimationResult struct {
	AllCached    bool     // True if all beans already had cached estimates
	TaskSpawned  bool     // True if an estimation task was spawned
	TaskID       string   // The estimation task ID if spawned
	UncachedIDs  []string // IDs of beans that need estimation
}

// EstimateBatch spawns an estimation task for beans without cached complexity.
// This function is non-blocking - it spawns the task and returns immediately.
// Returns EstimationResult indicating whether all beans are cached or if a task was spawned.
func (e *LLMEstimator) EstimateBatch(ctx context.Context, beanList []beans.Bean, forceEstimate bool) (*EstimationResult, error) {
	result := &EstimationResult{}

	if len(beanList) == 0 {
		result.AllCached = true
		return result, nil
	}

	// Filter out already cached beans (unless forcing re-estimation)
	var uncachedBeans []beans.Bean

	if forceEstimate {
		// Force re-estimation of all beans
		fmt.Println("Force re-estimation enabled, ignoring cached estimates")
		uncachedBeans = beanList
		for _, bean := range beanList {
			result.UncachedIDs = append(result.UncachedIDs, bean.ID)
		}
	} else {
		// Normal flow: filter out cached beans
		for _, bean := range beanList {
			fullDescription := bean.Title + "\n" + bean.Body
			descHash := db.HashDescription(fullDescription)
			_, _, found, _ := e.database.GetCachedComplexity(ctx, bean.ID, descHash)
			if !found {
				uncachedBeans = append(uncachedBeans, bean)
				result.UncachedIDs = append(result.UncachedIDs, bean.ID)
			}
		}
	}

	if len(uncachedBeans) == 0 {
		// All beans already cached
		fmt.Printf("All %d bean(s) already have cached complexity estimates\n", len(beanList))
		result.AllCached = true
		return result, nil
	}

	// Cannot estimate here - estimation must happen through tasks run by orchestrator
	return nil, fmt.Errorf("missing complexity estimates for %d bean(s): %s. Use 'sarge run --auto' to run estimation through the orchestrator",
		len(result.UncachedIDs), strings.Join(result.UncachedIDs, ", "))
}
