package control

import (
	"context"
	"fmt"
	"io"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// HandleSpawnOrchestratorTask handles a scheduled orchestrator spawn task
func (cp *ControlPlane) HandleSpawnOrchestratorTask(ctx context.Context, proj *project.Project, task *db.ScheduledTask) error {
	workID := task.WorkID
	workerName := task.Metadata["worker_name"]

	logging.Info("Spawning orchestrator for work",
		"work_id", workID,
		"attempt", task.AttemptCount+1)

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		// Work was deleted - nothing to do
		logging.Info("Work not found, task will be marked completed", "work_id", workID)
		return nil
	}

	if work.WorktreePath == "" {
		return fmt.Errorf("work %s has no worktree path", workID)
	}

	// Spawn the orchestrator
	if err := cp.OrchestratorSpawner.SpawnWorkOrchestrator(ctx, workID, proj.Config.Project.Name, work.WorktreePath, workerName, io.Discard); err != nil {
		return fmt.Errorf("failed to spawn orchestrator: %w", err)
	}

	logging.Info("Orchestrator spawned successfully", "work_id", workID)

	return nil
}
