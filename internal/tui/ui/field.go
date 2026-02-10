package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FormField is a labeled text input.
type FormField struct {
	Label string
	Input textinput.Model
}

// NewFormField creates a labeled text input with the given placeholder.
func NewFormField(label, placeholder string, width int) *FormField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 200
	ti.Width = width
	return &FormField{Label: label, Input: ti}
}

// Update delegates a key event to the underlying textinput.
func (f *FormField) Update(msg tea.KeyMsg) tea.Cmd {
	var cmd tea.Cmd
	f.Input, cmd = f.Input.Update(msg)
	return cmd
}

// Render returns the field with its label.
// When focused, the label is rendered in the accent color.
func (f *FormField) Render(focused bool, colors Colors) string {
	labelStyle := lipgloss.NewStyle().Foreground(colors.Dim)
	if focused {
		labelStyle = labelStyle.Foreground(colors.Accent)
	}
	return labelStyle.Render(f.Label) + "\n" + f.Input.View()
}

// Value returns the current input value.
func (f *FormField) Value() string { return f.Input.Value() }

// SetValue sets the input value.
func (f *FormField) SetValue(v string) { f.Input.SetValue(v) }

// Focus focuses the input and returns the blink command.
func (f *FormField) Focus() tea.Cmd { return f.Input.Focus() }

// Blur removes focus from the input.
func (f *FormField) Blur() { f.Input.Blur() }

// SetWidth updates the input width.
func (f *FormField) SetWidth(w int) { f.Input.Width = w }
