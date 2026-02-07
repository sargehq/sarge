package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/github"
)

// PRImportAction represents an action result from the panel
type PRImportAction int

const (
	PRImportActionNone PRImportAction = iota
	PRImportActionCancel
	PRImportActionSubmit
	PRImportActionPreview
)

// PRImportResult contains form values when submitted
type PRImportResult struct {
	PRURL string
}

// PRImportPanel renders the PR import form.
type PRImportPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Form state
	input     textinput.Model
	focusIdx  int
	importing bool

	// Preview state
	previewing bool
	prMetadata *github.PRMetadata
	previewErr error

	// Mouse state
	hoveredButton string
}

// NewPRImportPanel creates a new PRImportPanel
func NewPRImportPanel() *PRImportPanel {
	input := textinput.New()
	input.Placeholder = "https://github.com/owner/repo/pull/123"
	input.CharLimit = 500
	input.Width = 60

	return &PRImportPanel{
		width:  60,
		height: 20,
		input:  input,
	}
}

// Init initializes the panel and returns any initial command
func (p *PRImportPanel) Init() tea.Cmd {
	return textinput.Blink
}

// Reset resets the form to initial state
func (p *PRImportPanel) Reset() {
	p.input.Reset()
	p.input.Focus()
	p.focusIdx = 0
	p.importing = false
	p.previewing = false
	p.prMetadata = nil
	p.previewErr = nil
}

// Update handles key events and returns an action
func (p *PRImportPanel) Update(msg tea.KeyMsg) (tea.Cmd, PRImportAction) {
	// Check escape/cancel keys
	if msg.Type == tea.KeyEsc || msg.String() == "esc" {
		p.input.Blur()
		return nil, PRImportActionCancel
	}

	// Tab cycles between elements: input(0) -> Import(1) -> Cancel(2)
	if msg.Type == tea.KeyTab || msg.String() == "tab" {
		// Leave text input focus before switching
		if p.focusIdx == 0 {
			p.input.Blur()
		}

		p.focusIdx = (p.focusIdx + 1) % 3

		// Enter new focus
		if p.focusIdx == 0 {
			p.input.Focus()
		}
		return nil, PRImportActionNone
	}

	// Shift+Tab goes backwards
	if msg.Type == tea.KeyShiftTab {
		// Leave text input focus before switching
		if p.focusIdx == 0 {
			p.input.Blur()
		}

		p.focusIdx--
		if p.focusIdx < 0 {
			p.focusIdx = 2
		}

		// Enter new focus
		if p.focusIdx == 0 {
			p.input.Focus()
		}
		return nil, PRImportActionNone
	}

	// Enter submits from input or activates buttons
	if msg.String() == "enter" {
		prURL := strings.TrimSpace(p.input.Value())
		if prURL == "" {
			return nil, PRImportActionNone
		}

		switch p.focusIdx {
		case 0: // Input field
			// If preview not loaded yet, load it; otherwise import
			if p.prMetadata == nil && !p.previewing {
				return nil, PRImportActionPreview
			}
			return nil, PRImportActionSubmit
		case 1: // Import button
			return nil, PRImportActionSubmit
		case 2: // Cancel button
			p.input.Blur()
			return nil, PRImportActionCancel
		}
	}

	// Handle input based on focused element
	if p.focusIdx == 0 {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd, PRImportActionNone
	}

	return nil, PRImportActionNone
}

// GetResult returns the current form values
func (p *PRImportPanel) GetResult() PRImportResult {
	return PRImportResult{
		PRURL: strings.TrimSpace(p.input.Value()),
	}
}

// SetImporting sets the importing state
func (p *PRImportPanel) SetImporting(importing bool) {
	p.importing = importing
}

// SetPreviewing sets the previewing state
func (p *PRImportPanel) SetPreviewing(previewing bool) {
	p.previewing = previewing
}

// SetPreviewResult sets the PR metadata preview result
func (p *PRImportPanel) SetPreviewResult(metadata *github.PRMetadata, err error) {
	p.prMetadata = metadata
	p.previewErr = err
	p.previewing = false
}

// Blur removes focus from the input
func (p *PRImportPanel) Blur() {
	p.input.Blur()
}

// SetSize updates the panel dimensions
func (p *PRImportPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *PRImportPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *PRImportPanel) IsFocused() bool {
	return p.focused
}

// SetHoveredButton updates which button is hovered
func (p *PRImportPanel) SetHoveredButton(button string) {
	p.hoveredButton = button
}

