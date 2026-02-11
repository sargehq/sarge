package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// Button represents a clickable button.
type Button struct {
	Label string // Display text (e.g., "  Ok  ")
	ID    string // Zone ID for click detection (e.g., "ok")
}

// ButtonRow renders a horizontal row of buttons with focus and hover support.
type ButtonRow struct {
	Buttons    []Button
	Focused    int    // Index of focused button
	Hovered    string // ID of hovered button (from mouse)
	ZonePrefix string // Prefix for zone IDs (prevents collisions)
}

// NewButtonRow creates a button row. Use zone.NewPrefix() for the prefix.
func NewButtonRow(prefix string, buttons ...Button) *ButtonRow {
	return &ButtonRow{
		Buttons:    buttons,
		ZonePrefix: prefix,
	}
}

// Render returns the button row as a styled string.
func (b *ButtonRow) Render(colors Colors) string {
	var parts []string
	for i, btn := range b.Buttons {
		active := i == b.Focused || b.Hovered == btn.ID
		styled := renderButton(btn.Label, active, colors)
		zoneID := b.ZonePrefix + "btn-" + btn.ID
		parts = append(parts, zone.Mark(zoneID, styled))
	}
	return strings.Join(parts, "  ")
}

// FocusNext moves focus to the next button.
func (b *ButtonRow) FocusNext() {
	if len(b.Buttons) > 0 {
		b.Focused = (b.Focused + 1) % len(b.Buttons)
	}
}

// FocusPrev moves focus to the previous button.
func (b *ButtonRow) FocusPrev() {
	if len(b.Buttons) > 0 {
		b.Focused--
		if b.Focused < 0 {
			b.Focused = len(b.Buttons) - 1
		}
	}
}

// FocusedID returns the ID of the currently focused button.
func (b *ButtonRow) FocusedID() string {
	if b.Focused >= 0 && b.Focused < len(b.Buttons) {
		return b.Buttons[b.Focused].ID
	}
	return ""
}

// SetHovered updates the hovered button ID.
func (b *ButtonRow) SetHovered(id string) { b.Hovered = id }

// DetectClick returns the ID of the clicked button, or empty string.
func (b *ButtonRow) DetectClick(msg tea.MouseMsg) string {
	for _, btn := range b.Buttons {
		zoneID := b.ZonePrefix + "btn-" + btn.ID
		if zone.Get(zoneID).InBounds(msg) {
			return btn.ID
		}
	}
	return ""
}

// renderButton styles a single button.
func renderButton(label string, active bool, colors Colors) string {
	if active {
		return lipgloss.NewStyle().
			Foreground(colors.Black).
			Background(colors.Accent).
			Bold(true).
			Render(label)
	}
	accentStyle := lipgloss.NewStyle().Foreground(colors.Accent)
	// Style brackets with accent, label plain
	return "[" + accentStyle.Render(strings.TrimSpace(label)) + "]"
}
