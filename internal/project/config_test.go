package project

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
)

func TestGeneratedConfigIsValidTOML(t *testing.T) {
	cfg := &Config{
		Project: ProjectConfig{
			Name:      "test-project",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/example/repo",
			Path:   "main",
		},
	}
	content := cfg.GenerateDocumentedConfig()

	// Try to parse the generated content as valid TOML
	var parsed map[string]interface{}
	_, err := toml.Decode(content, &parsed)
	require.NoError(t, err, "Generated config is not valid TOML:\n%s", content)

	// Verify key fields are present
	project := parsed["project"].(map[string]interface{})
	require.Equal(t, "test-project", project["name"])

	repo := parsed["repo"].(map[string]interface{})
	require.Equal(t, "github", repo["type"])
}

func TestGeneratedConfigRoundTrip(t *testing.T) {
	original := &Config{
		Project: ProjectConfig{
			Name:      "test-project",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/example/repo",
			Path:   "main",
		},
	}

	content := original.GenerateDocumentedConfig()

	// Parse back into a Config struct
	var loaded Config
	_, err := toml.Decode(content, &loaded)
	require.NoError(t, err)

	// Verify fields match
	require.Equal(t, original.Project.Name, loaded.Project.Name)
	require.True(t, loaded.Project.CreatedAt.Equal(original.Project.CreatedAt))
	require.Equal(t, original.Repo.Type, loaded.Repo.Type)
	require.Equal(t, original.Repo.Source, loaded.Repo.Source)
	require.Equal(t, original.Repo.Path, loaded.Repo.Path)
}

func TestGeneratedConfigWithSpecialCharacters(t *testing.T) {
	// Test with special characters that could break TOML
	cfg := &Config{
		Project: ProjectConfig{
			Name:      "test-project-with\"quotes",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/user/repo with spaces",
			Path:   "main",
		},
	}

	content := cfg.GenerateDocumentedConfig()

	// This should parse successfully even with special characters
	var parsed Config
	_, err := toml.Decode(content, &parsed)
	require.NoError(t, err, "Failed to parse config with special characters:\n%s", content)

	// Verify values
	require.Equal(t, cfg.Project.Name, parsed.Project.Name)
	require.Equal(t, cfg.Repo.Source, parsed.Repo.Source)
}

func TestShouldSkipPermissionsDefault(t *testing.T) {
	// When Claude config is not specified in TOML, ShouldSkipPermissions should default to true
	tomlContent := `
[project]
  name = "test"
`
	var cfg Config
	_, err := toml.Decode(tomlContent, &cfg)
	require.NoError(t, err)

	require.True(t, cfg.Claude.ShouldSkipPermissions(), "Expected ShouldSkipPermissions() to return true by default")
}

func TestShouldSkipPermissionsExplicitFalse(t *testing.T) {
	// When explicitly set to false, ShouldSkipPermissions should return false
	tomlContent := `
[project]
  name = "test"

[claude]
  skip_permissions = false
`
	var cfg Config
	_, err := toml.Decode(tomlContent, &cfg)
	require.NoError(t, err)

	require.False(t, cfg.Claude.ShouldSkipPermissions(), "Expected ShouldSkipPermissions() to return false when explicitly set")
}

func TestGeneratedConfigWithUTF8(t *testing.T) {
	// Test with UTF-8 characters
	cfg := &Config{
		Project: ProjectConfig{
			Name:      "проект-名前-مشروع",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/日本/リポジトリ",
			Path:   "main",
		},
	}

	content := cfg.GenerateDocumentedConfig()

	// This should parse successfully
	var parsed Config
	_, err := toml.Decode(content, &parsed)
	require.NoError(t, err, "Failed to parse config with UTF-8")

	// Verify values
	require.Equal(t, cfg.Project.Name, parsed.Project.Name)
}

