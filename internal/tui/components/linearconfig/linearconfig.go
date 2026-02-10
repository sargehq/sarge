package linearconfig

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/sargehq/sarge/internal/tui/ui"
)

// Action represents the result of a user interaction.
type Action int

const (
	ActionNone   Action = iota
	ActionSubmit        // User confirmed the API key
	ActionCancel        // User cancelled
)

// Panel is the Linear API key configuration form.
type Panel struct {
	width  int
	height int
	colors ui.Colors

	apiKeyField *ui.FormField
	buttons     *ui.ButtonRow
	focusIdx    int // 0 = field, 1 = buttons

	zonePrefix string
}

// New creates a new Linear config panel.
func New(colors ui.Colors) *Panel {
	prefix := zone.NewPrefix()
	return &Panel{
		width:  60,
		height: 20,
		colors: colors,
		apiKeyField: ui.NewFormField("API Key", "lin_api_...", 40),
		buttons: ui.NewButtonRow(prefix,
			ui.Button{Label: "  Ok  ", ID: "ok"},
			ui.Button{Label: "Cancel", ID: "cancel"},
		),
		zonePrefix: prefix,
	}
}

// Init focuses the input field and returns the blink command.
func (p *Panel) Init() tea.Cmd {
	p.focusIdx = 0
	return p.apiKeyField.Focus()
}

// SetSize updates panel dimensions.
func (p *Panel) SetSize(width, height int) {
	p.width = width
	p.height = height
	inputWidth := width - 16
	if inputWidth < 20 {
		inputWidth = 20
	}
	if inputWidth > 50 {
		inputWidth = 50
	}
	p.apiKeyField.SetWidth(inputWidth)
}

// Update handles key events and returns a command and action.
func (p *Panel) Update(msg tea.KeyMsg) (tea.Cmd, Action) {
	if msg.Type == tea.KeyEsc {
		return nil, ActionCancel
	}

	switch msg.String() {
	case "tab":
		return p.cycleFocus(1), ActionNone
	case "shift+tab":
		return p.cycleFocus(-1), ActionNone
	case "enter":
		if p.focusIdx == 1 {
			// On buttons
			switch p.buttons.FocusedID() {
			case "ok":
				if strings.TrimSpace(p.apiKeyField.Value()) != "" {
					return nil, ActionSubmit
				}
				return nil, ActionNone
			case "cancel":
				return nil, ActionCancel
			}
		}
		// Enter on field = submit if non-empty
		if strings.TrimSpace(p.apiKeyField.Value()) != "" {
			return nil, ActionSubmit
		}
		return nil, ActionNone
	}

	// Route to focused element
	if p.focusIdx == 0 {
		cmd := p.apiKeyField.Update(msg)
		return cmd, ActionNone
	}

	// Button navigation
	switch msg.String() {
	case "h", "left":
		p.buttons.FocusPrev()
	case "l", "right":
		p.buttons.FocusNext()
	}
	return nil, ActionNone
}

// Value returns the entered API key.
func (p *Panel) Value() string {
	return strings.TrimSpace(p.apiKeyField.Value())
}

// Render returns the panel as a centered dialog.
func (p *Panel) Render() string {
	dimStyle := lipgloss.NewStyle().Foreground(p.colors.Dim)

	instructions := dimStyle.Render("Get your API key from:") + "\n" +
		dimStyle.Render("  Linear > Settings > API > Personal API keys") + "\n"

	field := p.apiKeyField.Render(p.focusIdx == 0, p.colors)
	buttons := p.buttons.Render(p.colors)
	hint := dimStyle.Render("[Tab] Next  [Enter] Confirm  [Esc] Cancel")

	content := instructions + "\n" + field + "\n\n" + buttons + "\n" + hint

	dlg := ui.Dialog{Title: "Linear Not Configured", MaxWidth: 56}
	return dlg.Render(p.width, p.height, content, p.colors)
}

// cycleFocus moves focus forward or backward and returns any tea command.
func (p *Panel) cycleFocus(dir int) tea.Cmd {
	if p.focusIdx == 0 {
		p.apiKeyField.Blur()
	}

	p.focusIdx += dir
	if p.focusIdx < 0 {
		p.focusIdx = 1
	} else if p.focusIdx > 1 {
		p.focusIdx = 0
	}

	if p.focusIdx == 0 {
		return p.apiKeyField.Focus()
	}
	return nil
}
