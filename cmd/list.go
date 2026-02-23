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
	Short: "List tracked beans",
	Long:  `List all beans in the tracking database with optional status filter.`,
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

	beanList, err := proj.DB.ListBeans(ctx, flagListStatus)
	if err != nil {
		return fmt.Errorf("failed to list beans: %w", err)
	}

	if len(beanList) == 0 {
		if flagListStatus != "" {
			fmt.Printf("No beans with status '%s'\n", flagListStatus)
		} else {
			fmt.Println("No beans tracked")
		}
		return nil
	}

	fmt.Printf("%-12s %-12s %-40s %s\n", "ID", "STATUS", "TITLE", "PR")
	fmt.Printf("%-12s %-12s %-40s %s\n", "---", "------", "-----", "--")

	for _, bean := range beanList {
		title := bean.Title
		if len(title) > 38 {
			title = title[:35] + "..."
		}
		prURL := bean.PRURL
		if prURL == "" {
			prURL = "-"
		}
		fmt.Printf("%-12s %-12s %-40s %s\n", bean.ID, bean.Status, title, prURL)
	}

	return nil
}
