package ui

import "github.com/charmbracelet/lipgloss"

// Colors holds the color palette for UI components.
// All components accept this instead of reading from a global theme,
// so they remain decoupled and testable.
type Colors struct {
	Accent lipgloss.Color // Active elements, hotkeys, focused borders
	Text   lipgloss.Color // Primary text
	Dim    lipgloss.Color // Secondary text, placeholders, hints
	Border lipgloss.Color // Default borders
	Error  lipgloss.Color // Error states
	Black  lipgloss.Color // Text on accent/inverted backgrounds
}
