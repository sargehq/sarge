package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var flagStatusProject string

var statusCmd = &cobra.Command{
	Use:   "status [bead-id]",
	Short: "Show bead tracking status",
	Long: `Show tracking status for beads.

With a bead ID: Show detailed status including zellij session/pane info.
Without ID: Show all beads currently processing with their session/pane.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&flagStatusProject, "project", "", "project directory (default: auto-detect from cwd)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	proj, err := project.Find(ctx, flagStatusProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// If specific bead requested
	if len(args) > 0 {
		beanID := args[0]
		bead, err := proj.DB.GetBean(ctx, beanID)
		if err != nil {
			return fmt.Errorf("failed to get bead: %w", err)
		}
		if bead == nil {
			return fmt.Errorf("bead %s not found in tracking database", beanID)
		}

		printBeadDetails(bead)
		return nil
	}

	// Show all processing beads
	beadList, err := proj.DB.ListBeans(ctx, db.StatusProcessing)
	if err != nil {
		return fmt.Errorf("failed to list beads: %w", err)
	}

	if len(beadList) == 0 {
		fmt.Println("No beads currently processing")
		return nil
	}

	fmt.Printf("Currently processing %d bead(s):\n\n", len(beadList))
	for _, b := range beadList {
		printBeadDetails(b)
		fmt.Println()
	}

	return nil
}

func printBeadDetails(bead *db.TrackedBean) {
	fmt.Printf("ID:      %s\n", bead.ID)
	fmt.Printf("Title:   %s\n", bead.Title)
	fmt.Printf("Status:  %s\n", bead.Status)

	if bead.ZellijSession != "" {
		fmt.Printf("Session: %s\n", bead.ZellijSession)
	}
	if bead.ZellijPane != "" {
		fmt.Printf("Pane:    %s\n", bead.ZellijPane)
	}
	if bead.WorktreePath != "" {
		fmt.Printf("Worktree: %s\n", bead.WorktreePath)
	}
	if bead.PRURL != "" {
		fmt.Printf("PR:      %s\n", bead.PRURL)
	}
	if bead.ErrorMessage != "" {
		fmt.Printf("Error:   %s\n", bead.ErrorMessage)
	}
	if bead.StartedAt != nil {
		fmt.Printf("Started: %s\n", bead.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if bead.CompletedAt != nil {
		fmt.Printf("Done:    %s\n", bead.CompletedAt.Format("2006-01-02 15:04:05"))
	}
}
