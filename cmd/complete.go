package cmd

import (
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/feedback"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	flagCompletePRURL   string
	flagCompleteProject string
	flagCompleteError   string
)

var completeCmd = &cobra.Command{
	Use:   "complete <bead-id|task-id>",
	Short: "[Agent] Mark a bead or task as completed (or failed with --error)",
	Long: `[Agent Command - Called by Claude Code, not for direct user invocation]

Mark a bead or task as completed in the tracking database.
With --error flag, marks the task as failed instead.

This command is called by Claude Code during task execution to report completion status.`,
	Args: cobra.ExactArgs(1),
	RunE: runComplete,
}

func init() {
	completeCmd.Flags().StringVar(&flagCompletePRURL, "pr", "", "PR URL to associate with completion")
	completeCmd.Flags().StringVar(&flagCompleteProject, "project", "", "project directory (default: auto-detect from cwd)")
	completeCmd.Flags().StringVar(&flagCompleteError, "error", "", "Error message to mark task as failed")
}

func runComplete(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	id := args[0]

	proj, err := project.Find(ctx, flagCompleteProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// If error flag is set, mark task as failed
	if flagCompleteError != "" {
		// Try to fail it as a task
		if err := proj.DB.FailTask(ctx, id, flagCompleteError); err == nil {
			fmt.Printf("Task %s marked as failed: %s\n", id, flagCompleteError)
			return nil
		}
		// If that didn't work, it might not be a valid task ID
		return fmt.Errorf("failed to mark %s as failed (is it a valid task ID?)", id)
	}

	// Check if this is a task ID (contains a dot like "w-xxx.1" or "w-xxx.pr")
	if strings.Contains(id, ".") {
		// Complete task directly - task IDs always contain dots, bead IDs don't
		// First, mark beads as completed based on their actual status in the beads system
		beadIDs, err := proj.DB.GetTaskBeads(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get beads for task %s: %w", id, err)
		}

		var closedBeadIDs []string
		for _, beadID := range beadIDs {
			// Check actual bead status in the beads system
			bead, err := proj.Beads.GetBead(ctx, beadID)
			if err != nil {
				fmt.Printf("Warning: failed to get bead %s status: %v\n", beadID, err)
				continue
			}
			if bead == nil {
				fmt.Printf("Warning: bead %s not found\n", beadID)
				continue
			}

			// Only mark as completed if bead is actually closed
			if bead.Status == beads.StatusClosed {
				if err := proj.DB.CompleteTaskBead(ctx, id, beadID); err != nil {
					fmt.Printf("Warning: failed to mark bead %s as completed: %v\n", beadID, err)
				} else {
					closedBeadIDs = append(closedBeadIDs, beadID)
				}
			}
		}

		// Now complete the task itself
		if err := proj.DB.CompleteTask(ctx, id, flagCompletePRURL); err != nil {
			return fmt.Errorf("failed to complete task %s: %w", id, err)
		}
		fmt.Printf("Task %s marked as completed", id)
		if flagCompletePRURL != "" {
			fmt.Printf(" (PR: %s)", flagCompletePRURL)
		}
		fmt.Println()

		// If PR URL is provided, set it on the work and schedule feedback polling immediately
		// This ensures feedback polling starts when the PR is created, not when work goes idle
		if flagCompletePRURL != "" {
			parts := strings.Split(id, ".")
			if len(parts) >= 1 {
				workID := parts[0]
				prFeedbackInterval := proj.Config.Scheduler.GetPRFeedbackInterval()
				commentResolutionInterval := proj.Config.Scheduler.GetCommentResolutionInterval()
				if err := proj.DB.SetWorkPRURLAndScheduleFeedback(ctx, workID, flagCompletePRURL, prFeedbackInterval, commentResolutionInterval); err != nil {
					fmt.Printf("Warning: failed to schedule PR feedback polling: %v\n", err)
				} else {
					fmt.Println("PR feedback polling scheduled")
				}

				// Spawn workflow watchers immediately to catch fast CI runs
				// This avoids the race condition where CI completes before the first feedback poll
				ghClient := github.NewClient()
				watcherCount, err := control.SpawnWorkflowWatchers(ctx, proj, ghClient, workID, flagCompletePRURL)
				if err != nil {
					fmt.Printf("Warning: failed to spawn workflow watchers: %v\n", err)
				} else if watcherCount > 0 {
					fmt.Printf("Spawned %d workflow watcher(s)\n", watcherCount)
				}
			}
		}

		// Resolve GitHub comments for closed beads
		if len(closedBeadIDs) > 0 {
			// Extract work ID from task ID (e.g., "w-xxx.1" -> "w-xxx")
			parts := strings.Split(id, ".")
			if len(parts) >= 1 {
				workID := parts[0]
				// Resolve feedback comments immediately

				if err := feedback.ResolveFeedbackForBeads(ctx, proj.DB, proj.Beads, workID, closedBeadIDs); err != nil {
					fmt.Printf("Warning: failed to resolve GitHub comments: %v\n", err)
				}
			}
		}

		// Close any parents whose children are all complete
		if err := proj.Beads.CloseEligibleParents(ctx, proj.BeadsPath()); err != nil {
			fmt.Printf("Warning: failed to close eligible parents: %v\n", err)
		}
		return nil
	}

	// Otherwise, continue with normal bead completion logic
	beadID := id

	// Check if this bead is part of a task
	taskID, err := proj.DB.GetTaskForBead(ctx, beadID)
	if err != nil {
		return fmt.Errorf("failed to look up task for bead: %w", err)
	}

	if taskID != "" {
		// Bead is part of a task - mark it complete in task_beads
		if err := proj.DB.CompleteTaskBead(ctx, taskID, beadID); err != nil {
			return fmt.Errorf("failed to complete task bead: %w", err)
		}
		fmt.Printf("Marked bead %s as completed in task %s\n", beadID, taskID)

		// Check if all beads in the task are complete and auto-complete the task
		autoCompleted, err := proj.DB.CheckAndCompleteTask(ctx, taskID, flagCompletePRURL)
		if err != nil {
			return fmt.Errorf("failed to check task completion: %w", err)
		}
		if autoCompleted {
			fmt.Printf("All beads complete - task %s marked as completed", taskID)
			if flagCompletePRURL != "" {
				fmt.Printf(" (PR: %s)", flagCompletePRURL)
			}
			fmt.Println()

			// Resolve GitHub comments for all beads in the task
			taskBeadIDs, err := proj.DB.GetTaskBeads(ctx, taskID)
			if err == nil && len(taskBeadIDs) > 0 {
				// Extract work ID from task ID (e.g., "w-xxx.1" -> "w-xxx")
				parts := strings.Split(taskID, ".")
				if len(parts) >= 1 {
					workID := parts[0]
					if err := feedback.ResolveFeedbackForBeads(ctx, proj.DB, proj.Beads, workID, taskBeadIDs); err != nil {
						fmt.Printf("Warning: failed to resolve GitHub comments: %v\n", err)
					}
				}
			}

			// Close any parents whose children are all complete
			if err := proj.Beads.CloseEligibleParents(ctx, proj.BeadsPath()); err != nil {
				fmt.Printf("Warning: failed to close eligible parents: %v\n", err)
			}
		}

		// Also update the beads table if the bead exists there (backwards compatibility)
		// Ignore "not found" errors since task_beads is the primary tracking for task-based beads
		_ = proj.DB.CompleteBead(ctx, beadID, flagCompletePRURL)
		return nil
	}

	// Standalone bead (not part of a task) - must exist in beads table
	if err := proj.DB.CompleteBead(ctx, beadID, flagCompletePRURL); err != nil {
		// Check if this might be a bead ID that doesn't exist in our tracking
		return fmt.Errorf("failed to complete bead %s: %w (hint: if the bead was closed via 'bd close', it may not be tracked here; use 'sarge complete <task-id>' instead)", beadID, err)
	}

	fmt.Printf("Marked bead %s as completed", beadID)
	if flagCompletePRURL != "" {
		fmt.Printf(" (PR: %s)", flagCompletePRURL)
	}
	fmt.Println()

	return nil
}
