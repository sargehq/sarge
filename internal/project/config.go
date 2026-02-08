package project

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/BurntSushi/toml"
)

//go:embed templates/config.tmpl
var configTemplateText string

// Config represents the project configuration stored in .co/config.toml.
type Config struct {
	Project   ProjectConfig   `toml:"project"`
	Repo      RepoConfig      `toml:"repo"`
	Beads     BeadsConfig     `toml:"beads"`
	Hooks     HooksConfig     `toml:"hooks"`
	Linear    LinearConfig    `toml:"linear"`
	Agent     AgentConfig     `toml:"agent"`
	Claude    ClaudeConfig    `toml:"claude"`
	Pi        PiConfig        `toml:"pi"`
	Workflow  WorkflowConfig  `toml:"workflow"`
	Scheduler SchedulerConfig `toml:"scheduler"`
	Zellij    ZellijConfig    `toml:"zellij"`
	LogParser LogParserConfig `toml:"log_parser"`
	IDE       IDEConfig       `toml:"ide"`
	Debug     DebugConfig     `toml:"debug"`
}

// IDEConfig contains IDE configuration for opening worktrees.
type IDEConfig struct {
	// Command is the IDE command to run (e.g., "code", "cursor", "zed").
	// If empty, falls back to EDITOR environment variable.
	Command string `toml:"command"`

	// Args is a list of additional arguments to pass to the IDE command.
	// The worktree path is always appended as the final argument.
	Args []string `toml:"args"`
}

// GetIDECommand returns the configured IDE command or falls back to EDITOR env var.
// Returns empty string if neither is configured.
func (i *IDEConfig) GetIDECommand() string {
	if i.Command != "" {
		return i.Command
	}
	return os.Getenv("EDITOR")
}

// DebugConfig contains debug/diagnostics configuration.
type DebugConfig struct {
	// Pprof enables the pprof HTTP server on an ephemeral port.
	// Defaults to false when not specified.
	Pprof bool `toml:"pprof"`
}

// LogParserConfig contains log parser configuration.
type LogParserConfig struct {
	// UseClaude controls whether to use Claude for log analysis instead of the Go parser.
	// Defaults to false when not specified.
	UseClaude bool `toml:"use_claude"`

	// Model specifies which Claude model to use for log analysis.
	// Valid values: "haiku", "sonnet", "opus"
	// Defaults to "haiku" when not specified.
	Model string `toml:"model"`
}

// ShouldUseClaude returns true if Claude should be used for log analysis.
func (l *LogParserConfig) ShouldUseClaude() bool {
	return l.UseClaude
}

// GetModel returns the configured Claude model for log analysis.
// Defaults to "haiku" when not specified or when an invalid model is configured.
// Valid models are: "haiku", "sonnet", "opus".
func (l *LogParserConfig) GetModel() string {
	if l.Model == "" {
		return "haiku"
	}
	// Validate the model is one of the allowed values
	switch l.Model {
	case "haiku", "sonnet", "opus":
		return l.Model
	default:
		// Return default for invalid values
		return "haiku"
	}
}

// AgentConfig contains coding agent configuration.
type AgentConfig struct {
	// Type selects which coding agent to use: "claude" (default) or "pi".
	Type string `toml:"type"`
}

// PiConfig contains pi coding agent configuration.
type PiConfig struct {
	// Provider selects the AI provider (e.g., "anthropic", "openai", "google").
	Provider string `toml:"provider"`

	// Model specifies which model to use with the selected provider.
	Model string `toml:"model"`

	// Thinking sets the reasoning/thinking level (e.g., "low", "medium", "high").
	Thinking string `toml:"thinking"`
}

// ClaudeConfig contains Claude Code configuration.
type ClaudeConfig struct {
	// SkipPermissions controls whether to run Claude with --dangerously-skip-permissions.
	// Defaults to true when not specified in config.
	SkipPermissions *bool `toml:"skip_permissions"`

	// TimeLimitMinutes is the maximum duration in minutes for a Claude session.
	// When set to 0 or omitted, there is no time limit.
	TimeLimitMinutes int `toml:"time_limit"`

	// TaskTimeoutMinutes controls the maximum execution time for a task in minutes.
	// Defaults to 60 minutes when not specified.
	TaskTimeoutMinutes *int `toml:"task_timeout_minutes"`
}

// ShouldSkipPermissions returns true if Claude should run with --dangerously-skip-permissions.
// Defaults to true when not explicitly configured.
func (c *ClaudeConfig) ShouldSkipPermissions() bool {
	if c.SkipPermissions == nil {
		return true // default to true
	}
	return *c.SkipPermissions
}

