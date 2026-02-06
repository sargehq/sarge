package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/agents"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/control"
	workpkg "github.com/sargehq/sarge/internal/work"
	"github.com/spf13/cobra"
)

var workCmd = &cobra.Command{
	Use:   "work",
	Short: "Manage work units",
	Long:  `Manage work units that group tasks together. Each work has its own git worktree and feature branch.`,
}

var workCreateCmd = &cobra.Command{
	Use:   "create <bead-id>",
	Short: "Create a new work unit from a bead",
	Long: `Create a new work unit from a single bead.
Creates a subdirectory with a git worktree for isolated development.

If the bead is an epic, all child beads are automatically included.
Transitive dependencies are also included.

Branch name is auto-generated from the bead title - you'll be prompted to accept or customize.

With --auto flag, runs the full automated workflow:
1. Creates tasks from beads
2. Executes all tasks
3. Runs review-fix loop until clean
4. Creates a pull request`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkCreate,
}

var workListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all work units",
	Long:  `List all work units with their status and details.`,
	Args:  cobra.NoArgs,
	RunE:  runWorkList,
}

var workShowCmd = &cobra.Command{
	Use:   "show [<id>]",
	Short: "Show work details (current directory or specified)",
	Long: `Show detailed information about a work unit.
If no ID is provided, shows the work for the current directory context.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkShow,
}

var workDestroyCmd = &cobra.Command{
	Use:   "destroy <id>",
	Short: "Destroy a work unit and its worktree",
	Long: `Destroy a work unit, removing its subdirectory and database records.
This is a destructive operation that cannot be undone.`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkDestroy,
}

var workPRCmd = &cobra.Command{
	Use:   "pr [<id>]",
	Short: "Create a PR task for Claude to generate pull request",
	Long: `Create a special task for Claude to review the work and create a pull request.
If no ID is provided, uses the work for the current directory context.

Claude will analyze all completed tasks and beads to generate a comprehensive PR description.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkPR,
}

var workReviewCmd = &cobra.Command{
	Use:   "review [<id>]",
	Short: "Create a review task to examine code changes",
	Long: `Create a task for Claude to review code changes in a work unit.
If no ID is provided, uses the work for the current directory context.

Claude will examine the work's branch/PR for quality, security issues,
and adherence to project standards.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkReview,
}

var workAddCmd = &cobra.Command{
	Use:   "add <bead-ids...>",
	Short: "Add beads to work",
	Long: `Add beads to an existing work unit.

Multiple beads can be specified separated by spaces or commas.
Epics are automatically expanded to include all child beads.

Use --plan when running to let the LLM group beads intelligently,
or --auto for a fully automated workflow.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWorkAdd,
}

var workRemoveCmd = &cobra.Command{
	Use:   "remove <bead-ids...>",
	Short: "Remove beads from work",
	Long: `Remove beads from an existing work unit.
Beads that are already assigned to a pending or processing task cannot be removed.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runWorkRemove,
}

var workConsoleCmd = &cobra.Command{
	Use:   "console [<id>]",
	Short: "Open a console tab in the work's worktree",
	Long: `Open a zellij tab with a shell in the work's worktree.
If no ID is provided, uses the work for the current directory context.

This is useful for running tests, exploring the codebase, or debugging
while the orchestrator runs in a separate tab.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkConsole,
}

var workClaudeCmd = &cobra.Command{
	Use:   "claude [<id>]",
	Short: "Open a Claude Code session in the work's worktree",
	Long: `Open a zellij tab with an interactive Claude Code session in the work's worktree.
If no ID is provided, uses the work for the current directory context.

This is useful for manual exploration, debugging, or ad-hoc changes
while the orchestrator runs in a separate tab.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkClaude,
}

var workRestartCmd = &cobra.Command{
	Use:   "restart [<id>]",
	Short: "Restart a failed work",
	Long: `Restart a work that is in failed state.

When a task fails, the work is marked as failed and the orchestrator halts.
Use this command after resolving the issue (e.g., resetting or deleting the failed task)
to resume processing.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkRestart,
}

