package cmd

import (
	"fmt"
	"os"

	"github.com/sargehq/sarge/internal/claude"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan <bead-id>",
	Short: "Launch Claude for planning a specific issue",
	Long: `Plan launches Claude Code for planning work on a specific issue.

This command is typically invoked by the TUI's Plan mode, which creates a
zellij tab for each issue and runs 'co plan <id>' within it.

Claude can then be used to:
- Investigate the issue (bd show <id>)
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

	beadID := args[0]
	zellijSession := fmt.Sprintf("co-%s", proj.Config.Project.Name)
	tabName := db.TabNameForBead(beadID)

	// Apply hooks.env to current process - inherited by child processes (Claude)
	applyHooksEnv(proj.Config.Hooks.Env)

	// Set BEADS_DIR so bd commands work in Claude
	_ = os.Setenv("BEADS_DIR", proj.BeadsPath())

	// Register the plan session in the database
	if err := proj.DB.RegisterPlanSession(ctx, beadID, zellijSession, tabName, os.Getpid()); err != nil {
		return fmt.Errorf("failed to register plan session: %w", err)
	}
	defer func() {
		// Unregister when done
		_ = proj.DB.UnregisterPlanSession(ctx, beadID)
	}()

	mainRepoPath := proj.MainRepoPath()

	// Launch Claude with the plan prompt
	if err := claude.RunPlanSession(ctx, beadID, mainRepoPath, os.Stdin, os.Stdout, os.Stderr, proj.Config); err != nil {
		return err
	}

	return nil
}