// TimeLimit returns the maximum duration for a Claude session.
// Returns 0 if no time limit is configured.
func (c *ClaudeConfig) TimeLimit() time.Duration {
	if c.TimeLimitMinutes <= 0 {
		return 0
	}
	return time.Duration(c.TimeLimitMinutes) * time.Minute
}

// GetTaskTimeout returns the task timeout duration.
// Defaults to 60 minutes when not explicitly configured.
// If time_limit is set and is less than the default/configured task_timeout_minutes,
// time_limit takes precedence.
func (c *ClaudeConfig) GetTaskTimeout() time.Duration {
	// Calculate the task timeout
	var taskTimeout time.Duration
	if c.TaskTimeoutMinutes == nil || *c.TaskTimeoutMinutes <= 0 {
		taskTimeout = 60 * time.Minute // default to 60 minutes
	} else {
		taskTimeout = time.Duration(*c.TaskTimeoutMinutes) * time.Minute
	}

	// If time_limit is set and is less than task timeout, use time_limit
	if c.TimeLimitMinutes > 0 {
		timeLimit := time.Duration(c.TimeLimitMinutes) * time.Minute
		if timeLimit < taskTimeout {
			return timeLimit
		}
	}

	return taskTimeout
}

// ProjectConfig contains project metadata.
type ProjectConfig struct {
	Name      string    `toml:"name"`
	CreatedAt time.Time `toml:"created_at"`
}

// RepoConfig contains repository configuration.
type RepoConfig struct {
	Type       string `toml:"type"`        // "local" or "github"
	Source     string `toml:"source"`      // Original path or URL
	Path       string `toml:"path"`        // Always "main"
	BaseBranch string `toml:"base_branch"` // Base branch for feature branches (default: "main")
}

// GetBaseBranch returns the configured base branch or "main" if not set.
func (r *RepoConfig) GetBaseBranch() string {
	if r.BaseBranch == "" {
		return "main"
	}
	return r.BaseBranch
}

// HooksConfig contains hook configuration.
type HooksConfig struct {
	// Env is a list of environment variables to set before running commands.
	// Format: ["KEY=value", "ANOTHER_KEY=value"]
	// These are applied when spawning Claude in zellij tabs.
	Env []string `toml:"env"`
}

// LinearConfig contains Linear integration configuration.
type LinearConfig struct {
	// APIKey is the Linear API key for authentication.
	APIKey string `toml:"api_key"`
}

// WorkflowConfig contains workflow configuration.
type WorkflowConfig struct {
	// MaxReviewIterations limits the number of review/fix cycles.
	// Defaults to 2 when not specified.
	MaxReviewIterations *int `toml:"max_review_iterations"`
}

// GetMaxReviewIterations returns the configured max review iterations or 2 if not specified.
func (w *WorkflowConfig) GetMaxReviewIterations() int {
	if w.MaxReviewIterations == nil {
		return 2
	}
	return *w.MaxReviewIterations
}

// SchedulerConfig contains scheduler timing configuration.
type SchedulerConfig struct {
	// PRFeedbackIntervalMinutes is the interval between PR feedback checks.
	// Defaults to 5 minutes when not specified.
	PRFeedbackIntervalMinutes *int `toml:"pr_feedback_interval_minutes"`

	// CommentResolutionIntervalMinutes is the interval between comment resolution checks.
	// Defaults to 5 minutes when not specified.
	CommentResolutionIntervalMinutes *int `toml:"comment_resolution_interval_minutes"`

	// SchedulerPollSeconds is the scheduler polling interval.
	// Defaults to 1 second when not specified.
	SchedulerPollSeconds *int `toml:"scheduler_poll_seconds"`

	// ActivityUpdateSeconds is the interval for updating task activity timestamps.
	// Defaults to 30 seconds when not specified.
	ActivityUpdateSeconds *int `toml:"activity_update_seconds"`
}

// GetPRFeedbackInterval returns the PR feedback check interval.
// Defaults to 5 minutes when not specified.
func (s *SchedulerConfig) GetPRFeedbackInterval() time.Duration {
	if s.PRFeedbackIntervalMinutes != nil && *s.PRFeedbackIntervalMinutes > 0 {
		return time.Duration(*s.PRFeedbackIntervalMinutes) * time.Minute
	}
	return 5 * time.Minute
}

// GetCommentResolutionInterval returns the comment resolution check interval.
// Defaults to 5 minutes when not specified.
func (s *SchedulerConfig) GetCommentResolutionInterval() time.Duration {
	if s.CommentResolutionIntervalMinutes != nil && *s.CommentResolutionIntervalMinutes > 0 {
		return time.Duration(*s.CommentResolutionIntervalMinutes) * time.Minute
	}
	return 5 * time.Minute
}

