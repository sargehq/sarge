package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/sargehq/sarge/internal/mise"
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
  - Mise beads version: ensures the mise config has the correct beads version

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

	// Check 2: Mise beads version
	miseIssues, err := checkMiseBeadsVersion(proj)
	if err != nil {
		return fmt.Errorf("mise beads version check failed: %w", err)
	}
	issues += miseIssues

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

func checkMiseBeadsVersion(proj *project.Project) (int, error) {
	requiredVersion := mise.RequiredBeadsVersion()
	if requiredVersion == "" {
		// No required version found in template — skip check
		return 0, nil
	}

	// Check both the project root and main repo path for mise configs
	dirs := []struct {
		label string
		path  string
	}{
		{"project root", proj.Root},
		{"main repo", proj.MainRepoPath()},
	}

	issues := 0
	for _, d := range dirs {
		configFile := mise.FindConfigFile(d.path)
		if configFile == "" {
			continue
		}
		configPath := filepath.Join(d.path, configFile)

		currentVersion, _, err := mise.ReadBeadsVersion(configPath)
		if err != nil {
			return 0, fmt.Errorf("failed to read beads version from %s: %w", configPath, err)
		}
		if currentVersion == "" {
			// No beads line in this config — skip
			continue
		}
		if currentVersion == requiredVersion {
			fmt.Printf("🔧 Mise beads (%s): up to date (%s)\n", d.label, currentVersion)
			continue
		}

		if doctorDryRun {
			fmt.Printf("🔧 Mise beads (%s): version mismatch\n", d.label)
			fmt.Printf("   current: %s, required: %s (would be updated)\n", currentVersion, requiredVersion)
			issues++
		} else {
			modified, err := mise.UpdateBeadsVersion(configPath, requiredVersion)
			if err != nil {
				return 0, fmt.Errorf("failed to update beads version in %s: %w", configPath, err)
			}
			if modified {
				fmt.Printf("🔧 Mise beads (%s): updated %s → %s\n", d.label, currentVersion, requiredVersion)
				issues++
			}
		}
	}
	return issues, nil
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
