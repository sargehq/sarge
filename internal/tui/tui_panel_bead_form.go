package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/beans"
)

// BeadFormMode indicates which mode the form is in
type BeadFormMode int

const (
	BeadFormModeCreate BeadFormMode = iota
	BeadFormModeAddChild
	BeadFormModeEdit
)

// BeadFormAction represents an action result from the panel
type BeadFormAction int

const (
	BeadFormActionNone BeadFormAction = iota
	BeadFormActionCancel
	BeadFormActionSubmit
)

// beadStatuses is the list of valid bead statuses for editing
var beadStatuses = []string{
	beans.StatusTodo,
	beans.StatusInProgress,
	beans.StatusDraft,
	beans.StatusScrapped,
	beans.StatusCompleted,
}

// beadPriorities is the ordered list of bean priority values for the form.
var beadPriorities = []string{
	beans.PriorityCritical,
	beans.PriorityHigh,
	beans.PriorityNormal,
	beans.PriorityLow,
	beans.PriorityDeferred,
}

// BeadFormResult contains form values when submitted
type BeadFormResult struct {
	Title       string
	Description string
	BeadType    string
	Priority    string
	Status      string // Only used in edit mode
	EditBeanID  string // Non-empty when editing
	ParentID    string // Non-empty when adding child
}

// BeadFormPanel renders the bead create/edit form.
type BeadFormPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Form mode
	mode       BeadFormMode
	editBeanID string
	parentID   string

	// Form state (owned directly)
	titleInput   textinput.Model
	descTextarea textarea.Model
	beadType     int
	priority     int
	status       int // Index into beadStatuses
	focusIdx     int

	// Mouse state
	hoveredButton string
}

// NewBeadFormPanel creates a new BeadFormPanel
func NewBeadFormPanel() *BeadFormPanel {
	titleInput := textinput.New()
	titleInput.Placeholder = "Enter title..."
	titleInput.CharLimit = 100
	titleInput.Width = 40

	descTextarea := textarea.New()
	descTextarea.Placeholder = "Enter description (optional)..."
	descTextarea.CharLimit = 32000
	descTextarea.SetWidth(60)
	descTextarea.SetHeight(4)

	return &BeadFormPanel{
		width:        60,
		height:       20,
		priority:     2,
		titleInput:   titleInput,
		descTextarea: descTextarea,
	}
}

// Init initializes the panel and returns any initial command
func (p *BeadFormPanel) Init() tea.Cmd {
	p.titleInput.Focus()
	return textinput.Blink
}

// Reset resets the form to initial state for creating a new bead
func (p *BeadFormPanel) Reset() {
	p.titleInput.Reset()
	p.titleInput.Focus()
	p.descTextarea.Reset()
	p.beadType = 0
	p.priority = 2
	p.focusIdx = 0
	p.mode = BeadFormModeCreate
	p.editBeanID = ""
	p.parentID = ""
}

// SetEditMode configures the form for editing an existing bean
func (p *BeadFormPanel) SetEditMode(beanID, title, description, beadType string, priority string, status string) {
	p.mode = BeadFormModeEdit
	p.editBeanID = beanID
	p.parentID = ""
	p.titleInput.SetValue(title)
	p.titleInput.Focus()
	p.descTextarea.SetValue(description)
	// Find the type index
	p.beadType = 0
	for i, t := range beadTypes {
		if t == beadType {
			p.beadType = i
			break
		}
	}
	// Find the priority index
	p.priority = 2 // default to normal
	for i, pr := range beadPriorities {
		if pr == priority {
			p.priority = i
			break
		}
	}
	// Find the status index
	p.status = 0
	for i, s := range beadStatuses {
		if s == status {
			p.status = i
			break
		}
	}
	p.focusIdx = 0
}

// SetAddChildMode configures the form for adding a child bead
func (p *BeadFormPanel) SetAddChildMode(parentID string) {
	p.Reset()
	p.mode = BeadFormModeAddChild
	p.parentID = parentID
}