// GetSchedulerPollInterval returns the scheduler polling interval.
// Defaults to 1 second when not specified.
func (s *SchedulerConfig) GetSchedulerPollInterval() time.Duration {
	if s.SchedulerPollSeconds != nil && *s.SchedulerPollSeconds > 0 {
		return time.Duration(*s.SchedulerPollSeconds) * time.Second
	}
	return 1 * time.Second
}

// GetActivityUpdateInterval returns the activity update interval.
// Defaults to 30 seconds when not specified.
func (s *SchedulerConfig) GetActivityUpdateInterval() time.Duration {
	if s.ActivityUpdateSeconds != nil && *s.ActivityUpdateSeconds > 0 {
		return time.Duration(*s.ActivityUpdateSeconds) * time.Second
	}
	return 30 * time.Second
}

// ZellijConfig contains zellij tab management configuration.
type ZellijConfig struct {
	// KillTabsOnDestroy controls whether to automatically kill zellij tabs
	// when work is destroyed. Includes work, task, console, and claude tabs.
	// Defaults to true when not specified.
	KillTabsOnDestroy *bool `toml:"kill_tabs_on_destroy"`
}

// BeadsConfig contains beads path configuration.
type BeadsConfig struct {
	// Path to beads directory (relative to project root)
	// "main/.beads" = beads in repository (synced with git)
	// ".co/.beads" = project-local beads (standalone, not synced)
	Path string `toml:"path"`
}

// ShouldKillTabsOnDestroy returns true if zellij tabs should be killed when work is destroyed.
// Defaults to true when not explicitly configured.
func (z *ZellijConfig) ShouldKillTabsOnDestroy() bool {
	if z.KillTabsOnDestroy == nil {
		return true // default to true
	}
	return *z.KillTabsOnDestroy
}

// LoadConfig reads and parses a config.toml file.
func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the config to the specified path.
func (c *Config) SaveConfig(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	return nil
}

// SaveDocumentedConfig writes a fully documented config to the specified path.
// This creates a config file with inline comments explaining all available options.
func (c *Config) SaveDocumentedConfig(path string) error {
	content := c.GenerateDocumentedConfig()
	return os.WriteFile(path, []byte(content), 0600)
}

// configTemplateData holds the data used to render the config template.
type configTemplateData struct {
	ProjectName string
	CreatedAt   string
	RepoType    string
	RepoSource  string
	RepoPath    string
	BeadsPath   string
}

// tomlString formats a string for TOML output with proper escaping.
// It wraps the string in double quotes and escapes special characters.
func tomlString(s string) string {
	// Escape backslashes first, then quotes
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)
	escaped = strings.ReplaceAll(escaped, "\t", `\t`)
	return `"` + escaped + `"`
}

// configTemplate is the parsed template for generating documented config files.
var configTemplate = template.Must(template.New("config").Funcs(template.FuncMap{
	"tomlString": tomlString,
}).Parse(configTemplateText))

// configSection represents a section block from the config template.
type configSection struct {
	// name is the TOML section name (e.g., "project", "hooks")
	name string
	// text is the full text of the section block including separator and comments
	text string
}

// parseConfigSections splits a config file into sections based on "# ====..." separator lines.
// Each section includes the separator, comments, and TOML content up to the next separator.
func parseConfigSections(content string) []configSection {
	lines := strings.Split(content, "\n")
	var sections []configSection
	var currentLines []string
	var currentName string

	for _, line := range lines {
		if strings.HasPrefix(line, "# ====") {
			// Save previous section if any
			if len(currentLines) > 0 {
				sections = append(sections, configSection{
					name: currentName,
					text: strings.Join(currentLines, "\n"),
				})
			}
			currentLines = []string{line}
			currentName = ""
		} else {
			currentLines = append(currentLines, line)
			// Detect section name from [section] or # [section] lines
			if currentName == "" {
				trimmed := strings.TrimSpace(line)
				// Match uncommented [section] or commented # [section]
				if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") {
					currentName = extractSectionName(trimmed)
				} else if strings.HasPrefix(trimmed, "# [") && strings.Contains(trimmed, "]") {
					currentName = extractSectionName(strings.TrimPrefix(trimmed, "# "))
				}
			}
		}
	}
	// Save last section
	if len(currentLines) > 0 {
		sections = append(sections, configSection{
			name: currentName,
			text: strings.Join(currentLines, "\n"),
		})
	}

	return sections
}

