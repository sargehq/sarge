package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
)

// LLMEstimator uses Claude Code via estimate tasks to estimate bead complexity.
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

// Estimate returns a complexity score (1-10) and estimated context tokens for a bead.
// Results are cached based on the description hash.
// Returns (0, 0, nil) if the bead needs estimation but an estimation task was spawned.
func (e *LLMEstimator) Estimate(ctx context.Context, bead beans.Bean) (score int, tokens int, err error) {
	// Calculate description hash for caching
	fullDescription := bead.Title + "\n" + bead.Body
	descHash := db.HashDescription(fullDescription)

	// Check cache first
	if e.database != nil {
		score, tokens, found, err := e.database.GetCachedComplexity(ctx, bead.ID, descHash)
		if err == nil && found {
			return score, tokens, nil
		}
	}

	// For single estimates, run a batch of one (never force)
	result, err := e.EstimateBatch(ctx, []beans.Bean{bead}, false)
	if err != nil {
		return 0, 0, err
	}

	// If a task was spawned, return zeros (estimation in progress)
	if result.TaskSpawned {
		return 0, 0, nil
	}

	// Retrieve the cached result
	score, tokens, found, err := e.database.GetCachedComplexity(ctx, bead.ID, descHash)
	if err != nil || !found {
		return 0, 0, fmt.Errorf("failed to retrieve estimate after batch: %w", err)
	}

	return score, tokens, nil
}

// EstimationResult contains the result of an estimation attempt.
type EstimationResult struct {
	AllCached    bool     // True if all beads already had cached estimates
	TaskSpawned  bool     // True if an estimation task was spawned
	TaskID       string   // The estimation task ID if spawned
	UncachedIDs  []string // IDs of beads that need estimation
}

// EstimateBatch spawns an estimation task for beads without cached complexity.
// This function is non-blocking - it spawns the task and returns immediately.
// Returns EstimationResult indicating whether all beads are cached or if a task was spawned.
func (e *LLMEstimator) EstimateBatch(ctx context.Context, beadList []beans.Bean, forceEstimate bool) (*EstimationResult, error) {
	result := &EstimationResult{}

	if len(beadList) == 0 {
		result.AllCached = true
		return result, nil
	}

	// Filter out already cached beads (unless forcing re-estimation)
	var uncachedBeads []beans.Bean

	if forceEstimate {
		// Force re-estimation of all beads
		fmt.Println("Force re-estimation enabled, ignoring cached estimates")
		uncachedBeads = beadList
		for _, bead := range beadList {
			result.UncachedIDs = append(result.UncachedIDs, bead.ID)
		}
	} else {
		// Normal flow: filter out cached beads
		for _, bead := range beadList {
			fullDescription := bead.Title + "\n" + bead.Body
			descHash := db.HashDescription(fullDescription)
			_, _, found, _ := e.database.GetCachedComplexity(ctx, bead.ID, descHash)
			if !found {
				uncachedBeads = append(uncachedBeads, bead)
				result.UncachedIDs = append(result.UncachedIDs, bead.ID)
			}
		}
	}

	if len(uncachedBeads) == 0 {
		// All beads already cached
		fmt.Printf("All %d bead(s) already have cached complexity estimates\n", len(beadList))
		result.AllCached = true
		return result, nil
	}

	// Cannot estimate here - estimation must happen through tasks run by orchestrator
	return nil, fmt.Errorf("missing complexity estimates for %d bead(s): %s. Use 'sarge run --auto' to run estimation through the orchestrator",
		len(result.UncachedIDs), strings.Join(result.UncachedIDs, ", "))
}