// Update handles key events and returns an action
func (p *BeadFormPanel) Update(msg tea.KeyMsg) (tea.Cmd, BeadFormAction) {
	// Check escape/cancel keys
	if msg.Type == tea.KeyEsc || msg.String() == "esc" {
		p.titleInput.Blur()
		p.descTextarea.Blur()
		return nil, BeadFormActionCancel
	}

	// Focus indices:
	// Create/AddChild mode: title(0) -> type(1) -> priority(2) -> description(3) -> ok(4) -> cancel(5)
	// Edit mode: title(0) -> type(1) -> priority(2) -> status(3) -> description(4) -> ok(5) -> cancel(6)
	maxFocusIdx := 5
	descIdx := 3
	okIdx := 4
	cancelIdx := 5
	if p.mode == BeadFormModeEdit {
		maxFocusIdx = 6
		descIdx = 4
		okIdx = 5
		cancelIdx = 6
	}

	// Tab cycles between elements
	if msg.Type == tea.KeyTab || msg.String() == "tab" {
		// Leave current focus
		if p.focusIdx == 0 {
			p.titleInput.Blur()
		} else if p.focusIdx == descIdx {
			p.descTextarea.Blur()
		}

		p.focusIdx = (p.focusIdx + 1) % (maxFocusIdx + 1)

		// Enter new focus
		if p.focusIdx == 0 {
			p.titleInput.Focus()
		} else if p.focusIdx == descIdx {
			p.descTextarea.Focus()
		}
		return nil, BeadFormActionNone
	}

	// Shift+Tab goes backwards
	if msg.Type == tea.KeyShiftTab {
		// Leave current focus
		if p.focusIdx == 0 {
			p.titleInput.Blur()
		} else if p.focusIdx == descIdx {
			p.descTextarea.Blur()
		}

		p.focusIdx--
		if p.focusIdx < 0 {
			p.focusIdx = maxFocusIdx
		}

		// Enter new focus
		if p.focusIdx == 0 {
			p.titleInput.Focus()
		} else if p.focusIdx == descIdx {
			p.descTextarea.Focus()
		}
		return nil, BeadFormActionNone
	}

	// Enter key handling depends on focused element
	if msg.String() == "enter" {
		switch p.focusIdx {
		case 0, 1, 2, 3: // Title, type, priority, or status - submit form (if not on description)
			if p.focusIdx != descIdx {
				title := strings.TrimSpace(p.titleInput.Value())
				if title != "" {
					return nil, BeadFormActionSubmit
				}
				return nil, BeadFormActionNone
			}
		}
		if p.focusIdx == okIdx { // Ok button - submit form
			title := strings.TrimSpace(p.titleInput.Value())
			if title != "" {
				return nil, BeadFormActionSubmit
			}
			return nil, BeadFormActionNone
		}
		if p.focusIdx == cancelIdx { // Cancel button - cancel form
			p.titleInput.Blur()
			p.descTextarea.Blur()
			return nil, BeadFormActionCancel
		}
		// For description textarea, Enter adds a newline (handled below)
	}

	// Ctrl+Enter submits from description textarea
	if msg.String() == "ctrl+enter" && p.focusIdx == descIdx {
		title := strings.TrimSpace(p.titleInput.Value())
		if title != "" {
			return nil, BeadFormActionSubmit
		}
		return nil, BeadFormActionNone
	}

	// Handle input based on focused element
	// In edit mode, status is at index 3, otherwise description is at index 3
	statusIdx := -1 // Not available in create/add-child modes
	if p.mode == BeadFormModeEdit {
		statusIdx = 3
	}

	switch p.focusIdx {
	case 0: // Title input
		var cmd tea.Cmd
		p.titleInput, cmd = p.titleInput.Update(msg)
		return cmd, BeadFormActionNone

	case 1: // Type selector
		switch msg.String() {
		case "j", "down", "right":
			p.beadType = (p.beadType + 1) % len(beadTypes)
		case "k", "up", "left":
			p.beadType--
			if p.beadType < 0 {
				p.beadType = len(beadTypes) - 1
			}
		}
		return nil, BeadFormActionNone

	case 2: // Priority
		switch msg.String() {
		case "j", "down", "right", "-":
			if p.priority < len(beadPriorities)-1 {
				p.priority++
			}
		case "k", "up", "left", "+", "=":
			if p.priority > 0 {
				p.priority--
			}
		}
		return nil, BeadFormActionNone

	default:
		// Handle dynamic indices based on mode
		if p.focusIdx == statusIdx {
			// Status selector (edit mode only)
			switch msg.String() {
			case "j", "down", "right":
				p.status = (p.status + 1) % len(beadStatuses)
			case "k", "up", "left":
				p.status--
				if p.status < 0 {
					p.status = len(beadStatuses) - 1
				}
			}
			return nil, BeadFormActionNone
		}

		if p.focusIdx == descIdx {
			// Description textarea
			var cmd tea.Cmd
			p.descTextarea, cmd = p.descTextarea.Update(msg)
			return cmd, BeadFormActionNone
		}

		if p.focusIdx == okIdx {
			// Ok button - Space can also activate it
			if msg.String() == " " {
				title := strings.TrimSpace(p.titleInput.Value())
				if title != "" {
					return nil, BeadFormActionSubmit
				}
			}
			return nil, BeadFormActionNone
		}

		if p.focusIdx == cancelIdx {
			// Cancel button - Space can also activate it
			if msg.String() == " " {
				p.titleInput.Blur()
				p.descTextarea.Blur()
				return nil, BeadFormActionCancel
			}
			return nil, BeadFormActionNone
		}
	}

	return nil, BeadFormActionNone
}