var workCompleteCmd = &cobra.Command{
	Use:   "complete [<id>]",
	Short: "Mark an idle work as completed",
	Long: `Explicitly mark a work as completed.

When all tasks finish, the work enters an idle state waiting for more tasks.
Use this command to mark the work as truly completed (e.g., after PR is merged).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runWorkComplete,
}

var (
	flagAutoRun    bool
	flagReviewAuto bool
	flagAddWork    string
	flagRemoveWork string
	flagBranchName string
	flagFromBranch string
	flagYes        bool
)

func init() {
	workCreateCmd.Flags().BoolVar(&flagAutoRun, "auto", false, "run full automated workflow (implement, review, fix, PR)")
	workCreateCmd.Flags().StringVar(&flagBranchName, "branch", "", "branch name to use (skip prompt)")
	workCreateCmd.Flags().StringVar(&flagFromBranch, "from-branch", "", "use an existing git branch instead of creating a new one")
	workCreateCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "skip confirmation prompts")
	workReviewCmd.Flags().BoolVar(&flagReviewAuto, "auto", false, "run review-fix loop until clean")
	workAddCmd.Flags().StringVar(&flagAddWork, "work", "", "work ID (default: auto-detect from current directory)")
	workRemoveCmd.Flags().StringVar(&flagRemoveWork, "work", "", "work ID (default: auto-detect from current directory)")
	workCmd.AddCommand(workCreateCmd)
	workCmd.AddCommand(workListCmd)
	workCmd.AddCommand(workShowCmd)
	workCmd.AddCommand(workDestroyCmd)
	workCmd.AddCommand(workPRCmd)
	workCmd.AddCommand(workReviewCmd)
	workCmd.AddCommand(workAddCmd)
	workCmd.AddCommand(workRemoveCmd)
	workCmd.AddCommand(workConsoleCmd)
	workCmd.AddCommand(workClaudeCmd)
	workCmd.AddCommand(workFeedbackCmd)
	workCmd.AddCommand(workRestartCmd)
	workCmd.AddCommand(workCompleteCmd)
}

func runWorkCreate(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Create WorkService for this operation
	svc := workpkg.NewWorkService(proj)

	// Get base branch from project config
	baseBranch := proj.Config.Repo.GetBaseBranch()

	mainRepoPath := proj.MainRepoPath()
	gitOps := git.NewOperations()
	beadID := args[0]

	// Expand the bead (handles epics and transitive deps)
	expandedIssueIDs, err := workpkg.CollectIssueIDsForAutomatedWorkflow(ctx, beadID, proj.Beads)
	if err != nil {
		return fmt.Errorf("failed to expand bead %s: %w", beadID, err)
	}

	if len(expandedIssueIDs) == 0 {
		return fmt.Errorf("no beads found for %s", beadID)
	}

	// Get issue details for branch name generation
	issuesResult, err := proj.Beads.GetBeadsWithDeps(ctx, expandedIssueIDs)
	if err != nil {
		return fmt.Errorf("failed to get issue details: %w", err)
	}

	// Convert to slice of issue pointers for branch name generation
	var groupIssues []*beads.Bead
	for _, issueID := range expandedIssueIDs {
		if issue, ok := issuesResult.Beads[issueID]; ok {
			issueCopy := issue
			groupIssues = append(groupIssues, &issueCopy)
		}
	}

	// Determine branch name
	var branchName string
	var useExistingBranch bool

	if flagFromBranch != "" {
		// Use an existing branch
		branchName = flagFromBranch
		useExistingBranch = true

		// Validate the branch exists
		existsLocal, existsRemote, err := gitOps.ValidateExistingBranch(ctx, mainRepoPath, branchName)
		if err != nil {
			return fmt.Errorf("failed to validate branch: %w", err)
		}
		if !existsLocal && !existsRemote {
			return fmt.Errorf("branch %s does not exist locally or on remote", branchName)
		}
	} else if flagBranchName != "" {
		// Use provided branch name
		branchName = flagBranchName
	} else {
		// Generate branch name from issue titles
		branchName = workpkg.GenerateBranchNameFromIssues(groupIssues)
		branchName, err = workpkg.EnsureUniqueBranchName(ctx, gitOps, mainRepoPath, branchName)
		if err != nil {
			return fmt.Errorf("failed to find unique branch name: %w", err)
		}

		// Prompt user unless -y flag is set
		if !flagYes {
			branchName, err = promptForBranchName(branchName)
			if err != nil {
				return err
			}
		}
	}

	// Create work asynchronously (control plane handles worktree creation, git push, orchestrator spawn)
	result, err := svc.CreateWorkAsyncWithOptions(ctx, workpkg.CreateWorkAsyncOptions{
		BranchName:        branchName,
		BaseBranch:        baseBranch,
		RootIssueID:       beadID,
		Auto:              flagAutoRun,
		UseExistingBranch: useExistingBranch,
		BeadIDs:           expandedIssueIDs,
	})
	if err != nil {
		return fmt.Errorf("failed to create work: %w", err)
	}

	fmt.Printf("\nCreated work: %s\n", result.WorkID)
	if result.WorkerName != "" {
		fmt.Printf("Worker: %s\n", result.WorkerName)
	}
	fmt.Printf("Branch: %s\n", result.BranchName)
	fmt.Printf("Base Branch: %s\n", result.BaseBranch)

	// Display beads
	fmt.Printf("\nBeads (%d):\n", len(groupIssues))
	for _, issue := range groupIssues {
		fmt.Printf("  - %s: %s\n", issue.ID, issue.Title)
	}

	// Ensure zellij session and control plane are running
	sessionResult, err := control.EnsureControlPlane(ctx, proj)
	if err != nil {
		fmt.Printf("Warning: failed to ensure control plane: %v\n", err)
	} else if sessionResult.SessionCreated {
		printSessionCreatedNotification(sessionResult.SessionName)
	}

	if flagAutoRun {
		fmt.Println("\nAutomated workflow will start once the control plane creates the worktree.")
	}

	fmt.Printf("\nThe control plane will create the worktree and start the orchestrator.\n")
	fmt.Printf("Switch to the zellij session to monitor progress.\n")

	return nil
}

// parseBeadArgs parses positional args into bead IDs.
// Both commas and spaces are treated as separators.
// Epics are expanded to their child beads.
// Returns an error if duplicate bead IDs are found.
func parseBeadArgs(ctx context.Context, args []string, beadsClient *beads.Client) ([]string, error) {
	seenBeads := make(map[string]bool)
	var allIssueIDs []string

	for _, arg := range args {
		// Split comma-separated bead IDs (commas and spaces both work as separators)
		beadIDs := workpkg.ParseBeadIDs(arg)
		if len(beadIDs) == 0 {
			continue
		}

		for _, beadID := range beadIDs {
			// Expand this bead (handles epics and transitive deps)
			expandedIDs, err := workpkg.CollectIssueIDsForAutomatedWorkflow(ctx, beadID, beadsClient)
			if err != nil {
				return nil, fmt.Errorf("failed to expand bead %s: %w", beadID, err)
			}
			for _, issueID := range expandedIDs {
				// Check for duplicates
				if seenBeads[issueID] {
					return nil, fmt.Errorf("duplicate bead %s specified", issueID)
				}
				seenBeads[issueID] = true
				allIssueIDs = append(allIssueIDs, issueID)
			}
		}
	}

	return allIssueIDs, nil
}

// promptForBranchName prompts the user to accept or customize the branch name.
func promptForBranchName(proposed string) (string, error) {
	fmt.Printf("\nProposed branch name: %s\n", proposed)
	fmt.Print("Accept? [Y/n/custom]: ")

	var response string
	fmt.Scanln(&response)
	response = strings.TrimSpace(response)

	if response == "" || strings.ToLower(response) == "y" {
		return proposed, nil
	}

	if strings.ToLower(response) == "n" {
		fmt.Print("Enter branch name: ")
		fmt.Scanln(&response)
		response = strings.TrimSpace(response)
		if response == "" {
			return "", fmt.Errorf("branch name cannot be empty")
		}
		return response, nil
	}

	// User entered a custom branch name directly
	return response, nil
}

// runWorkAdd adds beads to an existing work.
func runWorkAdd(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Create WorkService for this operation
	svc := workpkg.NewWorkService(proj)

	// Get work ID
	workID := flagAddWork
	if workID == "" {
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no --work specified")
		}
	}

	// Parse bead IDs from args
	beadIDs, err := parseBeadArgs(ctx, args, proj.Beads)
	if err != nil {
		return err
	}

	if len(beadIDs) == 0 {
		return fmt.Errorf("no beads specified")
	}

	// Add beads to work using WorkService (handles validation internally)
	result, err := svc.AddBeads(ctx, workID, beadIDs)
	if err != nil {
		return err
	}

	fmt.Printf("Added %d bead(s) to work %s\n", result.BeadsAdded, workID)
	return nil
}

// runWorkRemove removes beads from an existing work.
func runWorkRemove(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Create WorkService for this operation
	svc := workpkg.NewWorkService(proj)

	// Get work ID
	workID := flagRemoveWork
	if workID == "" {
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no --work specified")
		}
	}

	// Collect and trim bead IDs
	var beadIDs []string
	for _, beadID := range args {
		beadID = strings.TrimSpace(beadID)
		if beadID != "" {
			beadIDs = append(beadIDs, beadID)
		}
	}

	if len(beadIDs) == 0 {
		return fmt.Errorf("no beads specified")
	}

	// Remove beads using WorkService (handles validation internally)
	result, err := svc.RemoveBeads(ctx, workID, beadIDs)
	if err != nil {
		return err
	}

	fmt.Printf("Removed %d bead(s) from work %s\n", result.BeadsRemoved, workID)
	return nil
}

func runWorkList(cmd *cobra.Command, args []string) error {
	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// List all works
	works, err := proj.DB.ListWorks(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to list works: %w", err)
	}

	if len(works) == 0 {
		fmt.Println("No work units found.")
		return nil
	}

	// Display works
	fmt.Printf("%-10s %-12s %-15s %-20s %s\n", "ID", "Status", "Root Issue", "Branch", "PR URL")
	fmt.Printf("%-10s %-12s %-15s %-20s %s\n", strings.Repeat("-", 10), strings.Repeat("-", 12), strings.Repeat("-", 15), strings.Repeat("-", 20), strings.Repeat("-", 30))

	for _, work := range works {
		prURL := work.PRURL
		if prURL == "" {
			prURL = "-"
		}
		rootIssue := work.RootIssueID
		if rootIssue == "" {
			rootIssue = "-"
		}
		fmt.Printf("%-10s %-12s %-15s %-20s %s\n", work.ID, work.Status, rootIssue, work.BranchName, prURL)
	}

	// Show summary
	statusCounts := make(map[string]int)
	for _, work := range works {
		statusCounts[work.Status]++
	}

	fmt.Printf("\nTotal: %d work(s)", len(works))
	if len(statusCounts) > 0 {
		fmt.Print(" (")
		first := true
		for status, count := range statusCounts {
			if !first {
				fmt.Print(", ")
			}
			fmt.Printf("%d %s", count, status)
			first = false
		}
		fmt.Print(")")
	}
	fmt.Println()

	return nil
}

func runWorkShow(cmd *cobra.Command, args []string) error {
	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		// Try to detect work from current directory
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	// Display work details
	fmt.Printf("Work: %s\n", work.ID)
	fmt.Printf("Status: %s\n", work.Status)
	if work.RootIssueID != "" {
		fmt.Printf("Root Issue: %s\n", work.RootIssueID)
	}
	fmt.Printf("Branch: %s\n", work.BranchName)
	fmt.Printf("Base Branch: %s\n", work.BaseBranch)
	fmt.Printf("Worktree: %s\n", work.WorktreePath)

	if work.PRURL != "" {
		fmt.Printf("PR URL: %s\n", work.PRURL)
		// Display PR status details
		if work.PRState != "" {
			fmt.Printf("PR State: %s\n", work.PRState)
		}
		if work.CIStatus != "" {
			fmt.Printf("CI Status: %s\n", work.CIStatus)
		}
		if work.ApprovalStatus != "" {
			fmt.Printf("Approval Status: %s\n", work.ApprovalStatus)
		}
	}

	if work.ErrorMessage != "" {
		fmt.Printf("Error: %s\n", work.ErrorMessage)
	}

	if work.ZellijSession != "" {
		fmt.Printf("Zellij Session: %s\n", work.ZellijSession)
		if work.ZellijTab != "" {
			fmt.Printf("Zellij Tab: %s\n", work.ZellijTab)
		}
	}

	fmt.Printf("Created: %s\n", work.CreatedAt.Format("2006-01-02 15:04:05"))

	if work.StartedAt != nil {
		fmt.Printf("Started: %s\n", work.StartedAt.Format("2006-01-02 15:04:05"))
	}

	if work.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", work.CompletedAt.Format("2006-01-02 15:04:05"))
	}

	// Get tasks for this work
	tasks, err := proj.DB.GetWorkTasks(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work tasks: %w", err)
	}

	if len(tasks) > 0 {
		fmt.Printf("\nTasks (%d):\n", len(tasks))
		for i, task := range tasks {
			fmt.Printf("  %d. %s [%s]\n", i+1, task.ID, task.Status)
		}
	} else {
		fmt.Println("\nNo tasks planned for this work yet.")
	}

	return nil
}

func runWorkDestroy(cmd *cobra.Command, args []string) error {
	workID := args[0]

	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Create WorkService for this operation
	svc := workpkg.NewWorkService(proj)

	// Check if work has uncompleted tasks (for interactive confirmation)
	tasks, err := proj.DB.GetWorkTasks(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work tasks: %w", err)
	}

	activeTaskCount := 0
	for _, task := range tasks {
		if task.Status != db.StatusCompleted && task.Status != db.StatusFailed {
			activeTaskCount++
		}
	}

	if activeTaskCount > 0 {
		fmt.Printf("Warning: Work %s has %d active task(s). Are you sure you want to destroy it? (y/N): ", workID, activeTaskCount)
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Destruction cancelled.")
			return nil
		}
	}

	// Destroy the work using WorkService
	if err := svc.DestroyWork(ctx, workID, os.Stdout); err != nil {
		return err
	}

	fmt.Printf("Destroyed work: %s\n", workID)
	return nil
}

// CreatePRTaskResult contains the result of creating a PR task.
type CreatePRTaskResult struct {
	TaskID string
	// PRExists is true if a PR already exists for this work
	PRExists bool
	PRURL    string
}

// CreatePRTask creates a PR task for a work unit.
// The work must be completed before a PR task can be created.
// Returns an error if the work is not completed, or PRExists=true if a PR already exists.
func CreatePRTask(ctx context.Context, proj *project.Project, workID string) (*CreatePRTaskResult, error) {
	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Check if work is completed
	if work.Status != db.StatusCompleted {
		return nil, fmt.Errorf("work %s is not completed (status: %s)", workID, work.Status)
	}

	// Check if PR already exists
	if work.PRURL != "" {
		return &CreatePRTaskResult{
			PRExists: true,
			PRURL:    work.PRURL,
		}, nil
	}

	// Generate task ID for PR creation
	prTaskNum, err := proj.DB.GetNextTaskNumber(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next task number for PR: %w", err)
	}
	prTaskID := fmt.Sprintf("%s.%d", workID, prTaskNum)

	// Create a PR creation task
	if err := proj.DB.CreateTask(ctx, prTaskID, "pr", []string{}, 0, workID); err != nil {
		return nil, fmt.Errorf("failed to create PR task: %w", err)
	}

	return &CreatePRTaskResult{
		TaskID: prTaskID,
	}, nil
}

func runWorkPR(cmd *cobra.Command, args []string) error {
	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		// Try to detect work from current directory
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	// Create PR task using the shared function
	result, err := CreatePRTask(ctx, proj, workID)
	if err != nil {
		return err
	}

	// Check if PR already exists
	if result.PRExists {
		fmt.Printf("PR already exists for work %s: %s\n", workID, result.PRURL)
		return nil
	}

	fmt.Printf("Created PR task: %s\n", result.TaskID)

	// Auto-run the PR task
	fmt.Printf("Running PR task...\n")
	agent := agents.NewAgent(proj.Config)
	runner := agents.NewRunner(agent)
	if err := processTask(proj, result.TaskID, runner); err != nil {
		return err
	}

	// Close the root issue now that PR has been created
	work, err := proj.DB.GetWork(ctx, workID)
	if err == nil && work != nil && work.RootIssueID != "" {
		fmt.Printf("Closing root issue %s...\n", work.RootIssueID)
		if err := beads.NewCLI(proj.BeadsPath()).Close(ctx, work.RootIssueID); err != nil {
			fmt.Printf("Warning: failed to close root issue %s: %v\n", work.RootIssueID, err)
		}
	}

	return nil
}

// CreateReviewTaskResult contains the result of creating a review task.
type CreateReviewTaskResult struct {
	TaskID string
}

// CreateReviewTask creates a review task for a work unit.
// Review tasks examine code changes for quality and security issues.
// Returns the task ID of the created review task.
func CreateReviewTask(ctx context.Context, proj *project.Project, workID string) (*CreateReviewTaskResult, error) {
	// Verify work exists
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	// Generate task ID for review
	reviewTaskNum, err := proj.DB.GetNextTaskNumber(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next task number for review: %w", err)
	}
	reviewTaskID := fmt.Sprintf("%s.%d", workID, reviewTaskNum)

	// Create the review task
	if err := proj.DB.CreateTask(ctx, reviewTaskID, "review", []string{}, 0, workID); err != nil {
		return nil, fmt.Errorf("failed to create review task: %w", err)
	}

	return &CreateReviewTaskResult{
		TaskID: reviewTaskID,
	}, nil
}

func runWorkReview(cmd *cobra.Command, args []string) error {
	// Find project
	ctx := GetContext()
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		// Try to detect work from current directory
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	// Run review-fix loop if --auto is set
	agent := agents.NewAgent(proj.Config)
	runner := agents.NewRunner(agent)
	maxIterations := proj.Config.Workflow.GetMaxReviewIterations()
	for iteration := 0; ; iteration++ {
		// Check max iterations
		if flagReviewAuto && iteration >= maxIterations {
			fmt.Printf("Warning: Maximum review iterations (%d) reached\n", maxIterations)
			break
		}

		// Create a review task using the shared function
		result, err := CreateReviewTask(ctx, proj, workID)
		if err != nil {
			return err
		}
		reviewTaskID := result.TaskID

		// Mark manual reviews (non-auto) so orchestrator skips automated workflow
		if !flagReviewAuto {
			if err := proj.DB.SetTaskMetadata(ctx, reviewTaskID, "auto_workflow", "false"); err != nil {
				return fmt.Errorf("failed to set manual review metadata: %w", err)
			}
		}

		fmt.Printf("Created review task: %s\n", reviewTaskID)

		// Run the review task
		fmt.Printf("Running review task...\n")
		if err := processTask(proj, reviewTaskID, runner); err != nil {
			return fmt.Errorf("review task failed: %w", err)
		}

		// If not in auto mode, we're done after one review
		if !flagReviewAuto {
			break
		}

		// Get the review task to check its creation timestamp
		reviewTask, err := proj.DB.GetTask(ctx, reviewTaskID)
		if err != nil {
			return fmt.Errorf("failed to get review task: %w", err)
		}

		// Check if the review created any issues under the root issue
		var beadsToFix []beads.Bead
		if work.RootIssueID != "" {
			// Get all children of the root issue
			rootChildrenIssues, err := proj.Beads.GetBeadWithChildren(ctx, work.RootIssueID)
			if err != nil {
				return fmt.Errorf("failed to get children of root issue %s: %w", work.RootIssueID, err)
			}

			// Filter to only ready beads that were created by this review task
			// (excluding the root issue itself)
			expectedExternalRef := fmt.Sprintf("review-%s", reviewTask.ID)
			for _, issue := range rootChildrenIssues {
				if issue.ID != work.RootIssueID &&
					beads.IsWorkableStatus(issue.Status) &&
					issue.ExternalRef == expectedExternalRef {
					beadsToFix = append(beadsToFix, issue)
				}
			}
		}

		if len(beadsToFix) == 0 {
			fmt.Println("Review passed - no issues found!")
			break
		}

		fmt.Printf("Review found %d issue(s) - creating fix tasks...\n", len(beadsToFix))

		// Create and run fix tasks for each bead
		for _, b := range beadsToFix {
			nextNum, err := proj.DB.GetNextTaskNumber(ctx, workID)
			if err != nil {
				return fmt.Errorf("failed to get next task number: %w", err)
			}
			taskID := fmt.Sprintf("%s.%d", workID, nextNum)

			if err := proj.DB.CreateTask(ctx, taskID, "implement", []string{b.ID}, 0, workID); err != nil {
				return fmt.Errorf("failed to create fix task: %w", err)
			}

			fmt.Printf("Created fix task %s for bead %s: %s\n", taskID, b.ID, b.Title)

			// Run the fix task
			if err := processTask(proj, taskID, runner); err != nil {
				return fmt.Errorf("fix task %s failed: %w", taskID, err)
			}
		}

		// Loop back for another review
	}

	return nil
}

// detectWorkFromDirectory tries to detect the work from the current directory.
// Returns empty string if not in a work directory.
func detectWorkFromDirectory(proj *project.Project) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're in a work subdirectory (format: /project/w-xxx/tree)
	rel, err := filepath.Rel(proj.Root, cwd)
	if err != nil {
		return "", nil
	}

	// Check if we're in a work directory (w-xxx or w-xxx/tree/...)
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) >= 1 && strings.HasPrefix(parts[0], "w-") {
		return parts[0], nil
	}

	return "", nil
}

// getCurrentWork tries to detect the work context from the current directory.
func getCurrentWork(proj *project.Project) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Check if we're in a subdirectory of the project
	if !strings.HasPrefix(cwd, proj.Root) {
		return "", fmt.Errorf("not in project directory")
	}

	// Get relative path from project root
	relPath, err := filepath.Rel(proj.Root, cwd)
	if err != nil {
		return "", err
	}

	// Check if we're in a work directory (w-xxx or w-xxx/tree/...)
	parts := strings.Split(relPath, string(os.PathSeparator))
	if len(parts) > 0 && strings.HasPrefix(parts[0], "w-") {
		return parts[0], nil
	}

	return "", fmt.Errorf("not in a work directory")
}

func runWorkConsole(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		// Try to detect work from current directory
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	// Ensure control plane is running (creates session if needed)
	if _, err := control.EnsureControlPlane(ctx, proj); err != nil {
		return fmt.Errorf("failed to ensure control plane: %w", err)
	}

	// Open console in the work's worktree
	orchestratorMgr := workpkg.NewOrchestratorManager(proj.DB)
	return orchestratorMgr.OpenConsole(ctx, workID, proj.Config.Project.Name, work.WorktreePath, work.Name, proj.Config.Hooks.Env, os.Stdout)
}

func runWorkClaude(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		// Try to detect work from current directory
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	// Get work details
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	// Ensure control plane is running (creates session if needed)
	if _, err := control.EnsureControlPlane(ctx, proj); err != nil {
		return fmt.Errorf("failed to ensure control plane: %w", err)
	}

	// Open Claude Code session in the work's worktree
	orchestratorMgr := workpkg.NewOrchestratorManager(proj.DB)
	return orchestratorMgr.OpenClaudeSession(ctx, workID, proj.Config.Project.Name, work.WorktreePath, work.Name, proj.Config.Hooks.Env, proj.Config, os.Stdout)
}

func runWorkRestart(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	if work.Status != db.StatusFailed {
		return fmt.Errorf("work %s is not in failed state (current status: %s)", workID, work.Status)
	}

	if err := proj.DB.RestartWork(ctx, workID); err != nil {
		return fmt.Errorf("failed to restart work: %w", err)
	}

	fmt.Printf("Work %s restarted. The orchestrator will resume processing.\n", workID)
	return nil
}

// printSessionCreatedNotification displays a prominent notification when a new zellij session is created.
func printSessionCreatedNotification(sessionName string) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("  Zellij session created: %s\n", sessionName)
	fmt.Println()
	fmt.Println("  To attach to the session, run:")
	fmt.Printf("    zellij attach %s\n", sessionName)
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()
}

func runWorkComplete(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	var workID string
	if len(args) > 0 {
		workID = args[0]
	} else {
		workID, err = getCurrentWork(proj)
		if err != nil {
			return fmt.Errorf("not in a work directory and no work ID specified")
		}
	}

	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return fmt.Errorf("work %s not found", workID)
	}

	if work.Status != db.StatusIdle {
		return fmt.Errorf("work %s is not in idle state (current status: %s)", workID, work.Status)
	}

	if err := proj.DB.CompleteWork(ctx, workID, work.PRURL); err != nil {
		return fmt.Errorf("failed to complete work: %w", err)
	}

	fmt.Printf("Work %s marked as completed.\n", workID)
	return nil
}
