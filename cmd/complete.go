package cmd

import (
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/beans"
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
	Use:   "complete <bean-id|task-id>",
	Short: "[Agent] Mark a bean or task as completed (or failed with --error)",
	Long: `[Agent Command - Called by Claude Code, not for direct user invocation]

Mark a bean or task as completed in the tracking database.
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
		// Complete task directly - task IDs always contain dots, bean IDs don't
		// First, mark beans as completed based on their actual status in the beans system
		beanIDs, err := proj.DB.GetTaskBeans(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to get beans for task %s: %w", id, err)
		}

		var closedBeanIDs []string
		for _, beanID := range beanIDs {
			// Check actual bean status in the beans system
			bean, err := proj.Beans.GetBean(ctx, beanID)
			if err != nil {
				fmt.Printf("Warning: failed to get bean %s status: %v\n", beanID, err)
				continue
			}
			if bean == nil {
				fmt.Printf("Warning: bean %s not found\n", beanID)
				continue
			}

			// Only mark as completed if bean is actually closed
			if bean.Status == beans.StatusCompleted {
				if err := proj.DB.CompleteTaskBean(ctx, id, beanID); err != nil {
					fmt.Printf("Warning: failed to mark bean %s as completed: %v\n", beanID, err)
				} else {
					closedBeanIDs = append(closedBeanIDs, beanID)
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

		// Resolve GitHub comments for closed beans
		if len(closedBeanIDs) > 0 {
			// Extract work ID from task ID (e.g., "w-xxx.1" -> "w-xxx")
			parts := strings.Split(id, ".")
			if len(parts) >= 1 {
				workID := parts[0]
				// Resolve feedback comments immediately

				if err := feedback.ResolveFeedbackForBeans(ctx, proj.DB, proj.Beans, workID, closedBeanIDs); err != nil {
					fmt.Printf("Warning: failed to resolve GitHub comments: %v\n", err)
				}
			}
		}

		// Close any parents whose children are all complete
		if err := proj.Beans.CloseEligibleParents(ctx, proj.BeansPath()); err != nil {
			fmt.Printf("Warning: failed to close eligible parents: %v\n", err)
		}
		return nil
	}

	// Otherwise, continue with normal bean completion logic
	beanID := id

	// Check if this bean is part of a task
	taskID, err := proj.DB.GetTaskForBean(ctx, beanID)
	if err != nil {
		return fmt.Errorf("failed to look up task for bean: %w", err)
	}

	if taskID != "" {
		// Bean is part of a task - mark it complete in task_beans
		if err := proj.DB.CompleteTaskBean(ctx, taskID, beanID); err != nil {
			return fmt.Errorf("failed to complete task bean: %w", err)
		}
		fmt.Printf("Marked bean %s as completed in task %s\n", beanID, taskID)

		// Check if all beans in the task are complete and auto-complete the task
		autoCompleted, err := proj.DB.CheckAndCompleteTask(ctx, taskID, flagCompletePRURL)
		if err != nil {
			return fmt.Errorf("failed to check task completion: %w", err)
		}
		if autoCompleted {
			fmt.Printf("All beans complete - task %s marked as completed", taskID)
			if flagCompletePRURL != "" {
				fmt.Printf(" (PR: %s)", flagCompletePRURL)
			}
			fmt.Println()

			// Resolve GitHub comments for all beans in the task
			taskBeanIDs, err := proj.DB.GetTaskBeans(ctx, taskID)
			if err == nil && len(taskBeanIDs) > 0 {
				// Extract work ID from task ID (e.g., "w-xxx.1" -> "w-xxx")
				parts := strings.Split(taskID, ".")
				if len(parts) >= 1 {
					workID := parts[0]
					if err := feedback.ResolveFeedbackForBeans(ctx, proj.DB, proj.Beans, workID, taskBeanIDs); err != nil {
						fmt.Printf("Warning: failed to resolve GitHub comments: %v\n", err)
					}
				}
			}

			// Close any parents whose children are all complete
			if err := proj.Beans.CloseEligibleParents(ctx, proj.BeansPath()); err != nil {
				fmt.Printf("Warning: failed to close eligible parents: %v\n", err)
			}
		}

		// Also update the beans table if the bean exists there (backwards compatibility)
		// Ignore "not found" errors since task_beans is the primary tracking for task-based beans
		_ = proj.DB.CompleteBean(ctx, beanID, flagCompletePRURL)
		return nil
	}

	// Standalone bean (not part of a task) - must exist in beans table
	if err := proj.DB.CompleteBean(ctx, beanID, flagCompletePRURL); err != nil {
		// Check if this might be a bean ID that doesn't exist in our tracking
		return fmt.Errorf("failed to complete bean %s: %w (hint: if the bean was closed via 'bd close', it may not be tracked here; use 'sarge complete <task-id>' instead)", beanID, err)
	}

	fmt.Printf("Marked bean %s as completed", beanID)
	if flagCompletePRURL != "" {
		fmt.Printf(" (PR: %s)", flagCompletePRURL)
	}
	fmt.Println()

	return nil
}
