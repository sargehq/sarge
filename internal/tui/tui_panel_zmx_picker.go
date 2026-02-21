package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sargehq/sarge/internal/work"
)

// ZmxPickerAction represents an action result from the zmx session picker.
type ZmxPickerAction int

const (
	ZmxPickerActionNone   ZmxPickerAction = iota
	ZmxPickerActionCancel                 // User pressed Esc
	ZmxPickerActionSelect                 // User pressed Enter on a session
)

// zmxSessionsLoadedMsg is sent when zmx session listing completes.
type zmxSessionsLoadedMsg struct {
	sessions []work.WorkSession
	err      error
}

// zmxSessionAttachedMsg is sent when zmx session attach completes.
type zmxSessionAttachedMsg struct {
	sessionName string
	err         error
}

// ZmxPickerPanel shows a fuzzy-filterable list of zmx sessions for a work unit.
type ZmxPickerPanel struct {
	// Dimensions
	width  int
	height int

	// State
	sessions []work.WorkSession
	filtered []work.WorkSession
	cursor   int
	filter   textinput.Model
}

// NewZmxPickerPanel creates a new ZmxPickerPanel.
func NewZmxPickerPanel() *ZmxPickerPanel {
	ti := textinput.New()
	ti.Placeholder = "Filter sessions..."
	ti.CharLimit = 100
	ti.Width = 40

	return &ZmxPickerPanel{
		width:  60,
		height: 20,
		filter: ti,
	}
}

// Init initializes the panel.
func (p *ZmxPickerPanel) Init() tea.Cmd {
	return textinput.Blink
}

// SetSessions populates the picker with sessions and focuses the filter input.
func (p *ZmxPickerPanel) SetSessions(sessions []work.WorkSession) {
	p.sessions = sessions
	p.cursor = 0
	p.filter.Reset()
	p.filter.Focus()
	p.applyFilter()
}

// SetSize sets the panel dimensions.
func (p *ZmxPickerPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
	if width > 10 {
		p.filter.Width = width - 10
	}
}

// SelectedSession returns the currently selected session, or nil if none.
func (p *ZmxPickerPanel) SelectedSession() *work.WorkSession {
	if len(p.filtered) == 0 || p.cursor >= len(p.filtered) {
		return nil
	}
	s := p.filtered[p.cursor]
	return &s
}

// Update handles key events and returns an action.
func (p *ZmxPickerPanel) Update(msg tea.KeyMsg) (tea.Cmd, ZmxPickerAction) {
	switch msg.String() {
	case "esc":
		return nil, ZmxPickerActionCancel
	case "enter":
		if len(p.filtered) > 0 {
			return nil, ZmxPickerActionSelect
		}
		return nil, ZmxPickerActionNone
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
		return nil, ZmxPickerActionNone
	case "down", "j":
		if p.cursor < len(p.filtered)-1 {
			p.cursor++
		}
		return nil, ZmxPickerActionNone
	default:
		// Pass to text input for filtering
		var cmd tea.Cmd
		prevVal := p.filter.Value()
		p.filter, cmd = p.filter.Update(msg)
		if p.filter.Value() != prevVal {
			p.applyFilter()
		}
		return cmd, ZmxPickerActionNone
	}
}

// applyFilter updates the filtered list based on the current filter text.
func (p *ZmxPickerPanel) applyFilter() {
	query := strings.ToLower(p.filter.Value())
	if query == "" {
		p.filtered = make([]work.WorkSession, len(p.sessions))
		copy(p.filtered, p.sessions)
	} else {
		p.filtered = nil
		for _, s := range p.sessions {
			if strings.Contains(strings.ToLower(s.DisplayName), query) ||
				strings.Contains(strings.ToLower(s.Type), query) ||
				strings.Contains(strings.ToLower(s.TabName), query) {
				p.filtered = append(p.filtered, s)
			}
		}
	}
	// Reset cursor if out of bounds
	if p.cursor >= len(p.filtered) {
		p.cursor = 0
	}
}

// View renders the picker panel.
func (p *ZmxPickerPanel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12"))
	normalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Padding(1, 2)

	var b strings.Builder

	b.WriteString(titleStyle.Render("Select zmx session"))
	b.WriteString("\n\n")

	// Filter input
	b.WriteString(p.filter.View())
	b.WriteString("\n\n")

	if len(p.filtered) == 0 {
		if len(p.sessions) == 0 {
			b.WriteString(dimStyle.Render("No active sessions"))
		} else {
			b.WriteString(dimStyle.Render("No matching sessions"))
		}
	} else {
		// Calculate how many items we can show
		maxVisible := p.height - 8 // title + filter + borders + help
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
			label := s.DisplayName
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

	// Apply border
	maxWidth := p.width - 6
	if maxWidth < 30 {
		maxWidth = 30
	}
	content := borderStyle.Width(maxWidth).Render(b.String())
	return content
}
