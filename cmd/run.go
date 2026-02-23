package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/work"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	flagLimit         int
	flagDryRun        bool
	flagProject       string
	flagWork          string
	flagAutoClose     bool
	flagRunPlan       bool
	flagRunAuto       bool
	flagForceEstimate bool
)

var runCmd = &cobra.Command{
	Use:   "run [work-id]",
	Short: "Execute pending tasks for a work unit",
	Long: `Run creates tasks from work beans and executes them.

Before spawning the orchestrator, any unassigned beans in work_beans
are automatically converted to tasks based on their grouping.

Flags:
  --plan     Use LLM complexity estimation to auto-group beans into tasks
  --auto     Run full automated workflow (implement, review/fix loop, PR)

Without arguments:
- If in a work directory or --work specified: runs that work

With an ID:
- If ID is a work ID (e.g., w-xxx): runs that work`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTasks,
}

func init() {
	runCmd.Flags().IntVarP(&flagLimit, "limit", "n", 0, "maximum number of tasks to process (0 = unlimited)")
	runCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "show plan without executing")
	runCmd.Flags().StringVar(&flagProject, "project", "", "project directory (default: auto-detect from cwd)")
	runCmd.Flags().StringVar(&flagWork, "work", "", "work ID to run (default: auto-detect from cwd)")
	runCmd.Flags().BoolVar(&flagAutoClose, "auto-close", false, "automatically close tabs after task completion")
	runCmd.Flags().BoolVar(&flagRunPlan, "plan", false, "use LLM complexity estimation to auto-group beans")
	runCmd.Flags().BoolVar(&flagRunAuto, "auto", false, "run full automated workflow (implement, review/fix, PR)")
	runCmd.Flags().BoolVar(&flagForceEstimate, "force-estimate", false, "force re-estimation of complexity (with --plan)")
}

func runTasks(cmd *cobra.Command, args []string) error {
	var argID string
	if len(args) > 0 {
		argID = args[0]
	}
	ctx := GetContext()

	proj, err := project.Find(ctx, flagProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	fmt.Printf("Using project: %s\n", proj.Config.Project.Name)

	// Determine work context (required)
	// Priority: explicit arg > --work flag > directory context
	var workID string

	if argID != "" {
		// Check if it looks like a work ID
		if strings.HasPrefix(argID, "w-") {
			workID = argID
		} else {
			return fmt.Errorf("invalid ID format: %s (expected w-xxx)", argID)
		}
	} else if flagWork != "" {
		workID = flagWork
	} else {
		// Try to detect work from current directory
		workID, err = detectWorkFromDirectory(proj)
		if err != nil {
			return fmt.Errorf("failed to detect work directory: %w", err)
		}
		if workID == "" {
			return fmt.Errorf("no work context found. Use --work flag or run from a work directory")
		}
	}

	// Get work details to verify it exists
	workRecord, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if workRecord == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	fmt.Printf("\n=== Running work %s ===\n", workRecord.ID)
	fmt.Printf("Branch: %s\n", workRecord.BranchName)
	fmt.Printf("Worktree: %s\n", workRecord.WorktreePath)

	// Create WorkService for this operation
	svc := work.NewWorkService(proj)

	// Validate that work has a root issue
	if workRecord.RootIssueID == "" {
		return fmt.Errorf("work %s has no root issue associated. Create work with a bean ID using 'sarge work create <bean-id>'", workRecord.ID)
	}

	// Check if worktree exists
	if workRecord.WorktreePath == "" {
		return fmt.Errorf("work %s has no worktree path configured", workRecord.ID)
	}

	if !worktree.NewOperations().ExistsPath(workRecord.WorktreePath) {
		return fmt.Errorf("work %s worktree does not exist at %s", workRecord.ID, workRecord.WorktreePath)
	}

	// If --auto, run full automated workflow
	if flagRunAuto {
		result, err := svc.RunWorkAuto(ctx, workID, os.Stdout)
		if err != nil {
			return fmt.Errorf("failed to run automated workflow: %w", err)
		}
		fmt.Println("\nAutomated workflow started.")
		if result.OrchestratorSpawned {
			fmt.Println("Orchestrator spawned in zellij tab.")
		}
		// Ensure control plane is running (handles scheduled tasks like PR feedback polling)
		if _, err := control.EnsureControlPlane(ctx, proj); err != nil {
			fmt.Printf("Warning: failed to ensure control plane: %v\n", err)
		}
		fmt.Println("Switch to the zellij session to monitor progress.")
		return nil
	}

	// Run work (creates tasks and ensures orchestrator is running)
	result, err := svc.RunWorkWithOptions(ctx, workID, work.RunWorkOptions{UsePlan: flagRunPlan, ForceEstimate: flagForceEstimate}, os.Stdout)
	if err != nil {
		return fmt.Errorf("failed to run work: %w", err)
	}

	if result.TasksCreated > 0 {
		fmt.Printf("\nCreated %d task(s) from work beans.\n", result.TasksCreated)
	}

	if result.OrchestratorSpawned {
		fmt.Println("\nOrchestrator spawned in zellij tab.")
	} else {
		fmt.Println("\nOrchestrator is already running.")
	}

	// Ensure control plane is running (handles scheduled tasks like PR feedback polling)
	if _, err := control.EnsureControlPlane(ctx, proj); err != nil {
		fmt.Printf("Warning: failed to ensure control plane: %v\n", err)
	}

	fmt.Println("Switch to the zellij session to monitor progress.")
	return nil
}