// Render returns the PR import form content
func (p *PRImportPanel) Render() string {
	var content strings.Builder

	// Adapt input width
	inputWidth := p.width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	p.input.Width = inputWidth

	// Show focus label with context-aware hint
	prURLLabel := "PR URL:"
	if p.focusIdx == 0 {
		if p.prMetadata != nil {
			prURLLabel = tuiValueStyle().Render("PR URL:") + " (Enter to import)"
		} else {
			prURLLabel = tuiValueStyle().Render("PR URL:") + " (Enter to load preview)"
		}
	}

	content.WriteString(tuiLabelStyle().Render("Import from GitHub PR"))
	content.WriteString("\n\n")
	content.WriteString(prURLLabel)
	content.WriteString("\n")
	content.WriteString(p.input.View())
	content.WriteString("\n\n")

	// Show PR preview if available
	if p.previewing {
		content.WriteString(tuiDimStyle().Render("Loading PR details..."))
		content.WriteString("\n\n")
	} else if p.previewErr != nil {
		content.WriteString(tuiErrorStyle().Render(fmt.Sprintf("Error: %v", p.previewErr)))
		content.WriteString("\n\n")
	} else if p.prMetadata != nil {
		content.WriteString(tuiLabelStyle().Render("PR Preview:"))
		content.WriteString("\n")
		content.WriteString(fmt.Sprintf("  #%d: %s\n", p.prMetadata.Number, tuiValueStyle().Render(p.prMetadata.Title)))
		content.WriteString(fmt.Sprintf("  Author: %s\n", p.prMetadata.Author))
		content.WriteString(fmt.Sprintf("  State: %s\n", formatPRState(p.prMetadata.State)))
		content.WriteString(fmt.Sprintf("  Branch: %s -> %s\n", p.prMetadata.HeadRefName, p.prMetadata.BaseRefName))
		if len(p.prMetadata.Labels) > 0 {
			content.WriteString(fmt.Sprintf("  Labels: %s\n", strings.Join(p.prMetadata.Labels, ", ")))
		}
		content.WriteString("\n")
	}

	// Render buttons (Import and Cancel only)
	var importLabel, cancelLabel string
	focusHint := ""

	if p.focusIdx == 1 {
		importLabel = tuiValueStyle().Render("[Import]")
		focusHint = tuiDimStyle().Render(" (press Enter)")
	} else {
		importLabel = styleButtonWithHover("Import", p.hoveredButton == "import")
	}

	if p.focusIdx == 2 {
		cancelLabel = tuiValueStyle().Render("[Cancel]")
		focusHint = tuiDimStyle().Render(" (press Enter)")
	} else {
		cancelLabel = styleButtonWithHover("Cancel", p.hoveredButton == "cancel")
	}

	content.WriteString(zone.Mark("dialog-import", importLabel) + "  " + zone.Mark("dialog-cancel", cancelLabel) + focusHint)
	content.WriteString("\n")

	if p.importing {
		content.WriteString(tuiDimStyle().Render("Importing..."))
	} else {
		content.WriteString(tuiDimStyle().Render("[Tab] Next field  [Esc] Cancel"))
	}

	return content.String()
}

// formatPRState formats the PR state with appropriate styling
func formatPRState(state string) string {
	switch state {
	case "OPEN":
		return tuiSuccessStyle().Render("OPEN")
	case "CLOSED":
		return tuiErrorStyle().Render("CLOSED")
	case "MERGED":
		return lipgloss.NewStyle().Foreground(CurrentTheme().Merged).Render("MERGED")
	default:
		return state
	}
}

// RenderWithPanel returns the panel with border styling
func (p *PRImportPanel) RenderWithPanel(contentHeight int) string {
	panelContent := p.Render()

	panelStyle := tuiPanelStyle().Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(CurrentTheme().Accent)
	}

	result := panelStyle.Render(tuiTitleStyle().Render("Import PR") + "\n" + panelContent)

	// If the result is taller than expected (due to lipgloss wrapping), fix it
	if lipgloss.Height(result) > contentHeight {
		lines := strings.Split(result, "\n")
		extraLines := len(lines) - contentHeight
		if extraLines > 0 && len(lines) > 3 {
			topBorder := lines[0]
			titleLine := lines[1]
			bottomBorder := lines[len(lines)-1]
			contentLines := lines[2 : len(lines)-1]
			keepContentLines := len(contentLines) - extraLines
			if keepContentLines < 1 {
				keepContentLines = 1
			}
			if keepContentLines < len(contentLines) {
				contentLines = contentLines[:keepContentLines]
			}
			lines = []string{topBorder, titleLine}
			lines = append(lines, contentLines...)
			lines = append(lines, bottomBorder)
			result = strings.Join(lines, "\n")
		}
	}

	return result
}
