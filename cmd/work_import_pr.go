package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/work"
	"github.com/spf13/cobra"
)

var workImportPRCmd = &cobra.Command{
	Use:   "import-pr <pr-url>",
	Short: "Import a PR into a work unit",
	Long: `Create a work unit from an existing GitHub pull request.

This command fetches the PR's branch, creates a worktree, and sets up the work
for further development or review. The PR's branch becomes the work's feature branch.

A bean is automatically created from the PR metadata to track the work in the beans system.

Examples:
  sarge work import-pr https://github.com/owner/repo/pull/123
  sarge work import-pr https://github.com/owner/repo/pull/123 --branch custom-branch-name`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkImportPR,
}

var flagImportPRBranch string

func init() {
	workImportPRCmd.Flags().StringVar(&flagImportPRBranch, "branch", "", "override the local branch name (default: use PR's branch name)")
	workCmd.AddCommand(workImportPRCmd)
}

func runWorkImportPR(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	prURL := args[0]

	// Create work service
	workSvc := work.NewWorkService(proj)

	// Fetch PR metadata first (user needs to see PR info)
	fmt.Printf("Fetching PR metadata from %s...\n", prURL)
	metadata, err := workSvc.GitHubClient.GetPRMetadata(ctx, prURL, "")
	if err != nil {
		return fmt.Errorf("failed to fetch PR metadata: %w", err)
	}

	fmt.Printf("PR #%d: %s\n", metadata.Number, metadata.Title)
	fmt.Printf("Author: %s\n", metadata.Author)
	fmt.Printf("State: %s\n", metadata.State)
	fmt.Printf("Branch: %s -> %s\n", metadata.HeadRefName, metadata.BaseRefName)

	// Check if PR is still open
	if metadata.State != "OPEN" {
		fmt.Printf("Warning: PR is %s\n", metadata.State)
	}

	// Determine branch name
	branchName := flagImportPRBranch
	if branchName == "" {
		branchName = metadata.HeadRefName
	}

	// Create a bean from PR metadata (user needs feedback on bean creation)
	fmt.Printf("\nCreating bean from PR metadata...\n")
	beanResult, err := workSvc.CreateBeanFromPR(ctx, metadata, &work.CreateBeanOptions{
		BeansDir:     proj.BeansPath(),
		SkipIfExists: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create bean: %w", err)
	}
	if beanResult.Created {
		fmt.Printf("Created bean: %s\n", beanResult.BeanID)
	} else {
		fmt.Printf("Bean already exists: %s (%s)\n", beanResult.BeanID, beanResult.SkipReason)
	}
	rootIssueID := beanResult.BeanID

	// Schedule PR import via control plane (handles worktree, git, mise)
	fmt.Printf("\nScheduling PR import...\n")
	result, err := workSvc.ImportPRAsync(ctx, work.ImportPRAsyncOptions{
		PRURL:       prURL,
		BranchName:  branchName,
		RootIssueID: rootIssueID,
	})
	if err != nil {
		return fmt.Errorf("failed to schedule PR import: %w", err)
	}

	fmt.Printf("\nCreated work: %s\n", result.WorkID)
	if result.WorkerName != "" {
		fmt.Printf("Worker: %s\n", result.WorkerName)
	}
	fmt.Printf("Branch: %s\n", result.BranchName)
	fmt.Printf("PR URL: %s\n", prURL)
	fmt.Printf("\nWorktree setup is in progress via control plane.\n")

	// Ensure zellij session and control plane are running
	sessionResult, err := control.EnsureControlPlane(ctx, proj)
	if err != nil {
		fmt.Printf("Warning: failed to ensure control plane: %v\n", err)
	} else if sessionResult.SessionCreated {
		printSessionCreatedNotification(sessionResult.SessionName)
	}

	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  cd %s\n", result.WorkID)
	fmt.Printf("  sarge run               # Execute tasks (after worktree is ready)\n")

	return nil
}
