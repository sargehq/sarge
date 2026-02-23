package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	flagListStatus  string
	flagListProject string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List tracked beads",
	Long:  `List all beads in the tracking database with optional status filter.`,
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringVarP(&flagListStatus, "status", "s", "", "filter by status (pending, processing, completed, failed)")
	listCmd.Flags().StringVar(&flagListProject, "project", "", "project directory (default: auto-detect from cwd)")
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	proj, err := project.Find(ctx, flagListProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	beadList, err := proj.DB.ListBeans(ctx, flagListStatus)
	if err != nil {
		return fmt.Errorf("failed to list beads: %w", err)
	}

	if len(beadList) == 0 {
		if flagListStatus != "" {
			fmt.Printf("No beads with status '%s'\n", flagListStatus)
		} else {
			fmt.Println("No beads tracked")
		}
		return nil
	}

	fmt.Printf("%-12s %-12s %-40s %s\n", "ID", "STATUS", "TITLE", "PR")
	fmt.Printf("%-12s %-12s %-40s %s\n", "---", "------", "-----", "--")

	for _, bead := range beadList {
		title := bead.Title
		if len(title) > 38 {
			title = title[:35] + "..."
		}
		prURL := bead.PRURL
		if prURL == "" {
			prURL = "-"
		}
		fmt.Printf("%-12s %-12s %-40s %s\n", bead.ID, bead.Status, title, prURL)
	}

	return nil
}
