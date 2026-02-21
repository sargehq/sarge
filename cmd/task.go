package cmd

import (
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	flagTaskStatus string
	flagTaskType   string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
	Long:  `Commands for managing and inspecting tasks in the sarge orchestrator.`,
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks",
	Long: `List all tasks in the tracking proj.DB.

Examples:
  sarge task list                    # List all tasks
  sarge task list --status pending   # List pending tasks
  sarge task list --status completed # List completed tasks
  sarge task list --type estimate    # List estimate tasks`,
	RunE: runTaskList,
}

var taskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show detailed information about a task",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskShow,
}

var taskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id>...",
	Short: "Delete one or more tasks from the database",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runTaskDelete,
}

var taskResetCmd = &cobra.Command{
	Use:   "reset <task-id>",
	Short: "Reset a failed or stuck task to pending",
	Args:  cobra.ExactArgs(1),
	RunE:  runTaskReset,
}

func init() {
	rootCmd.AddCommand(taskCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskShowCmd)
	taskCmd.AddCommand(taskDeleteCmd)
	taskCmd.AddCommand(taskResetCmd)

	// List command flags
	taskListCmd.Flags().StringVar(&flagTaskStatus, "status", "", "filter by status (pending, processing, completed, failed)")
	taskListCmd.Flags().StringVar(&flagTaskType, "type", "", "filter by type (estimate, implement)")
}

func runTaskList(cmd *cobra.Command, args []string) error {
	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	defer proj.Close()

	// Get all tasks
	var tasks []*db.Task
	tasks, err = proj.DB.ListTasks(ctx, flagTaskStatus)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	// Filter by type if specified
	if flagTaskType != "" {
		var filtered []*db.Task
		for _, task := range tasks {
			if task.TaskType == flagTaskType {
				filtered = append(filtered, task)
			}
		}
		tasks = filtered
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found")
		return nil
	}

	// Print header
	fmt.Printf("%-20s %-12s %-10s %-8s %-20s %s\n",
		"ID", "Status", "Type", "Budget", "Created", "Beads")
	fmt.Println(strings.Repeat("-", 100))

	// Print each task
	for _, task := range tasks {
		// Get beads for this task
		beadIDs, err := proj.DB.GetTaskBeads(ctx, task.ID)
		if err != nil {
			beadIDs = []string{"<error>"}
		}

		// Format status with color codes for terminal
		statusDisplay := formatStatus(task.Status)

		// Format type
		typeDisplay := task.TaskType
		if typeDisplay == "" {
			typeDisplay = "implement"
		}

		// Format created time
		createdDisplay := task.CreatedAt.Format("2006-01-02 15:04")

		// Format budget
		budgetDisplay := "-"
		if task.ComplexityBudget > 0 {
			budgetDisplay = fmt.Sprintf("%d", task.ComplexityBudget)
		}

		// Print task row
		fmt.Printf("%-20s %-12s %-10s %-8s %-20s %s\n",
			task.ID,
			statusDisplay,
			typeDisplay,
			budgetDisplay,
			createdDisplay,
			strings.Join(beadIDs, ", "))

		// Show dependencies (what this task depends on)
		deps, err := proj.DB.GetTaskDependencies(ctx, task.ID)
		if err == nil && len(deps) > 0 {
			fmt.Printf("  ├─ Depends on: %s\n", strings.Join(deps, ", "))
		}

		// Show dependents (what depends on this task)
		dependents, err := proj.DB.GetTaskDependents(ctx, task.ID)
		if err == nil && len(dependents) > 0 {
			fmt.Printf("  ├─ Blocks: %s\n", strings.Join(dependents, ", "))
		}

		// Show error message if failed
		if task.Status == db.StatusFailed && task.ErrorMessage != "" {
			fmt.Printf("  ├─ Error: %s\n", task.ErrorMessage)
		}

		// Show PR URL if completed
		if task.Status == db.StatusCompleted && task.PRURL != "" {
			fmt.Printf("  └─ PR: %s\n", task.PRURL)
		}
	}

	// Summary
	fmt.Printf("\nTotal: %d task(s)\n", len(tasks))

	return nil
}

