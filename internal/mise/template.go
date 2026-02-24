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
// Each tool has an "active" flag: true = uncommented, false = commented out.
type miseTemplateData struct {
	AgentType   string // "claude" or "pi"
	AgentActive bool   // true = uncommented, false = commented out
	GHActive    bool   // true = uncommented, false = commented out
	ZellijActive bool  // true = uncommented, false = commented out
}

// ToolSelections holds user choices about which tools to include in mise config.
type ToolSelections struct {
	AgentType       string // "claude" or "pi" — which agent was chosen
	AgentInMise     bool   // whether to activate (uncomment) the agent in mise
	IncludeGH       bool   // whether to activate (uncomment) gh in mise
	IncludeZellij   bool   // whether to activate (uncomment) zellij in mise
	MultiplexerType string // "zellij" (default) or "zmx" — which multiplexer to use
}

// DefaultToolSelections returns the default tool selections (gh and zellij included, agent not in mise).
func DefaultToolSelections() ToolSelections {
	return ToolSelections{
		AgentType:     "claude",
		AgentInMise:   false,
		IncludeGH:     true,
		IncludeZellij: true,
	}
}

// toTemplateData converts ToolSelections to miseTemplateData for rendering.
func (s ToolSelections) toTemplateData() miseTemplateData {
	agentType := s.AgentType
	if agentType == "" {
		agentType = "claude"
	}
	return miseTemplateData{
		AgentType:    agentType,
		AgentActive:  s.AgentInMise,
		GHActive:     s.IncludeGH,
		ZellijActive: s.IncludeZellij,
	}
}

// GenerateConfig creates a .mise.toml file in the given directory with sarge's required tools.
// The agentType parameter selects which coding agent tool to include ("claude" or "pi"; defaults to "claude" if empty).
// Returns nil if a mise config already exists (doesn't overwrite).
func GenerateConfig(dir string, agentType string) error {
	selections := DefaultToolSelections()
	selections.AgentType = agentType
	return GenerateConfigWithSelections(dir, selections)
}

// RegenerateConfigWithSelections force-overwrites .mise.toml with the given tool selections,
// regardless of whether a mise config already exists.
// If an existing .mise.toml is present, a backup is created at .mise.toml.bak before overwriting.
func RegenerateConfigWithSelections(dir string, selections ToolSelections) error {
	data := selections.toTemplateData()

	var buf bytes.Buffer
	if err := miseTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("failed to render mise config template: %w", err)
	}

	configPath := filepath.Join(dir, ".mise.toml")

	// Create a backup of any existing config before overwriting.
	if existing, err := os.ReadFile(configPath); err == nil { //nolint:gosec // path is constructed from caller-supplied dir + hardcoded filename
		backupPath := configPath + ".bak"
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			return fmt.Errorf("failed to create backup of mise config: %w", err)
		}
	}

	if err := os.WriteFile(configPath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("failed to write mise config: %w", err)
	}

	return nil
}

// GenerateConfigWithSelections creates a .mise.toml file with the specified tool selections.
// Returns nil if a mise config already exists (doesn't overwrite).
func GenerateConfigWithSelections(dir string, selections ToolSelections) error {
	// Check if any mise config already exists
	if existingConfig := findConfigFile(dir); existingConfig != "" {
		return nil // Skip generation - config already exists
	}

	data := selections.toTemplateData()

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
