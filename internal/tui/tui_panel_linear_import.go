package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// LinearImportAction represents an action result from the panel
type LinearImportAction int

const (
	LinearImportActionNone LinearImportAction = iota
	LinearImportActionCancel
	LinearImportActionSubmit
)

// LinearImportResult contains form values when submitted
type LinearImportResult struct {
	IssueIDs   string
	CreateDeps bool
	Update     bool
	DryRun     bool
	MaxDepth   int
}

// LinearImportPanel renders the Linear import form.
type LinearImportPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Form state (owned directly)
	input      textarea.Model
	createDeps bool
	update     bool
	dryRun     bool
	maxDepth   int
	focusIdx   int
	importing  bool

	// Mouse state
	hoveredButton string
}

// NewLinearImportPanel creates a new LinearImportPanel
func NewLinearImportPanel() *LinearImportPanel {
	input := textarea.New()
	input.Placeholder = "Enter Linear issue IDs or URLs (one per line)..."
	input.CharLimit = 2000
	input.SetWidth(60)
	input.SetHeight(4)

	return &LinearImportPanel{
		width:    60,
		height:   20,
		maxDepth: 2,
		input:    input,
	}
}

// Init initializes the panel and returns any initial command
func (p *LinearImportPanel) Init() tea.Cmd {
	return textarea.Blink
}

// Reset resets the form to initial state
func (p *LinearImportPanel) Reset() {
	p.input.Reset()
	p.input.Focus()
	p.focusIdx = 0
	p.createDeps = false
	p.update = false
	p.dryRun = false
	p.maxDepth = 2
	p.importing = false
}

// Update handles key events and returns an action
func (p *LinearImportPanel) Update(msg tea.KeyMsg) (tea.Cmd, LinearImportAction) {
	// Check escape/cancel keys
	if msg.Type == tea.KeyEsc || msg.String() == "esc" {
		p.input.Blur()
		return nil, LinearImportActionCancel
	}

	// Tab cycles between elements: input(0) -> createDeps(1) -> update(2) -> dryRun(3) -> maxDepth(4) -> Ok(5) -> Cancel(6)
	if msg.Type == tea.KeyTab || msg.String() == "tab" {
		// Leave textarea focus before switching
		if p.focusIdx == 0 {
			p.input.Blur()
		}

		p.focusIdx = (p.focusIdx + 1) % 7

		// Enter new focus
		if p.focusIdx == 0 {
			p.input.Focus()
		}
		return nil, LinearImportActionNone
	}

	// Shift+Tab goes backwards
	if msg.Type == tea.KeyShiftTab {
		// Leave textarea focus before switching
		if p.focusIdx == 0 {
			p.input.Blur()
		}

		p.focusIdx--
		if p.focusIdx < 0 {
			p.focusIdx = 6
		}

		// Enter new focus
		if p.focusIdx == 0 {
			p.input.Focus()
		}
		return nil, LinearImportActionNone
	}

	// Ctrl+Enter submits from textarea
	if msg.String() == "ctrl+enter" && p.focusIdx == 0 {
		issueIDs := strings.TrimSpace(p.input.Value())
		if issueIDs != "" {
			return nil, LinearImportActionSubmit
		}
		return nil, LinearImportActionNone
	}

	// Enter or Space activates buttons and submits from other fields (but not from textarea - use Ctrl+Enter there)
	if (msg.String() == "enter" || msg.String() == " ") && p.focusIdx != 0 {
		// Handle Ok button (focus = 5)
		if p.focusIdx == 5 {
			issueIDs := strings.TrimSpace(p.input.Value())
			if issueIDs != "" {
				return nil, LinearImportActionSubmit
			}
			return nil, LinearImportActionNone
		}
		// Handle Cancel button (focus = 6)
		if p.focusIdx == 6 {
			p.input.Blur()
			return nil, LinearImportActionCancel
		}
		// Only Enter (not space) submits the form from other non-textarea fields
		if msg.String() == "enter" {
			issueIDs := strings.TrimSpace(p.input.Value())
			if issueIDs != "" {
				return nil, LinearImportActionSubmit
			}
		}
		return nil, LinearImportActionNone
	}

	// Handle input based on focused element
	switch p.focusIdx {
	case 0: // Textarea field
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd, LinearImportActionNone

	case 1: // Create dependencies checkbox
		if msg.String() == " " || msg.String() == "x" {
			p.createDeps = !p.createDeps
		}
		return nil, LinearImportActionNone

	case 2: // Update existing checkbox
		if msg.String() == " " || msg.String() == "x" {
			p.update = !p.update
		}
		return nil, LinearImportActionNone

	case 3: // Dry run checkbox
		if msg.String() == " " || msg.String() == "x" {
			p.dryRun = !p.dryRun
		}
		return nil, LinearImportActionNone

	case 4: // Max depth
		switch msg.String() {
		case "j", "down", "-":
			if p.maxDepth > 1 {
				p.maxDepth--
			}
		case "k", "up", "+", "=":
			if p.maxDepth < 5 {
				p.maxDepth++
			}
		}
		return nil, LinearImportActionNone
	}

	return nil, LinearImportActionNone
}

// GetResult returns the current form values
func (p *LinearImportPanel) GetResult() LinearImportResult {
	return LinearImportResult{
		IssueIDs:   strings.TrimSpace(p.input.Value()),
		CreateDeps: p.createDeps,
		Update:     p.update,
		DryRun:     p.dryRun,
		MaxDepth:   p.maxDepth,
	}
}

