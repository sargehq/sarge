package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var flagStatusProject string

var statusCmd = &cobra.Command{
	Use:   "status [bean-id]",
	Short: "Show bean tracking status",
	Long: `Show tracking status for beans.

With a bean ID: Show detailed status including zellij session/pane info.
Without ID: Show all beans currently processing with their session/pane.`,
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

	// If specific bean requested
	if len(args) > 0 {
		beanID := args[0]
		bean, err := proj.DB.GetBean(ctx, beanID)
		if err != nil {
			return fmt.Errorf("failed to get bean: %w", err)
		}
		if bean == nil {
			return fmt.Errorf("bean %s not found in tracking database", beanID)
		}

		printBeanDetails(bean)
		return nil
	}

	// Show all processing beans
	beanList, err := proj.DB.ListBeans(ctx, db.StatusProcessing)
	if err != nil {
		return fmt.Errorf("failed to list beans: %w", err)
	}

	if len(beanList) == 0 {
		fmt.Println("No beans currently processing")
		return nil
	}

	fmt.Printf("Currently processing %d bean(s):\n\n", len(beanList))
	for _, b := range beanList {
		printBeanDetails(b)
		fmt.Println()
	}

	return nil
}

func printBeanDetails(bean *db.TrackedBean) {
	fmt.Printf("ID:      %s\n", bean.ID)
	fmt.Printf("Title:   %s\n", bean.Title)
	fmt.Printf("Status:  %s\n", bean.Status)

	if bean.ZellijSession != "" {
		fmt.Printf("Session: %s\n", bean.ZellijSession)
	}
	if bean.ZellijPane != "" {
		fmt.Printf("Pane:    %s\n", bean.ZellijPane)
	}
	if bean.WorktreePath != "" {
		fmt.Printf("Worktree: %s\n", bean.WorktreePath)
	}
	if bean.PRURL != "" {
		fmt.Printf("PR:      %s\n", bean.PRURL)
	}
	if bean.ErrorMessage != "" {
		fmt.Printf("Error:   %s\n", bean.ErrorMessage)
	}
	if bean.StartedAt != nil {
		fmt.Printf("Started: %s\n", bean.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if bean.CompletedAt != nil {
		fmt.Printf("Done:    %s\n", bean.CompletedAt.Format("2006-01-02 15:04:05"))
	}
}