// extractSectionName extracts the section name from a TOML header like "[section_name]".
func extractSectionName(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return ""
	}
	end := strings.Index(s, "]")
	if end < 0 {
		return ""
	}
	return s[1:end]
}

// findExistingSections returns a set of section names that exist (uncommented) in the config text.
func findExistingSections(content string) map[string]bool {
	sections := make(map[string]bool)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		// Only match uncommented section headers
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") && !strings.HasPrefix(trimmed, "#") {
			name := extractSectionName(trimmed)
			if name != "" {
				sections[name] = true
			}
		}
	}
	return sections
}

// UpdateConfig merges new config template sections into an existing config file.
// It preserves all existing user values and comments. New sections from the template
// that are not present in the existing file are appended (commented out).
// A backup of the original file is created at path + ".bak".
// Returns a list of section names that were added, or nil if no changes were needed.
func UpdateConfig(existingPath string, cfg *Config) ([]string, error) {
	// Read existing config
	existingBytes, err := os.ReadFile(existingPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing config: %w", err)
	}
	existingContent := string(existingBytes)

	// Generate the latest template
	templateContent := cfg.GenerateDocumentedConfig()

	// Parse template into sections
	templateSections := parseConfigSections(templateContent)

	// Find which sections already exist in the user's config
	existingSections := findExistingSections(existingContent)

	// Also check for commented-out section headers in existing config
	// to avoid re-adding sections that user already has (even if commented)
	for _, line := range strings.Split(existingContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# [") && strings.Contains(trimmed, "]") {
			name := extractSectionName(strings.TrimPrefix(trimmed, "# "))
			if name != "" {
				existingSections[name] = true
			}
		}
	}

	// Collect new sections to add
	var newSections []string
	var toAppend []string
	for _, section := range templateSections {
		if section.name == "" {
			continue
		}
		if !existingSections[section.name] {
			newSections = append(newSections, section.name)
			toAppend = append(toAppend, section.text)
		}
	}

	if len(toAppend) == 0 {
		return nil, nil // nothing to update
	}

	// Create backup
	backupPath := existingPath + ".bak"
	if err := os.WriteFile(backupPath, existingBytes, 0600); err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	// Build merged content
	merged := strings.TrimRight(existingContent, "\n") + "\n"
	for _, section := range toAppend {
		merged += "\n" + section + "\n"
	}

	// Write merged config
	if err := os.WriteFile(existingPath, []byte(merged), 0600); err != nil {
		return nil, fmt.Errorf("failed to write updated config: %w", err)
	}

	return newSections, nil
}

// DryRunUpdateConfig checks what sections would be added by UpdateConfig without writing anything.
// Returns the list of section names that would be added, or nil if no changes are needed.
func DryRunUpdateConfig(existingPath string, cfg *Config) ([]string, error) {
	existingBytes, err := os.ReadFile(existingPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing config: %w", err)
	}
	existingContent := string(existingBytes)

	templateContent := cfg.GenerateDocumentedConfig()
	templateSections := parseConfigSections(templateContent)
	existingSections := findExistingSections(existingContent)

	// Also check for commented-out section headers
	for _, line := range strings.Split(existingContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# [") && strings.Contains(trimmed, "]") {
			name := extractSectionName(strings.TrimPrefix(trimmed, "# "))
			if name != "" {
				existingSections[name] = true
			}
		}
	}

	var newSections []string
	for _, section := range templateSections {
		if section.name == "" {
			continue
		}
		if !existingSections[section.name] {
			newSections = append(newSections, section.name)
		}
	}

	if len(newSections) == 0 {
		return nil, nil
	}
	return newSections, nil
}

// GenerateDocumentedConfig generates a documented config.toml string with comments.
// This includes the actual project values plus commented-out examples for optional sections.
func (c *Config) GenerateDocumentedConfig() string {
	data := configTemplateData{
		ProjectName:   c.Project.Name,
		CreatedAt:     c.Project.CreatedAt.Format(time.RFC3339),
		RepoType:      c.Repo.Type,
		RepoSource:    c.Repo.Source,
		RepoPath:      c.Repo.Path,
		BeadsPath: c.Beads.Path,
	}

	var buf bytes.Buffer
	if err := configTemplate.Execute(&buf, data); err != nil {
		// Fall back to a minimal valid TOML if template execution fails
		return fmt.Sprintf("[project]\nname = %q\ncreated_at = %s\n[repo]\ntype = %q\nsource = %q\npath = %q\n",
			c.Project.Name, data.CreatedAt, c.Repo.Type, c.Repo.Source, c.Repo.Path)
	}
	return buf.String()
}
