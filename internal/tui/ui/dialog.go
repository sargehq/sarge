package ui

import "github.com/charmbracelet/lipgloss"

// Dialog renders content inside a centered bordered box.
type Dialog struct {
	Title    string
	MaxWidth int
}

// Render wraps content in a bordered box, centered in the given dimensions.
func (d Dialog) Render(width, height int, content string, colors Colors) string {
	maxW := d.MaxWidth
	if maxW <= 0 {
		maxW = 56
	}
	if maxW > width-6 {
		maxW = width - 6
	}
	if maxW < 30 {
		maxW = 30
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colors.Border).
		Padding(1, 2).
		Width(maxW)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colors.Text)

	inner := titleStyle.Render(d.Title) + "\n\n" + content
	box := boxStyle.Render(inner)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
