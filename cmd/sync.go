package cmd

import (
	"fmt"
	"strings"

	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull from upstream in all repositories",
	Long: `Synchronize the local git state with upstream by performing git pull
in each repository (main and all worktrees).

This helps keep all work units up-to-date with the latest changes from the remote repository.`,
	Args: cobra.NoArgs,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVar(&flagProject, "project", "", "project directory (default: auto-detect from cwd)")
}

func runSync(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	proj, err := project.Find(ctx, flagProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}

	fmt.Printf("Syncing project: %s\n", proj.Config.Project.Name)

	// Get all worktrees
	wtOps := worktree.NewOperations()
	gitOps := git.NewOperations()
	worktrees, err := wtOps.List(ctx, proj.MainRepoPath())
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Track results
	successCount := 0
	failCount := 0

	// Pull in each worktree
	for _, wt := range worktrees {
		// Skip beans internal worktrees (managed by beans system, not sarge)
		if strings.Contains(wt.Path, ".git/beans-worktrees") {
			continue
		}

		branchInfo := wt.Branch
		if branchInfo == "" {
			branchInfo = "(detached)"
		}

		fmt.Printf("  Pulling %s [%s]... ", wt.Path, branchInfo)

		if err := gitOps.Pull(ctx, wt.Path); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			failCount++
		} else {
			fmt.Printf("OK\n")
			successCount++
		}
	}

	// Print summary
	fmt.Printf("\nSync complete: %d succeeded, %d failed\n", successCount, failCount)

	if failCount > 0 {
		return fmt.Errorf("%d repository(s) failed to sync", failCount)
	}

	return nil
}
