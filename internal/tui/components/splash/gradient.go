package splash

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderGradientText applies a gradient color to each line of text.
func renderGradientText(text string, colors []lipgloss.Color) string {
	if len(colors) == 0 {
		return text
	}
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
