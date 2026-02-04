package mise

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed template/mise.tmpl
var miseTemplate string

// GenerateConfig creates a .mise.toml file in the given directory with sarge's required tools.
// Returns nil if a mise config already exists (doesn't overwrite).
func GenerateConfig(dir string) error {
	// Check if any mise config already exists
	if existingConfig := findConfigFile(dir); existingConfig != "" {
		return nil // Skip generation - config already exists
	}

	configPath := filepath.Join(dir, ".mise.toml")

	if err := os.WriteFile(configPath, []byte(miseTemplate), 0600); err != nil {
		return fmt.Errorf("failed to write mise config: %w", err)
	}

	return nil
}
