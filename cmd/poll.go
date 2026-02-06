package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/progress"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	flagPollInterval time.Duration
	flagPollProject  string
	flagPollWork     string
)

var pollCmd = &cobra.Command{
	Use:   "poll [work-id|task-id]",
	Short: "Monitor work/task progress with text output",
	Long: `Poll monitors the progress of works and tasks with simple text output.

Without arguments:
- If in a work directory or --work specified: monitors that work's tasks
- Otherwise: monitors all active works in the project

With an ID:
- If ID contains a dot (e.g., w-xxx.1): monitors that specific task
- If ID is a work ID (e.g., w-xxx): monitors all tasks in that work

Output shows:
- Work status and branch
- Task progress counts
- Status indicators (○ pending, ● processing, ✓ completed, ✗ failed)

For interactive management (create, destroy, plan, run works),
use 'sarge' instead.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPoll,
}

func init() {
	rootCmd.AddCommand(pollCmd)
	pollCmd.Flags().DurationVarP(&flagPollInterval, "interval", "i", 2*time.Second, "polling interval")
	pollCmd.Flags().StringVar(&flagPollProject, "project", "", "project directory (default: auto-detect)")
	pollCmd.Flags().StringVar(&flagPollWork, "work", "", "work ID to monitor (default: auto-detect)")
}

func runPoll(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, flagPollProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// Determine what to monitor
	var workID, taskID string

	if len(args) > 0 {
		argID := args[0]
		if strings.Contains(argID, ".") {
			// Task ID
			taskID = argID
		} else if strings.HasPrefix(argID, "w-") {
			// Work ID
			workID = argID
		} else {
			return fmt.Errorf("invalid ID format: %s", argID)
		}
	} else if flagPollWork != "" {
		workID = flagPollWork
	} else {
		// Try to detect from current directory
		// Ignore errors - poll can show all works as fallback
		workID, _ = detectWorkFromDirectory(proj)
	}

	fmt.Println("Monitoring progress...")
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println()

	ticker := time.NewTicker(flagPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var works []*progress.WorkProgress
			var err error
			if taskID != "" {
				works, err = progress.FetchTaskPollData(ctx, proj, taskID)
			} else if workID != "" {
				works, err = progress.FetchWorkPollData(ctx, proj, workID)
			} else {
				works, err = progress.FetchAllWorksPollData(ctx, proj)
			}
			if err != nil {
				fmt.Printf("[%s] Error: %v\n", time.Now().Format("15:04:05"), err)
				continue
			}

			// Clear screen (ANSI escape)
			fmt.Print("\033[2J\033[H")

			fmt.Printf("=== Progress Update [%s] ===\n\n", time.Now().Format("15:04:05"))

			for _, wp := range works {
				printWorkProgress(wp)
			}

			if len(works) == 0 {
				fmt.Println("No active works found")
			}

			// Check if all work is complete
			allComplete := true
			for _, wp := range works {
				if wp.Work.Status != db.StatusCompleted {
					allComplete = false
					break
				}
			}

			if allComplete && len(works) > 0 {
				fmt.Println("\nAll work completed!")
				return nil
			}
		}
	}
}

func printWorkProgress(wp *progress.WorkProgress) {
	statusSymbol := "?"
	switch wp.Work.Status {
	case db.StatusPending:
		statusSymbol = "○"
	case db.StatusProcessing:
		statusSymbol = "●"
	case db.StatusCompleted:
		statusSymbol = "✓"
	case db.StatusFailed:
		statusSymbol = "✗"
	}

	fmt.Printf("%s Work: %s (%s)\n", statusSymbol, wp.Work.ID, wp.Work.Status)
	fmt.Printf("  Branch: %s\n", wp.Work.BranchName)
	if wp.Work.RootIssueID != "" {
		fmt.Printf("  Root Issue: %s\n", wp.Work.RootIssueID)
	}

	if wp.Work.PRURL != "" {
		fmt.Printf("  PR: %s\n", wp.Work.PRURL)
	}

	completed := 0
	for _, tp := range wp.Tasks {
		if tp.Task.Status == db.StatusCompleted {
			completed++
		}
	}
	fmt.Printf("  Progress: %d/%d tasks\n", completed, len(wp.Tasks))

	fmt.Println("  Tasks:")
	for _, tp := range wp.Tasks {
		taskSymbol := "?"
		switch tp.Task.Status {
		case db.StatusPending:
			taskSymbol = "○"
		case db.StatusProcessing:
			taskSymbol = "●"
		case db.StatusCompleted:
			taskSymbol = "✓"
		case db.StatusFailed:
			taskSymbol = "✗"
		}

		taskType := tp.Task.TaskType
		if taskType == "" {
			taskType = "implement"
		}

		fmt.Printf("    %s %s [%s]\n", taskSymbol, tp.Task.ID, taskType)

		for _, bp := range tp.Beads {
			beadSymbol := "○"
			switch bp.Status {
			case db.StatusCompleted:
				beadSymbol = "✓"
			case db.StatusProcessing:
				beadSymbol = "●"
			case db.StatusFailed:
				beadSymbol = "✗"
			}
			fmt.Printf("      %s %s\n", beadSymbol, bp.ID)
		}

		if tp.Task.Status == db.StatusFailed && tp.Task.ErrorMessage != "" {
			fmt.Printf("      Error: %s\n", tp.Task.ErrorMessage)
		}
	}
	fmt.Println()
}
