package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/sargehq/sarge/internal/mise"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Reconfigure the current project",
	Long: `Reconfigure the current project interactively.

Re-runs tool selection, regenerates .mise.toml, runs mise install,
and updates config.toml with new settings.`,
	RunE: runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// 1. Find the project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// 2. Re-run tool selection interactively
	selections := promptToolSelections()

	// 3. Regenerate .mise.toml (force-overwrite)
	fmt.Println("Regenerating .mise.toml...")
	if err := mise.RegenerateConfigWithSelections(proj.Root, selections); err != nil {
		return fmt.Errorf("failed to regenerate mise config: %w", err)
	}

	// 4. Run mise install (trust + install + setup task if present)
	fmt.Println("Running mise install...")
	if err := mise.Initialize(proj.Root); err != nil {
		return fmt.Errorf("failed to run mise install: %w", err)
	}

	// 5. Merge new config sections into config.toml
	configPath := filepath.Join(proj.Root, project.ConfigDir, project.ConfigFile)
	fmt.Println("Updating config.toml...")
	added, err := project.UpdateConfig(configPath, proj.Config)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}
	if len(added) > 0 {
		fmt.Printf("  Added new sections: %v\n", added)
	}

	// 6. Print confirmation summary
	fmt.Println()
	fmt.Println("✓ Project reconfigured successfully!")
	fmt.Printf("  Config:      %s\n", configPath)

	return nil
}
