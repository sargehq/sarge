package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/sargehq/sarge/internal/agentsetup"
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
  - Config update: ensures config2.toml has all available sections
  - Mise beans version: ensures the mise config has the correct beans version
  - Beans extension (pi): ensures the beans-prime extension is installed
  - Sarge extension (pi): ensures the sarge-complete extension is installed

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

	// Check 2: Mise beans version
	miseIssues, err := checkMiseBeansVersion(proj)
	if err != nil {
		return fmt.Errorf("mise beans version check failed: %w", err)
	}
	issues += miseIssues

	// Check 3: Beans extension for pi
	skillIssues, err := checkPiBeansSkill(proj)
	if err != nil {
		return fmt.Errorf("beans extension check failed: %w", err)
	}
	issues += skillIssues

	// Check 4: Pi sarge-complete extension
	extIssues, err := checkPiExtension(proj)
	if err != nil {
		return fmt.Errorf("pi extension check failed: %w", err)
	}
	issues += extIssues

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

func checkMiseBeansVersion(proj *project.Project) (int, error) {
	requiredVersion := mise.RequiredBeansVersion()
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

		currentVersion, _, err := mise.ReadBeansVersion(configPath)
		if err != nil {
			return 0, fmt.Errorf("failed to read beans version from %s: %w", configPath, err)
		}
		if currentVersion == "" {
			// No beans line in this config — skip
			continue
		}
		if currentVersion == requiredVersion {
			fmt.Printf("🔧 Mise beans (%s): up to date (%s)\n", d.label, currentVersion)
			continue
		}

		if doctorDryRun {
			fmt.Printf("🔧 Mise beans (%s): version mismatch\n", d.label)
			fmt.Printf("   current: %s, required: %s (would be updated)\n", currentVersion, requiredVersion)
			issues++
		} else {
			modified, err := mise.UpdateBeansVersion(configPath, requiredVersion)
			if err != nil {
				return 0, fmt.Errorf("failed to update beans version in %s: %w", configPath, err)
			}
			if modified {
				fmt.Printf("🔧 Mise beans (%s): updated %s → %s\n", d.label, currentVersion, requiredVersion)
				issues++
			}
		}
	}
	return issues, nil
}

func checkPiBeansSkill(proj *project.Project) (int, error) {
	repoDir := proj.MainRepoPath()
	if agentsetup.BeansPrimeExtensionInstalled(repoDir) {
		fmt.Println("🧩 Beans extension (pi): installed")
		return 0, nil
	}

	if doctorDryRun {
		fmt.Println("🧩 Beans extension (pi): missing")
		fmt.Println("   Would install .pi/extensions/beans-prime.ts in main repo")
		return 1, nil
	}

	if err := agentsetup.InstallBeansPrimeExtension(repoDir); err != nil {
		return 0, fmt.Errorf("failed to install pi beans-prime extension: %w", err)
	}
	fmt.Println("🧩 Beans extension (pi): installed .pi/extensions/beans-prime.ts in main repo")
	return 1, nil
}

func checkPiExtension(proj *project.Project) (int, error) {
	repoDir := proj.MainRepoPath()
	if agentsetup.PiExtensionInstalled(repoDir) {
		fmt.Println("🧩 Sarge extension (pi): installed")
		return 0, nil
	}

	if doctorDryRun {
		fmt.Println("🧩 Sarge extension (pi): missing")
		fmt.Println("   Would install .pi/extensions/sarge-complete.ts in main repo")
		return 1, nil
	}

	if err := agentsetup.InstallPiExtension(repoDir); err != nil {
		return 0, fmt.Errorf("failed to install pi sarge-complete extension: %w", err)
	}
	fmt.Println("🧩 Sarge extension (pi): installed .pi/extensions/sarge-complete.ts in main repo")
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
