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
	// Render the gradient ASCII art using theme colors
	art := renderGradientText(strings.TrimLeft(splashArt, "\n"), CurrentTheme().SplashGradient)

	// Getting-started hints
	hints := "\n\n" + styleHotkeys("[n] Create issue  [i] Import from Linear  [I] Import from PR  [?] Help")

	content := art + hints

	// Center everything in the available space
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

// shouldShowSplash returns true when the splash screen should be shown:
// no issues, no works, not loading, and initial data fetch completed
func (m *planModel) shouldShowSplash() bool {
	return m.viewMode == ViewNormal &&
		len(m.beadItems) == 0 &&
		len(m.workTiles) == 0 &&
		!m.loading &&
		!m.lastUpdate.IsZero()
}
