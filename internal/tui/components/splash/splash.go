package splash

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

//go:embed logo.txt
var splashArt string

// Config holds all parameters the splash screen needs from its caller.
// The caller fills this from the theme and tool availability checks.
type Config struct {
	// Colors (populated from theme)
	Gradient []lipgloss.Color // Splash gradient colors
	Accent   lipgloss.Color   // Hotkey brackets
	Error    lipgloss.Color   // Warning box border/title
	Warning  lipgloss.Color   // Warning text highlights
	Dim      lipgloss.Color   // Dimmed/disabled hints
	Text     lipgloss.Color   // Normal text
	Border   lipgloss.Color   // Info box border (non-error)

	// Tool availability
	HasBd bool // beads CLI
	HasGh bool // GitHub CLI

	// Platform for install instructions
	Platform Platform
}

// hintItem represents a single hint on the splash screen.
type hintItem struct {
	key     string
	label   string
	enabled bool
}

// Render renders the splash/welcome screen centered in the given dimensions.
func Render(width, height int, cfg Config) string {
	// Gradient ASCII art
	art := renderGradientText(strings.TrimLeft(splashArt, "\n"), cfg.Gradient)

	// Tool warning box (empty string if all tools present)
	warnings := renderToolWarnings(cfg)
	if warnings != "" {
		warnings = "\n\n" + warnings
	}

	// Hints with dimming for unavailable tools
	hints := "\n\n" + renderHints(cfg)

	content := art + warnings + hints

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// RenderInfoBox renders a standalone centered information box.
// Used for "Linear not configured", "tool missing" overlays, etc.
func RenderInfoBox(width, height int, title, body string, cfg Config) string {
	maxWidth := min(60, width-10)
	if maxWidth < 30 {
		maxWidth = 30
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(cfg.Text)
	bodyStyle := lipgloss.NewStyle().Foreground(cfg.Text)
	dismissStyle := lipgloss.NewStyle().Foreground(cfg.Dim)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cfg.Border).
		Padding(1, 2).
		Width(maxWidth)

	content := titleStyle.Render(title) + "\n\n" +
		bodyStyle.Render(body) + "\n\n" +
		dismissStyle.Render("Press any key to dismiss...")

	box := boxStyle.Render(content)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderHints renders the getting-started hints line with dimming.
func renderHints(cfg Config) string {
	items := []hintItem{
		{key: "n", label: "Create issue", enabled: cfg.HasBd},
		{key: "i", label: "Import from Linear", enabled: true},
		{key: "I", label: "Import from PR", enabled: cfg.HasGh},
		{key: "?", label: "Help", enabled: true},
	}

	var parts []string
	for _, item := range items {
		parts = append(parts, renderHint(item, cfg))
	}
	return strings.Join(parts, "  ")
}

// renderHint renders a single hint item, dimmed if disabled.
func renderHint(item hintItem, cfg Config) string {
	if !item.enabled {
		dimStyle := lipgloss.NewStyle().Foreground(cfg.Dim)
		return dimStyle.Render(fmt.Sprintf("[%s] %s", item.key, item.label))
	}
	// Accent-colored key in brackets, normal label
	accentStyle := lipgloss.NewStyle().Foreground(cfg.Accent)
	return "[" + accentStyle.Render(item.key) + "] " + item.label
}
