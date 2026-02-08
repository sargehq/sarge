package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	doctorDryRun bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check and fix project health",
	Long: `Run project health checks and apply fixes.

Checks include:
  - Config update: ensures config.toml has all available sections

Use --dry-run to preview changes without applying them.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := GetContext()
		proj, err := project.Find(ctx, "")
		if err != nil {
			return fmt.Errorf("not in a project directory: %w", err)
		}
		defer proj.Close()

		return runDoctor(proj)
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorDryRun, "dry-run", false, "show what would change without writing anything")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(proj *project.Project) error {
	fmt.Println("🩺 Running project health checks...")
	fmt.Println()

	issues := 0

	// Check 1: Config update
	configIssues, err := checkConfigUpdate(proj)
	if err != nil {
		return fmt.Errorf("config check failed: %w", err)
	}
	issues += configIssues

	// Summary
	fmt.Println()
	if issues == 0 {
		fmt.Println("✅ All checks passed. Project is healthy!")
	} else if doctorDryRun {
		fmt.Printf("⚠️  Found %d issue(s). Run without --dry-run to apply fixes.\n", issues)
	} else {
		fmt.Printf("🔧 Fixed %d issue(s).\n", issues)
	}

	return nil
}

func checkConfigUpdate(proj *project.Project) (int, error) {
	configPath := filepath.Join(proj.Root, project.ConfigDir, project.ConfigFile)

	cfg, err := project.LoadConfig(configPath)
	if err != nil {
		return 0, fmt.Errorf("failed to load config: %w", err)
	}

	if doctorDryRun {
		return checkConfigDryRun(configPath, cfg)
	}

	return checkConfigApply(configPath, cfg)
}

func checkConfigDryRun(configPath string, cfg *project.Config) (int, error) {
	// Use DryRunUpdateConfig to preview changes
	added, err := project.DryRunUpdateConfig(configPath, cfg)
	if err != nil {
		return 0, err
	}

	if len(added) == 0 {
		fmt.Println("📋 Config: up to date")
		return 0, nil
	}

	fmt.Println("📋 Config: new sections available")
	for _, name := range added {
		fmt.Printf("   + [%s] (would be added commented-out)\n", name)
	}
	return 1, nil
}

func checkConfigApply(configPath string, cfg *project.Config) (int, error) {
	added, err := project.UpdateConfig(configPath, cfg)
	if err != nil {
		return 0, err
	}

	if len(added) == 0 {
		fmt.Println("📋 Config: up to date")
		return 0, nil
	}

	fmt.Println("📋 Config: updated with new sections")
	for _, name := range added {
		fmt.Printf("   + [%s] (added commented-out)\n", name)
	}
	fmt.Printf("   Backup saved to %s.bak\n", configPath)
	return 1, nil
}
