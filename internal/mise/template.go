package mise

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed template/mise.tmpl
var miseTemplateText string

var miseTemplate = template.Must(template.New("mise").Parse(miseTemplateText))

// miseTemplateData holds the data used to render the mise config template.
type miseTemplateData struct {
	AgentType string
}

// GenerateConfig creates a .mise.toml file in the given directory with sarge's required tools.
// The agentType parameter selects which coding agent tool to include ("claude" or "pi").
// Returns nil if a mise config already exists (doesn't overwrite).
func GenerateConfig(dir string, agentType string) error {
	// Check if any mise config already exists
	if existingConfig := findConfigFile(dir); existingConfig != "" {
		return nil // Skip generation - config already exists
	}

	// Default to claude if not specified
	if agentType == "" {
		agentType = "claude"
	}

	data := miseTemplateData{
		AgentType: agentType,
	}

	var buf bytes.Buffer
	if err := miseTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render mise config template: %w", err)
	}

	configPath := filepath.Join(dir, ".mise.toml")
	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write mise config: %w", err)
	}

	return nil
}