func TestLogParserConfig_ShouldUseAgent(t *testing.T) {
	tests := []struct {
		name     string
		config   LogParserConfig
		expected bool
	}{
		{
			name:     "Default (false)",
			config:   LogParserConfig{},
			expected: false,
		},
		{
			name:     "Explicitly enabled",
			config:   LogParserConfig{UseAgent: true},
			expected: true,
		},
		{
			name:     "Explicitly disabled",
			config:   LogParserConfig{UseAgent: false},
			expected: false,
		},
		{
			name:     "Enabled with model",
			config:   LogParserConfig{UseAgent: true, Model: "sonnet"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ShouldUseAgent()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogParserConfig_GetModel(t *testing.T) {
	tests := []struct {
		name     string
		config   LogParserConfig
		expected string
	}{
		{
			name:     "Default (empty) returns empty",
			config:   LogParserConfig{},
			expected: "",
		},
		{
			name:     "Haiku model",
			config:   LogParserConfig{Model: "haiku"},
			expected: "haiku",
		},
		{
			name:     "Sonnet model",
			config:   LogParserConfig{Model: "sonnet"},
			expected: "sonnet",
		},
		{
			name:     "Opus model",
			config:   LogParserConfig{Model: "opus"},
			expected: "opus",
		},
		{
			name:     "Custom model passed through",
			config:   LogParserConfig{Model: "gpt-4o-mini"},
			expected: "gpt-4o-mini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetModel()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestLogParserConfigFromTOML(t *testing.T) {
	tests := []struct {
		name         string
		tomlContent  string
		wantUseAgent bool
		wantModel    string
	}{
		{
			name: "Not specified defaults",
			tomlContent: `
[project]
name = "test"
`,
			wantUseAgent: false,
			wantModel:    "",
		},
		{
			name: "Enabled with haiku",
			tomlContent: `
[project]
name = "test"

[log_parser]
use_agent = true
model = "haiku"
`,
			wantUseAgent: true,
			wantModel:    "haiku",
		},
		{
			name: "Enabled with sonnet",
			tomlContent: `
[project]
name = "test"

[log_parser]
use_agent = true
model = "sonnet"
`,
			wantUseAgent: true,
			wantModel:    "sonnet",
		},
		{
			name: "Enabled without model (defaults to empty)",
			tomlContent: `
[project]
name = "test"

[log_parser]
use_agent = true
`,
			wantUseAgent: true,
			wantModel:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			_, err := toml.Decode(tt.tomlContent, &cfg)
			require.NoError(t, err)

			require.Equal(t, tt.wantUseAgent, cfg.LogParser.ShouldUseAgent())
			require.Equal(t, tt.wantModel, cfg.LogParser.GetModel())
		})
	}
}

func TestIDEConfig_GetIDECommand(t *testing.T) {
	tests := []struct {
		name      string
		command   string
		editorEnv string
		expected  string
	}{
		{
			name:      "returns Command when set",
			command:   "code",
			editorEnv: "vim",
			expected:  "code",
		},
		{
			name:      "falls back to EDITOR env var when Command is empty",
			command:   "",
			editorEnv: "nvim",
			expected:  "nvim",
		},
		{
			name:      "returns empty string when neither is configured",
			command:   "",
			editorEnv: "",
			expected:  "",
		},
		{
			name:      "Command takes precedence over EDITOR",
			command:   "cursor",
			editorEnv: "emacs",
			expected:  "cursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore EDITOR environment variable
			origEditor := os.Getenv("EDITOR")
			defer func() {
				if origEditor == "" {
					os.Unsetenv("EDITOR")
				} else {
					os.Setenv("EDITOR", origEditor)
				}
			}()

			// Set test environment
			if tt.editorEnv == "" {
				os.Unsetenv("EDITOR")
			} else {
				os.Setenv("EDITOR", tt.editorEnv)
			}

			cfg := IDEConfig{Command: tt.command}
			result := cfg.GetIDECommand()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIDEConfigFromTOML(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantCommand string
		wantArgs    []string
	}{
		{
			name: "Not specified defaults to empty",
			tomlContent: `
[project]
name = "test"
`,
			wantCommand: "",
			wantArgs:    nil,
		},
		{
			name: "Command only",
			tomlContent: `
[project]
name = "test"

[ide]
command = "code"
`,
			wantCommand: "code",
			wantArgs:    nil,
		},
		{
			name: "Command with args",
			tomlContent: `
[project]
name = "test"

[ide]
command = "cursor"
args = ["--new-window", "--wait"]
`,
			wantCommand: "cursor",
			wantArgs:    []string{"--new-window", "--wait"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			_, err := toml.Decode(tt.tomlContent, &cfg)
			require.NoError(t, err)

			require.Equal(t, tt.wantCommand, cfg.IDE.Command)
			require.Equal(t, tt.wantArgs, cfg.IDE.Args)
		})
	}
}

func TestPiConfigFromTOML(t *testing.T) {
	tests := []struct {
		name         string
		tomlContent  string
		wantProvider string
		wantModel    string
		wantThinking string
	}{
		{
			name: "Not specified defaults to empty",
			tomlContent: `
[project]
name = "test"
`,
			wantProvider: "",
			wantModel:    "",
			wantThinking: "",
		},
		{
			name: "All fields specified",
			tomlContent: `
[project]
name = "test"

[pi]
provider = "anthropic"
model = "claude-sonnet-4-5-20250929"
thinking = "high"
`,
			wantProvider: "anthropic",
			wantModel:    "claude-sonnet-4-5-20250929",
			wantThinking: "high",
		},
		{
			name: "Only provider",
			tomlContent: `
[project]
name = "test"

[pi]
provider = "openai"
`,
			wantProvider: "openai",
			wantModel:    "",
			wantThinking: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			_, err := toml.Decode(tt.tomlContent, &cfg)
			require.NoError(t, err)

			require.Equal(t, tt.wantProvider, cfg.Pi.Provider)
			require.Equal(t, tt.wantModel, cfg.Pi.Model)
			require.Equal(t, tt.wantThinking, cfg.Pi.Thinking)
		})
	}
}

func TestUpdateConfig_AddsNewSection(t *testing.T) {
	// Create a minimal existing config (only project and repo sections)
	existingContent := `# Sarge Project Configuration

# =============================================================================
# Project Metadata
# =============================================================================
[project]
name = "test-project"
created_at = 2026-01-26T10:30:00Z

# =============================================================================
# Repository Configuration
# =============================================================================
[repo]
type = "github"
source = "https://github.com/example/repo"
path = "main"

# =============================================================================
# Beads Configuration
# =============================================================================
[beads]
path = "main/.beads"
`

	// Create temp file
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.toml"
	require.NoError(t, os.WriteFile(configPath, []byte(existingContent), 0600))

	cfg := &Config{
		Project: ProjectConfig{
			Name:      "test-project",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/example/repo",
			Path:   "main",
		},
		Beads: BeansConfig{
			Path: "main/.beads",
		},
	}

	added, err := UpdateConfig(configPath, cfg)
	require.NoError(t, err)

	// Should have added new sections
	require.NotEmpty(t, added, "Expected new sections to be added")
	require.Contains(t, added, "hooks")
	require.Contains(t, added, "claude")

	// Read back the merged config
	mergedBytes, err := os.ReadFile(configPath)
	require.NoError(t, err)
	merged := string(mergedBytes)

	// Original values should be preserved
	require.Contains(t, merged, `name = "test-project"`)
	require.Contains(t, merged, `type = "github"`)

	// New sections should be present (check for commented section headers)
	require.Contains(t, merged, "# [hooks]")
	// When AgentType is empty (default), [claude] section is rendered uncommented
	require.Contains(t, merged, "[claude]")

	// Backup should exist
	backupBytes, err := os.ReadFile(configPath + ".bak")
	require.NoError(t, err)
	require.Equal(t, existingContent, string(backupBytes))

	// Merged config should be valid TOML
	var parsed map[string]interface{}
	_, err = toml.Decode(merged, &parsed)
	require.NoError(t, err, "Merged config is not valid TOML:\n%s", merged)
}

func TestUpdateConfig_ExistingSectionsNotModified(t *testing.T) {
	// Config with user-configured claude section
	existingContent := `# =============================================================================
# Project Metadata
# =============================================================================
[project]
name = "my-project"
created_at = 2026-01-26T10:30:00Z

# =============================================================================
# Repository Configuration
# =============================================================================
[repo]
type = "local"
source = "/home/user/repo"
path = "main"

# =============================================================================
# Beads Configuration
# =============================================================================
[beads]
path = ".co/.beads"

# =============================================================================
# Claude Configuration
# =============================================================================
[claude]
skip_permissions = false
time_limit = 45
`

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.toml"
	require.NoError(t, os.WriteFile(configPath, []byte(existingContent), 0600))

	cfg := &Config{
		Project: ProjectConfig{
			Name:      "my-project",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "local",
			Source: "/home/user/repo",
			Path:   "main",
		},
		Beads: BeansConfig{
			Path: ".co/.beads",
		},
	}

	added, err := UpdateConfig(configPath, cfg)
	require.NoError(t, err)

	// Claude section already existed, so it should NOT be in the added list
	for _, name := range added {
		require.NotEqual(t, "claude", name, "claude section should not be re-added")
	}

	// Read merged config and verify claude values preserved
	mergedBytes, err := os.ReadFile(configPath)
	require.NoError(t, err)
	merged := string(mergedBytes)

	require.Contains(t, merged, "skip_permissions = false")
	require.Contains(t, merged, "time_limit = 45")
}

func TestUpdateConfig_Idempotent(t *testing.T) {
	// Start with a minimal config
	existingContent := `# =============================================================================
# Project Metadata
# =============================================================================
[project]
name = "test"
created_at = 2026-01-26T10:30:00Z

# =============================================================================
# Repository Configuration
# =============================================================================
[repo]
type = "github"
source = "https://github.com/example/repo"
path = "main"

# =============================================================================
# Beads Configuration
# =============================================================================
[beads]
path = "main/.beads"
`

	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.toml"
	require.NoError(t, os.WriteFile(configPath, []byte(existingContent), 0600))

	cfg := &Config{
		Project: ProjectConfig{
			Name:      "test",
			CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
		},
		Repo: RepoConfig{
			Type:   "github",
			Source: "https://github.com/example/repo",
			Path:   "main",
		},
		Beads: BeansConfig{
			Path: "main/.beads",
		},
	}

	// First update
	added1, err := UpdateConfig(configPath, cfg)
	require.NoError(t, err)
	require.NotEmpty(t, added1)

	// Read result of first update
	firstResult, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Second update - should be idempotent
	added2, err := UpdateConfig(configPath, cfg)
	require.NoError(t, err)
	require.Empty(t, added2, "Second update should add nothing")

	// Content should be identical after second run
	secondResult, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, string(firstResult), string(secondResult))
}

func TestGenerateDocumentedConfig_AgentType(t *testing.T) {
	baseCfg := func(agentType string) *Config {
		return &Config{
			Project: ProjectConfig{
				Name:      "test-project",
				CreatedAt: time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC),
			},
			Repo: RepoConfig{
				Type:   "github",
				Source: "https://github.com/example/repo",
				Path:   "main",
			},
			Beads: BeansConfig{
				Path: "main/.beads",
			},
			Agent: AgentConfig{
				Type: agentType,
			},
		}
	}

	t.Run("default empty agent type renders claude section uncommented", func(t *testing.T) {
		cfg := baseCfg("")
		content := cfg.GenerateDocumentedConfig()

		// Valid TOML
		var parsed map[string]interface{}
		_, err := toml.Decode(content, &parsed)
		require.NoError(t, err, "Generated config is not valid TOML:\n%s", content)

		// [agent] should be commented out (default = claude)
		require.NotContains(t, content, "\n[agent]\n")
		require.Contains(t, content, "# [agent]")

		// [claude] should be uncommented
		require.Contains(t, content, "\n[claude]\n")

		// [pi] should be commented out
		require.NotContains(t, content, "\n[pi]\n")
		require.Contains(t, content, "# [pi]")
	})

	t.Run("claude agent type renders claude section uncommented", func(t *testing.T) {
		cfg := baseCfg("claude")
		content := cfg.GenerateDocumentedConfig()

		var parsed map[string]interface{}
		_, err := toml.Decode(content, &parsed)
		require.NoError(t, err, "Generated config is not valid TOML:\n%s", content)

		// [agent] should be commented out (claude is default)
		require.NotContains(t, content, "\n[agent]\n")
		require.Contains(t, content, "# [agent]")

		// [claude] should be uncommented
		require.Contains(t, content, "\n[claude]\n")

		// [pi] should be commented out
		require.NotContains(t, content, "\n[pi]\n")
		require.Contains(t, content, "# [pi]")
	})

	t.Run("pi agent type renders agent and pi sections uncommented", func(t *testing.T) {
		cfg := baseCfg("pi")
		content := cfg.GenerateDocumentedConfig()

		var parsed map[string]interface{}
		_, err := toml.Decode(content, &parsed)
		require.NoError(t, err, "Generated config is not valid TOML:\n%s", content)

		// [agent] should be uncommented with type = "pi"
		require.Contains(t, content, "\n[agent]\n")
		require.Contains(t, content, `type = "pi"`)

		// [pi] should be uncommented
		require.Contains(t, content, "\n[pi]\n")

		// [claude] should be commented out
		require.NotContains(t, content, "\n[claude]\n")
		require.Contains(t, content, "# [claude]")

		// Verify round-trip: parse back and check Agent.Type
		var loaded Config
		_, err = toml.Decode(content, &loaded)
		require.NoError(t, err)
		require.Equal(t, "pi", loaded.Agent.Type)
	})

	t.Run("pi agent type round-trips through write and load", func(t *testing.T) {
		cfg := baseCfg("pi")
		tmpDir := t.TempDir()
		configPath := tmpDir + "/config.toml"

		err := cfg.SaveDocumentedConfig(configPath)
		require.NoError(t, err)

		loaded, err := LoadConfig(configPath)
		require.NoError(t, err)
		require.Equal(t, "pi", loaded.Agent.Type)
		require.Equal(t, "test-project", loaded.Project.Name)
	})

	t.Run("default agent type round-trips through write and load", func(t *testing.T) {
		cfg := baseCfg("")
		tmpDir := t.TempDir()
		configPath := tmpDir + "/config.toml"

		err := cfg.SaveDocumentedConfig(configPath)
		require.NoError(t, err)

		loaded, err := LoadConfig(configPath)
		require.NoError(t, err)
		// Empty agent type means default (claude), no [agent] section written
		require.Equal(t, "", loaded.Agent.Type)
	})
}

func TestAgentConfigFromTOML(t *testing.T) {
	tests := []struct {
		name     string
		toml     string
		wantType string
	}{
		{
			name: "Default empty",
			toml: `
[project]
name = "test"
`,
			wantType: "",
		},
		{
			name: "Claude agent",
			toml: `
[project]
name = "test"

[agent]
type = "claude"
`,
			wantType: "claude",
		},
		{
			name: "Pi agent",
			toml: `
[project]
name = "test"

[agent]
type = "pi"
`,
			wantType: "pi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			_, err := toml.Decode(tt.toml, &cfg)
			require.NoError(t, err)
			require.Equal(t, tt.wantType, cfg.Agent.Type)
		})
	}
}

func TestLoadConfig_DetectsUndecodedKeys(t *testing.T) {
	// Simulate a commented-out section header with uncommented keys beneath it.
	// The keys "use_agent" and "model" would normally belong to [log_parser], but
	// with that header commented out they float up to the [project] section,
	// becoming unrecognized.
	configContent := `
[project]
name = "test"

# [log_parser]
use_agent = true
model = "gpt-4"
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	_, err := LoadConfig(configPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unrecognized keys")
	require.Contains(t, err.Error(), "use_agent")
	require.Contains(t, err.Error(), "model")
}

func TestLoadConfig_ValidConfigNoFalsePositive(t *testing.T) {
	// A fully valid config should not trigger unrecognized key errors.
	configContent := `
[project]
name = "test"

[log_parser]
use_agent = true
model = "gpt-4"
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.Equal(t, "test", cfg.Project.Name)
	require.True(t, cfg.LogParser.UseAgent)
	require.Equal(t, "gpt-4", cfg.LogParser.Model)
}

func TestLoadConfig_CommentedKeysNoFalsePositive(t *testing.T) {
	// Commented-out keys should not trigger false positives.
	configContent := `
[project]
name = "test"
# description = "some desc"

[log_parser]
# name = "disabled"
model = "gpt-4"
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.Equal(t, "test", cfg.Project.Name)
	require.Equal(t, "gpt-4", cfg.LogParser.Model)
}

func TestLoadConfig_UnrecognizedKeyInSection(t *testing.T) {
	// A typo or unknown key within a valid section should be detected.
	configContent := `
[project]
name = "test"
bogus_key = "oops"
`
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0600))

	_, err := LoadConfig(configPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unrecognized keys")
	require.Contains(t, err.Error(), "bogus_key")
}

func TestGetAttachMode_DefaultsToWindow(t *testing.T) {
	m := &MultiplexerConfig{}
	require.Equal(t, "window", m.GetAttachMode())
}

func TestGetAttachMode_ReturnsTab(t *testing.T) {
	m := &MultiplexerConfig{AttachMode: "tab"}
	require.Equal(t, "tab", m.GetAttachMode())
}

func TestGetAttachMode_UnknownDefaultsToWindow(t *testing.T) {
	m := &MultiplexerConfig{AttachMode: "something-else"}
	require.Equal(t, "window", m.GetAttachMode())
}