// GetResult returns the current form values
func (p *BeadFormPanel) GetResult() BeadFormResult {
	return BeadFormResult{
		Title:       strings.TrimSpace(p.titleInput.Value()),
		Description: strings.TrimSpace(p.descTextarea.Value()),
		BeadType:    beadTypes[p.beadType],
		Priority:    beadPriorities[p.priority],
		Status:      beadStatuses[p.status],
		EditBeanID:  p.editBeanID,
		ParentID:    p.parentID,
	}
}

// GetMode returns the current form mode
func (p *BeadFormPanel) GetMode() BeadFormMode {
	return p.mode
}

// Blur removes focus from all inputs
func (p *BeadFormPanel) Blur() {
	p.titleInput.Blur()
	p.descTextarea.Blur()
}

// SetSize updates the panel dimensions
func (p *BeadFormPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *BeadFormPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *BeadFormPanel) IsFocused() bool {
	return p.focused
}

// SetMode updates the form mode (deprecated - use Reset/SetEditMode/SetAddChildMode)
func (p *BeadFormPanel) SetMode(mode BeadFormMode, editBeanID, parentID string) {
	p.mode = mode
	p.editBeanID = editBeanID
	p.parentID = parentID
}

// SetFormState updates the form state (deprecated - panel owns its state now)
func (p *BeadFormPanel) SetFormState(
	titleInput *textinput.Model,
	descTextarea *textarea.Model,
	beadType int,
	priority int,
	focusIdx int,
) {
	// No-op: panel owns its own state now
	// This method is kept for backwards compatibility during migration
}

// SetHoveredButton updates which button is hovered
func (p *BeadFormPanel) SetHoveredButton(button string) {
	p.hoveredButton = button
}