// SetImporting sets the importing state
func (p *LinearImportPanel) SetImporting(importing bool) {
	p.importing = importing
}

// Blur removes focus from the input
func (p *LinearImportPanel) Blur() {
	p.input.Blur()
}

// SetSize updates the panel dimensions
func (p *LinearImportPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *LinearImportPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *LinearImportPanel) IsFocused() bool {
	return p.focused
}

// SetHoveredButton updates which button is hovered
func (p *LinearImportPanel) SetHoveredButton(button string) {
	p.hoveredButton = button
}

// Render returns the Linear import form content
func (p *LinearImportPanel) Render() string {
	var content strings.Builder

	// Adapt textarea width
	inputWidth := p.width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	p.input.SetWidth(inputWidth)

	// Show focus labels
	issueIDsLabel := "Issue IDs/URLs:"
	createDepsLabel := "Create Dependencies:"
	updateLabel := "Update Existing:"
	dryRunLabel := "Dry Run:"
	maxDepthLabel := "Max Dependency Depth:"

	if p.focusIdx == 0 {
		issueIDsLabel = tuiValueStyle().Render("Issue IDs/URLs:") + " (one per line, Ctrl+Enter to submit)"
	}
	if p.focusIdx == 1 {
		createDepsLabel = tuiValueStyle().Render("Create Dependencies:") + " (space to toggle)"
	}
	if p.focusIdx == 2 {
		updateLabel = tuiValueStyle().Render("Update Existing:") + " (space to toggle)"
	}
	if p.focusIdx == 3 {
		dryRunLabel = tuiValueStyle().Render("Dry Run:") + " (space to toggle)"
	}
	if p.focusIdx == 4 {
		maxDepthLabel = tuiValueStyle().Render("Max Dependency Depth:") + " (+/- adjust)"
	}

	// Checkbox display
	createDepsCheck := " "
	updateCheck := " "
	dryRunCheck := " "
	if p.createDeps {
		createDepsCheck = "x"
	}
	if p.update {
		updateCheck = "x"
	}
	if p.dryRun {
		dryRunCheck = "x"
	}

	content.WriteString(tuiLabelStyle().Render("Import from Linear (Bulk)"))
	content.WriteString("\n\n")
	content.WriteString(issueIDsLabel)
	content.WriteString("\n")
	content.WriteString(p.input.View())
	content.WriteString("\n\n")
	content.WriteString(createDepsLabel + " [" + createDepsCheck + "]")
	content.WriteString("\n")
	content.WriteString(updateLabel + " [" + updateCheck + "]")
	content.WriteString("\n")
	content.WriteString(dryRunLabel + " [" + dryRunCheck + "]")
	content.WriteString("\n\n")
	content.WriteString(maxDepthLabel + " " + tuiValueStyle().Render(fmt.Sprintf("%d", p.maxDepth)))
	content.WriteString("\n\n")

	// Render Ok and Cancel buttons
	var okLabel string
	var cancelLabel string
	focusHint := ""

	if p.focusIdx == 5 {
		okLabel = tuiValueStyle().Render("[ Ok ]")
		focusHint = tuiDimStyle().Render(" (press Enter to import)")
	} else {
		okLabel = styleButtonWithHover("  Ok  ", p.hoveredButton == "ok")
	}

	if p.focusIdx == 6 {
		cancelLabel = tuiValueStyle().Render("[Cancel]")
		focusHint = tuiDimStyle().Render(" (press Enter to cancel)")
	} else {
		cancelLabel = styleButtonWithHover("Cancel", p.hoveredButton == "cancel")
	}

	content.WriteString(zone.Mark("dialog-ok", okLabel) + "  " + zone.Mark("dialog-cancel", cancelLabel) + focusHint)
	content.WriteString("\n")

	if p.importing {
		content.WriteString(tuiDimStyle().Render("Importing..."))
	} else {
		content.WriteString(tuiDimStyle().Render("[Tab] Next field  [Enter] Activate"))
	}

	return content.String()
}

// RenderWithPanel returns the panel with border styling
func (p *LinearImportPanel) RenderWithPanel(contentHeight int) string {
	panelContent := p.Render()

	panelStyle := tuiPanelStyle().Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(lipgloss.Color("214"))
	}

	result := panelStyle.Render(tuiTitleStyle().Render("Linear Import") + "\n" + panelContent)

	// If the result is taller than expected (due to lipgloss wrapping), fix it
	// by removing extra lines from the INNER content while preserving borders and title
	if lipgloss.Height(result) > contentHeight {
		lines := strings.Split(result, "\n")
		extraLines := len(lines) - contentHeight
		// Need at least 4 lines: top border, title, 1+ content, bottom border
		if extraLines > 0 && len(lines) > 3 {
			// Keep first line (top border), second line (title), and last line (bottom border)
			// Remove extra lines from content area only
			topBorder := lines[0]
			titleLine := lines[1]
			bottomBorder := lines[len(lines)-1]
			// Content is from lines[2] to lines[len-2]
			contentLines := lines[2 : len(lines)-1]
			// Calculate how many content lines we can keep
			keepContentLines := len(contentLines) - extraLines
			if keepContentLines < 1 {
				keepContentLines = 1 // Always show at least 1 content line
			}
			// Truncate content from the end
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
