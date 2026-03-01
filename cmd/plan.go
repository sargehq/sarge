package cmd

import (
	"fmt"
	"os"

	"log/slog"

	"github.com/sargehq/sarge/internal/agents"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/git"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan <bean-id>",
	Short: "Launch agent for planning a specific issue",
	Long: `Plan launches the coding agent for planning work on a specific issue.

This command is typically invoked by the TUI's Plan mode, which creates a
session for each issue and runs 'sarge plan <id>' within it.

The agent can then be used to:
- Investigate the issue (beans show <id>)
- Break down the issue into subtasks
- Plan implementation strategies
- Create related issues

Each issue gets its own dedicated planning session in a separate tab.`,
	Args: cobra.ExactArgs(1),
	RunE: runPlan,
}

func init() {
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}
	defer proj.Close()

	beanID := args[0]
	sessionName := fmt.Sprintf("sarge-%s", proj.Config.Project.Name)
	tabName := db.TabNameForBean(beanID)

	// Apply hooks.env to current process - inherited by child processes
	applyHooksEnv(proj.Config.Hooks.Env)

	// Set BEANS_PATH so beans commands work in agent sessions
	_ = os.Setenv("BEANS_PATH", proj.BeansPath())

	// Register the plan session in the database
	if err := proj.DB.RegisterPlanSession(ctx, beanID, sessionName, tabName, os.Getpid()); err != nil {
		return fmt.Errorf("failed to register plan session: %w", err)
	}
	defer func() {
		// Unregister when done
		_ = proj.DB.UnregisterPlanSession(ctx, beanID)
	}()

	mainRepoPath := proj.MainRepoPath()

	// Fetch the base branch from origin before planning so the agent works
	// against up-to-date refs. Failure is non-fatal to allow offline work.
	baseBranch := proj.Config.Repo.GetBaseBranch()
	if err := git.NewOperations().FetchBranch(ctx, mainRepoPath, baseBranch); err != nil {
		slog.Warn("failed to fetch base branch before plan session", "branch", baseBranch, "error", err)
	}

	// Launch agent interactively with plan params
	agent, err := agents.NewAgent(proj.Config)
	if err != nil {
		return err
	}
	return agent.RunInteractive(ctx, types.TaskParams{Type: types.TaskTypePlan, BeanID: beanID, BeansPath: proj.BeansPath()}, mainRepoPath, os.Stdin, os.Stdout, os.Stderr, proj.Config)
}
