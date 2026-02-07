package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DOS Rebel font ASCII art for "sarge"
const splashArt = `
  █████   ██████   ████████   ███████  ██████
 ███░░   ░░░░░███ ░░███░░███ ███░░███ ███░░███
░░█████   ███████  ░███ ░░░ ░███ ░███░███████
 ░░░░███ ███░░███  ░███     ░███ ░███░███░░░
 ██████ ░░████████ █████    ░░███████░░██████
░░░░░░   ░░░░░░░░ ░░░░░      ░░░░░███ ░░░░░░
                             ███ ░███
                            ░░██████
                             ░░░░░░           `

// Warm gradient palette for the splash art
var splashGradient = []lipgloss.Color{
	lipgloss.Color("#FF6B6B"),
	lipgloss.Color("#FF8E53"),
	lipgloss.Color("#FFA07A"),
	lipgloss.Color("#FFB347"),
	lipgloss.Color("#FFC93C"),
	lipgloss.Color("#FFD700"),
	lipgloss.Color("#FFDF00"),
	lipgloss.Color("#FFE44D"),
	lipgloss.Color("#FFEC80"),
}

// renderGradientText applies a gradient color to each line of text
func renderGradientText(text string, colors []lipgloss.Color) string {
	lines := strings.Split(text, "\n")
	var result strings.Builder
	for i, line := range lines {
		if line == "" {
			result.WriteString("\n")
			continue
		}
		color := colors[i%len(colors)]
		style := lipgloss.NewStyle().Foreground(color)
		result.WriteString(style.Render(line))
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}
	return result.String()
}

// renderSplash renders the splash/welcome screen centered in the given dimensions
func renderSplash(width, height int) string {
	// Render the gradient ASCII art
	art := renderGradientText(strings.TrimLeft(splashArt, "\n"), splashGradient)

	// Getting-started hints
	hints := "\n\n" + styleHotkeys("[n] Create issue  [i] Import from Linear  [I] Import from PR  [?] Help")

	content := art + hints

	// Center everything in the available space
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// shouldShowSplash returns true when the splash screen should be shown:
// no issues, no works, not loading, and initial data fetch completed
func (m *planModel) shouldShowSplash() bool {
	return len(m.beadItems) == 0 &&
		len(m.workTiles) == 0 &&
		!m.loading &&
		!m.lastUpdate.IsZero()
}