// Render returns the bead form content
func (p *BeadFormPanel) Render(visibleLines int) string {
	var content strings.Builder

	// Adapt input widths to available space
	inputWidth := p.width - 4
	if inputWidth < 20 {
		inputWidth = 20
	}
	p.titleInput.Width = inputWidth
	p.descTextarea.SetWidth(inputWidth)
	// Calculate dynamic height for description textarea
	descHeight := max(visibleLines-12, 4)
	p.descTextarea.SetHeight(descHeight)

	// Calculate dynamic focus indices based on mode
	// Create/AddChild mode: title(0) -> type(1) -> priority(2) -> description(3) -> ok(4) -> cancel(5)
	// Edit mode: title(0) -> type(1) -> priority(2) -> status(3) -> description(4) -> ok(5) -> cancel(6)
	statusIdx := -1
	descIdx := 3
	okIdx := 4
	cancelIdx := 5
	if p.mode == BeadFormModeEdit {
		statusIdx = 3
		descIdx = 4
		okIdx = 5
		cancelIdx = 6
	}

	typeFocused := p.focusIdx == 1
	priorityFocused := p.focusIdx == 2
	statusFocused := p.focusIdx == statusIdx
	descFocused := p.focusIdx == descIdx

	// Type rotator display
	currentType := beadTypes[p.beadType]
	var typeDisplay string
	if typeFocused {
		typeDisplay = fmt.Sprintf("< %s >", tuiValueStyle.Render(currentType))
	} else {
		typeDisplay = typeFeatureStyle.Render(currentType)
	}

	// Priority display
	currentPriority := beadPriorities[p.priority]
	var priorityDisplay string
	if priorityFocused {
		priorityDisplay = fmt.Sprintf("< %s >", tuiValueStyle.Render(currentPriority))
	} else {
		priorityDisplay = currentPriority
	}

	// Status display (edit mode only)
	var statusDisplay string
	if p.mode == BeadFormModeEdit {
		currentStatus := beadStatuses[p.status]
		if statusFocused {
			statusDisplay = fmt.Sprintf("< %s >", tuiValueStyle.Render(currentStatus))
		} else {
			statusDisplay = currentStatus
		}
	}

	// Show focus labels
	titleLabel := "Title:"
	typeLabel := "Type:"
	priorityLabel := "Priority:"
	statusLabel := "Status:"
	descLabel := "Description:"
	if p.focusIdx == 0 {
		titleLabel = tuiValueStyle.Render("Title:") + " (editing)"
	}
	if typeFocused {
		typeLabel = tuiValueStyle.Render("Type:") + " (j/k)"
	}
	if priorityFocused {
		priorityLabel = tuiValueStyle.Render("Priority:") + " (j/k)"
	}
	if statusFocused {
		statusLabel = tuiValueStyle.Render("Status:") + " (j/k)"
	}
	if descFocused {
		descLabel = tuiValueStyle.Render("Description:") + " (optional)"
	}

	// Determine mode and render appropriate header
	var header string
	switch p.mode {
	case BeadFormModeEdit:
		header = "Edit Issue " + issueIDStyle.Render(p.editBeanID)
	case BeadFormModeAddChild:
		// Include parent on same line to save vertical space
		header = "Add Child to " + tuiValueStyle.Render(p.parentID)
	default:
		header = "Create New Issue"
	}

	content.WriteString(tuiLabelStyle.Render(header))
	content.WriteString("\n")

	// Render form fields
	content.WriteString("\n")
	content.WriteString(titleLabel)
	content.WriteString("\n")
	content.WriteString(p.titleInput.View())
	content.WriteString("\n\n")
	content.WriteString(typeLabel + " " + typeDisplay)
	content.WriteString("\n")
	content.WriteString(priorityLabel + " " + priorityDisplay)
	content.WriteString("\n")

	// Show status field only in edit mode
	if p.mode == BeadFormModeEdit {
		content.WriteString(statusLabel + " " + statusDisplay)
		content.WriteString("\n")
	}

	content.WriteString("\n")
	content.WriteString(descLabel)
	content.WriteString("\n")
	content.WriteString(p.descTextarea.View())
	content.WriteString("\n\n")

	// Render Ok and Cancel buttons with zone markers for click detection
	okFocused := p.focusIdx == okIdx
	cancelFocused := p.focusIdx == cancelIdx
	okButton := zone.Mark("dialog-ok", styleButtonWithHover("  Ok  ", p.hoveredButton == "ok" || okFocused))
	cancelButton := zone.Mark("dialog-cancel", styleButtonWithHover("Cancel", p.hoveredButton == "cancel" || cancelFocused))

	content.WriteString(okButton + "  " + cancelButton)
	content.WriteString("\n")
	content.WriteString(tuiDimStyle.Render("[Tab] Next  [Enter/Space] Select"))

	return content.String()
}

// RenderWithPanel returns the panel with border styling
func (p *BeadFormPanel) RenderWithPanel(contentHeight int) string {
	panelContent := p.Render(contentHeight - 3)

	panelStyle := tuiPanelStyle.Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(lipgloss.Color("214"))
	}

	// Determine title based on mode
	var title string
	switch p.mode {
	case BeadFormModeEdit:
		title = "Edit Issue"
	case BeadFormModeAddChild:
		title = "Add Child"
	default:
		title = "Create Issue"
	}

	result := panelStyle.Render(tuiTitleStyle.Render(title) + "\n" + panelContent)

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
