package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sargehq/sarge/internal/bridge"
)

// SessionPickerAction represents an action from the session picker.
type SessionPickerAction int

const (
	SessionPickerActionNone   SessionPickerAction = iota
	SessionPickerActionCancel                     // User pressed Esc
	SessionPickerActionSelect                     // User pressed Enter
)

// sessionPickerEntry represents a bridge session in the picker list.
type sessionPickerEntry struct {
	ID    string
	Type  string // "orch", "agent", "plan"
	State bridge.SessionState
}

// SessionPickerPanel shows a filterable list of bridge sessions for a work.
type SessionPickerPanel struct {
	width  int
	height int

	sessions []sessionPickerEntry
	filtered []sessionPickerEntry
	cursor   int
	filter   textinput.Model
}

// NewSessionPickerPanel creates a new SessionPickerPanel.
func NewSessionPickerPanel() *SessionPickerPanel {
	ti := textinput.New()
	ti.Placeholder = "Filter sessions..."
	ti.CharLimit = 100
	ti.SetWidth(40)

	return &SessionPickerPanel{
		width:  60,
		height: 20,
		filter: ti,
	}
}

// Init initializes the panel.
func (p *SessionPickerPanel) Init() tea.Cmd {
	return textinput.Blink
}

// SetSessions populates the picker from a bridge.
func (p *SessionPickerPanel) SetSessions(sessions map[string]bridge.SessionState) {
	p.sessions = nil
	for id, state := range sessions {
		// Infer type from ID naming convention
		sessionType := "orch"
		if strings.Contains(id, "agent") {
			sessionType = "agent"
		} else if strings.Contains(id, "plan") {
			sessionType = "plan"
		}
		p.sessions = append(p.sessions, sessionPickerEntry{
			ID:    id,
			Type:  sessionType,
			State: state,
		})
	}
	p.cursor = 0
	p.filter.Reset()
	p.filter.Focus()
	p.applyFilter()
}

// SetSize sets the panel dimensions.
func (p *SessionPickerPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
	if width > 10 {
		p.filter.SetWidth(width - 10)
	}
}

// SelectedSessionID returns the ID of the currently selected session.
func (p *SessionPickerPanel) SelectedSessionID() string {
	if len(p.filtered) == 0 || p.cursor >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.cursor].ID
}

// Update handles key events and returns an action.
func (p *SessionPickerPanel) Update(msg tea.KeyPressMsg) (tea.Cmd, SessionPickerAction) {
	switch msg.String() {
	case "esc":
		return nil, SessionPickerActionCancel
	case "enter":
		if len(p.filtered) > 0 {
			return nil, SessionPickerActionSelect
		}
		return nil, SessionPickerActionNone
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, SessionPickerActionNone
	case "down", "j":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
		return nil, SessionPickerActionNone
	default:
		var cmd tea.Cmd
		prevVal := p.filter.Value()
		p.filter, cmd = p.filter.Update(msg)
		if p.filter.Value() != prevVal {
			p.applyFilter()
		}
		return cmd, SessionPickerActionNone
	}
}

// applyFilter updates the filtered list.
func (p *SessionPickerPanel) applyFilter() {
	query := strings.ToLower(p.filter.Value())
	if query == "" {
		p.filtered = make([]sessionPickerEntry, len(p.sessions))
		copy(p.filtered, p.sessions)
	} else {
		p.filtered = nil
		for _, s := range p.sessions {
			if strings.Contains(strings.ToLower(s.ID), query) ||
				strings.Contains(strings.ToLower(s.Type), query) {
				p.filtered = append(p.filtered, s)
			}
		}
	}
	if p.cursor >= len(p.filtered) {
		p.cursor = 0
	}
}

// View renders the session picker.
func (p *SessionPickerPanel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2)

	var b strings.Builder

	b.WriteString(titleStyle.Render("Select bridge session"))
	b.WriteString("\n\n")
	b.WriteString(p.filter.View())
	b.WriteString("\n\n")

	if len(p.filtered) == 0 {
		if len(p.sessions) == 0 {
			b.WriteString(dimStyle.Render("No active bridge sessions"))
		} else {
			b.WriteString(dimStyle.Render("No matching sessions"))
		}
	} else {
		maxVisible := p.height - 8
		if maxVisible < 3 {
			maxVisible = 3
		}

		start := 0
		if p.cursor >= maxVisible {
			start = p.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(p.filtered) {
			end = len(p.filtered)
		}

		for i := start; i < end; i++ {
			s := p.filtered[i]
			stateIcon := "○"
			switch s.State {
			case bridge.SessionReady:
				stateIcon = "●"
			case bridge.SessionStreaming:
				stateIcon = "◉"
			case bridge.SessionDead:
				stateIcon = "✗"
			}
			label := fmt.Sprintf("%s [%s] %s", stateIcon, s.Type, s.ID)
			if i == p.cursor {
				b.WriteString(selectedStyle.Render(" > " + label + " "))
			} else {
				b.WriteString(normalStyle.Render("   " + label))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("↑↓/jk navigate • enter select • esc cancel"))

	maxWidth := p.width - 6
	if maxWidth < 30 {
		maxWidth = 30
	}
	return borderStyle.Width(maxWidth).Render(b.String())
}
