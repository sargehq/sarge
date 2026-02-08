package splash

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sargehq/sarge/internal/tui/ui"
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

// PromptConfig holds parameters for rendering the splash screen with a prompt.
type PromptConfig struct {
	Config    Config
	InputView string           // Pre-rendered textinput view from bubbletea
	Timeline  *ui.ChatTimeline // Scrollable chat timeline component
	Busy      bool             // True when agent is processing (blocks input, dims prompt)
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

// RenderWithPrompt renders the splash screen with a chat timeline and prompt input.
// Layout: vPad | art | gap | flex messages | gap | input | hints | vPad
func RenderWithPrompt(width, height int, pcfg PromptConfig) string {
	cfg := pcfg.Config
	tl := pcfg.Timeline

	// Gradient ASCII art
	art := renderGradientText(strings.TrimLeft(splashArt, "\n"), cfg.Gradient)
	artHeight := strings.Count(art, "\n") + 1

	// Vertical padding: max(10, 25% of viewport height) on top and bottom
	vPad := height / 4
	if vPad < 10 {
		vPad = 10
	}

	// Content width for messages and input (centered with padding)
	contentWidth := width - 4
	if contentWidth > 100 {
		contentWidth = 100
	}
	if contentWidth < 40 {
		contentWidth = 40
	}

	// Render the input box
	promptBorder := cfg.Accent
	if pcfg.Busy || (tl != nil && tl.SelectedIdx() >= 0) {
		promptBorder = cfg.Dim
	}
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(promptBorder).
		Padding(0, 1).
		Width(contentWidth)
	inputBox := inputStyle.Render(pcfg.InputView)
	inputHeight := lipgloss.Height(inputBox)

	// Hints below the input: shortcut bar + prompt-specific hints
	dimStyle := lipgloss.NewStyle().Foreground(cfg.Dim)
	shortcutBar := renderHints(cfg)
	hasMessages := tl != nil && tl.MessageCount() > 0
	var promptHintText string
	if pcfg.Busy {
		promptHintText = dimStyle.Render("[↑] messages  [q] quit")
	} else if tl != nil && tl.SelectedIdx() >= 0 {
		promptHintText = dimStyle.Render("[o] open  [↓] input  [esc] deselect")
	} else if hasMessages {
		promptHintText = dimStyle.Render("[enter] send  [↑] messages  [q] quit")
	} else {
		promptHintText = dimStyle.Render("[enter] send  [q] quit")
	}
	hintsContent := shortcutBar + "\n" + promptHintText
	hintsLine := lipgloss.NewStyle().Width(contentWidth).Align(lipgloss.Center).Render(hintsContent)
	hintsHeight := 2

	// Message area flexes to fill remaining space
	fixedHeight := vPad + artHeight + inputHeight + hintsHeight + vPad + 2 // 2 gaps
	msgAreaHeight := height - fixedHeight
	if msgAreaHeight < 0 {
		msgAreaHeight = 0
	}

	// Render messages via the timeline component
	var messagesView string
	if tl != nil && tl.MessageCount() > 0 {
		tl.SetSize(contentWidth, msgAreaHeight)
		messagesView = tl.Render()
	} else {
		// Empty state: centered hint with breathing room below
		emptyHint := dimStyle.Render("What do you want Sarge to work on next?") + "\n"
		messagesView = lipgloss.Place(contentWidth, msgAreaHeight, lipgloss.Center, lipgloss.Center, emptyHint)
	}

	// Center helper
	center := func(s string) string {
		return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(s)
	}

	// Assemble layout
	totalContent := lipgloss.JoinVertical(lipgloss.Left,
		strings.Repeat("\n", vPad-1),
		center(art),
		"",
		center(messagesView),
		"",
		center(inputBox),
		center(hintsLine),
		strings.Repeat("\n", vPad-1),
	)

	return totalContent
}
