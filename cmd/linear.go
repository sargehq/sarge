package cmd

import (
	"fmt"
	"os"

	"github.com/sargehq/sarge/internal/linear"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var linearCmd = &cobra.Command{
	Use:   "linear",
	Short: "Linear integration commands",
	Long:  "Commands for importing and syncing issues from Linear",
}

var linearImportCmd = &cobra.Command{
	Use:   "import <issue-id-or-url>...",
	Short: "Import issues from Linear into beans",
	Long: `Import one or more Linear issues into the beans issue tracker.

Examples:
  # Import single issue by ID
  sarge linear import ENG-123

  # Import by URL
  sarge linear import https://linear.app/company/issue/ENG-123/title

  # Import multiple issues
  sarge linear import ENG-123 ENG-124 ENG-125

  # Import with dependencies
  sarge linear import ENG-123 --create-deps

  # Update existing bean from Linear
  sarge linear import ENG-123 --update

  # Dry run (preview without creating)
  sarge linear import ENG-123 --dry-run

Authentication:
  The Linear API key can be provided via (in order of precedence):
  1. --api-key flag
  2. [linear] api_key in .sarge/config.toml

Environment Variables:
  BEANS_PATH         Beans directory (default: auto-detect)
`,
	Args: cobra.MinimumNArgs(1),
	RunE: runLinearImport,
}

var (
	linearAPIKey       string
	linearBeansDir     string
	linearDryRun       bool
	linearUpdateExist  bool
	linearCreateDeps   bool
	linearMaxDepDepth  int
	linearStatusFilter string
	linearPriorityFilt string
	linearAssigneeFilt string
)

func init() {
	rootCmd.AddCommand(linearCmd)
	linearCmd.AddCommand(linearImportCmd)

	// Import command flags
	linearImportCmd.Flags().StringVar(&linearAPIKey, "api-key", "", "Linear API key (or set [linear] api_key in config.toml)")
	linearImportCmd.Flags().StringVar(&linearBeansDir, "beans-dir", "", "Beans directory (default: auto-detect)")
	linearImportCmd.Flags().BoolVar(&linearDryRun, "dry-run", false, "Preview import without creating beans")
	linearImportCmd.Flags().BoolVar(&linearUpdateExist, "update", false, "Update existing beans if already imported")
	linearImportCmd.Flags().BoolVar(&linearCreateDeps, "create-deps", false, "Import blocking issues as dependencies")
	linearImportCmd.Flags().IntVar(&linearMaxDepDepth, "max-dep-depth", 1, "Maximum dependency depth to import")
	linearImportCmd.Flags().StringVar(&linearStatusFilter, "status-filter", "", "Only import issues with this status")
	linearImportCmd.Flags().StringVar(&linearPriorityFilt, "priority-filter", "", "Only import issues with this priority (P0-P4)")
	linearImportCmd.Flags().StringVar(&linearAssigneeFilt, "assignee-filter", "", "Only import issues assigned to this user")
}

func runLinearImport(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Get API key from flag or config
	apiKey := linearAPIKey
	if apiKey == "" {
		// Try to get from project config
		if proj, err := project.Find(ctx, ""); err == nil && proj.Config != nil {
			apiKey = proj.Config.Linear.APIKey
		}
	}
	if apiKey == "" {
		return fmt.Errorf("linear API key is required (set via --api-key flag or [linear] api_key in config.toml)")
	}

	// Get beans directory
	beansDir := linearBeansDir
	if beansDir == "" {
		beansDir = os.Getenv("BEANS_PATH")
	}
	if beansDir == "" {
		// Auto-detect: look for .beans directory in current or parent directories
		beansDir = "."
	}

	// Create fetcher
	fetcher, err := linear.NewFetcher(apiKey, beansDir)
	if err != nil {
		return fmt.Errorf("failed to create Linear fetcher: %w", err)
	}

	// Prepare import options
	opts := &linear.ImportOptions{
		DryRun:         linearDryRun,
		UpdateExisting: linearUpdateExist,
		CreateDeps:     linearCreateDeps,
		MaxDepDepth:    linearMaxDepDepth,
		StatusFilter:   linearStatusFilter,
		PriorityFilter: linearPriorityFilt,
		AssigneeFilter: linearAssigneeFilt,
	}

	// Import issues
	if len(args) == 1 {
		// Single issue import
		result, err := fetcher.FetchAndImport(ctx, args[0], opts)
		if err != nil {
			return fmt.Errorf("import failed: %w", err)
		}
		printImportResult(result)
	} else {
		// Batch import
		results, err := fetcher.FetchBatch(ctx, args, opts)
		if err != nil {
			return fmt.Errorf("batch import failed: %w", err)
		}
		printBatchResults(results)
	}

	return nil
}

func printImportResult(result *linear.ImportResult) {
	if result.Error != nil {
		fmt.Fprintf(os.Stderr, "✗ Error: %v\n", result.Error)
		return
	}

	if result.SkipReason != "" {
		fmt.Printf("○ %s: %s\n", result.LinearID, result.SkipReason)
		if result.BeanID != "" {
			fmt.Printf("  Bean: %s\n", result.BeanID)
		}
		return
	}

	if result.Success {
		fmt.Printf("✓ Imported %s -> %s\n", result.LinearID, result.BeanID)
		if result.LinearURL != "" {
			fmt.Printf("  URL: %s\n", result.LinearURL)
		}
	}
}

func printBatchResults(results []*linear.ImportResult) {
	successCount := 0
	skipCount := 0
	errorCount := 0

	for _, result := range results {
		if result.Error != nil {
			errorCount++
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", result.LinearID, result.Error)
		} else if result.SkipReason != "" {
			skipCount++
			fmt.Printf("○ %s: %s\n", result.LinearID, result.SkipReason)
		} else if result.Success {
			successCount++
			fmt.Printf("✓ %s -> %s\n", result.LinearID, result.BeanID)
		}
	}

	fmt.Printf("\nSummary: %d imported, %d skipped, %d failed\n",
		successCount, skipCount, errorCount)
}