func formatStatus(status string) string {
	switch status {
	case db.StatusPending:
		return "pending"
	case db.StatusProcessing:
		return "processing"
	case db.StatusCompleted:
		return "✓ completed"
	case db.StatusFailed:
		return "✗ failed"
	default:
		return status
	}
}

func runTaskShow(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	taskID := args[0]

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	defer proj.Close()

	// Get task
	task, err := proj.DB.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Get beads for this task
	beadIDs, err := proj.DB.GetTaskBeads(ctx, task.ID)
	if err != nil {
		return fmt.Errorf("failed to get task beads: %w", err)
	}

	// Print task details
	fmt.Printf("Task ID:     %s\n", task.ID)
	fmt.Printf("Status:      %s\n", formatStatus(task.Status))
	fmt.Printf("Type:        %s\n", func() string {
		if task.TaskType == "" {
			return "implement"
		}
		return task.TaskType
	}())

	if task.ComplexityBudget > 0 {
		fmt.Printf("Budget:      %d\n", task.ComplexityBudget)
	}
	if task.ActualComplexity > 0 {
		fmt.Printf("Actual:      %d\n", task.ActualComplexity)
	}

	fmt.Printf("Created:     %s\n", task.CreatedAt.Format("2006-01-02 15:04:05"))
	if task.StartedAt != nil {
		fmt.Printf("Started:     %s\n", task.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if task.CompletedAt != nil {
		fmt.Printf("Completed:   %s\n", task.CompletedAt.Format("2006-01-02 15:04:05"))
	}

	if task.WorktreePath != "" {
		fmt.Printf("Worktree:    %s\n", task.WorktreePath)
	}

	if task.PRURL != "" {
		fmt.Printf("PR:          %s\n", task.PRURL)
	}
	if task.ErrorMessage != "" {
		fmt.Printf("Error:       %s\n", task.ErrorMessage)
	}

	// Print beads
	fmt.Printf("\nBeads (%d):\n", len(beadIDs))
	for _, beadID := range beadIDs {
		// Get bead status
		beadStatus, err := proj.DB.GetTaskBeadStatus(ctx, taskID, beadID)
		if err != nil || beadStatus == "" {
			beadStatus = "unknown"
		}
		fmt.Printf("  - %s (%s)\n", beadID, beadStatus)
	}

	// Print metadata if any
	metadata, err := proj.DB.GetAllTaskMetadata(ctx, taskID)
	if err == nil && len(metadata) > 0 {
		fmt.Printf("\nMetadata:\n")
		for key, value := range metadata {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}

	return nil
}

func runTaskDelete(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	defer proj.Close()

	// Delete each task
	for _, taskID := range args {
		// Check task exists
		task, err := proj.DB.GetTask(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get task %s: %w", taskID, err)
		}
		if task == nil {
			return fmt.Errorf("task %s not found", taskID)
		}

		// Delete task and all associated records (uses transaction internally)
		if err := proj.DB.DeleteTask(ctx, taskID); err != nil {
			return fmt.Errorf("failed to delete task %s: %w", taskID, err)
		}

		fmt.Printf("Deleted task %s\n", taskID)
	}

	return nil
}

func runTaskReset(cmd *cobra.Command, args []string) error {
	taskID := args[0]
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	defer proj.Close()

	// Check task exists
	task, err := proj.DB.GetTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}

	// Reset task status
	if err := proj.DB.ResetTaskStatus(ctx, taskID); err != nil {
		return fmt.Errorf("failed to reset task status: %w", err)
	}

	// Reset all bead statuses for this task
	if err := proj.DB.ResetTaskBeadStatuses(ctx, taskID); err != nil {
		return fmt.Errorf("failed to reset bead statuses: %w", err)
	}

	fmt.Printf("Reset task %s to pending\n", taskID)
	return nil
}
