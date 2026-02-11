package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// CreateWorkAction represents an action result from the panel
type CreateWorkAction int

const (
	CreateWorkActionNone CreateWorkAction = iota
	CreateWorkActionCancel
	CreateWorkActionExecute
	CreateWorkActionAuto
)

// CreateWorkResult contains form values when submitted
type CreateWorkResult struct {
	BranchName        string
	BeadID            string
	UseExistingBranch bool
}

// CreateWorkPanel renders the work creation form.
type CreateWorkPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Form state (owned directly)
	beadID      string
	branchInput textinput.Model
	fieldIdx    int // 0=mode toggle, 1=branch input/selector, 2=buttons
	buttonIdx   int // 0=Execute, 1=Auto, 2=Cancel

	// Branch mode selection
	useExistingBranch   bool     // true = select existing branch, false = create new
	branches            []string // all available branches
	filteredBranches    []string // branches matching filter
	branchFilter        string   // current filter text
	selectedBranchIdx   int      // selected index in filteredBranches
	branchScrollOffset  int      // scroll offset for branch list
	maxVisibleBranches  int      // max branches visible at once

	// Mouse state
	hoveredButton string
}

// NewCreateWorkPanel creates a new CreateWorkPanel
func NewCreateWorkPanel() *CreateWorkPanel {
	branchInput := textinput.New()
	branchInput.Placeholder = "Branch name..."
	branchInput.CharLimit = 100
	branchInput.Width = 60

	return &CreateWorkPanel{
		width:              60,
		height:             20,
		branchInput:        branchInput,
		maxVisibleBranches: 8,
	}
}

// Init initializes the panel and returns any initial command
func (p *CreateWorkPanel) Init() tea.Cmd {
	p.branchInput.Focus()
	return textinput.Blink
}

// Reset resets the form to initial state
func (p *CreateWorkPanel) Reset(beadID string, branchName string) {
	p.beadID = beadID
	p.branchInput.SetValue(branchName)
	p.branchInput.Focus()
	p.fieldIdx = 0
	p.buttonIdx = 0

	// Reset branch selection state
	p.useExistingBranch = false
	p.branches = nil
	p.filteredBranches = nil
	p.branchFilter = ""
	p.selectedBranchIdx = 0
	p.branchScrollOffset = 0
}

// SetBranches sets the available branches for selection
func (p *CreateWorkPanel) SetBranches(branches []string) {
	p.branches = branches
	p.applyBranchFilter()
}

// applyBranchFilter filters branches based on current filter text
func (p *CreateWorkPanel) applyBranchFilter() {
	if p.branchFilter == "" {
		p.filteredBranches = p.branches
	} else {
		filterLower := strings.ToLower(p.branchFilter)
		p.filteredBranches = nil
		for _, b := range p.branches {
			if strings.Contains(strings.ToLower(b), filterLower) {
				p.filteredBranches = append(p.filteredBranches, b)
			}
		}
	}

	// Reset selection if out of bounds
	if p.selectedBranchIdx >= len(p.filteredBranches) {
		p.selectedBranchIdx = 0
	}
	p.branchScrollOffset = 0
}

// Update handles key events and returns an action
func (p *CreateWorkPanel) Update(msg tea.KeyMsg) (tea.Cmd, CreateWorkAction) {
	if msg.Type == tea.KeyEsc {
		p.branchInput.Blur()
		return nil, CreateWorkActionCancel
	}

	// Tab cycles between mode(0), branch(1), buttons(2)
	if msg.Type == tea.KeyTab {
		p.fieldIdx = (p.fieldIdx + 1) % 3
		p.updateFocus()
		return nil, CreateWorkActionNone
	}

	// Shift+Tab goes backwards
	if msg.Type == tea.KeyShiftTab {
		p.fieldIdx--
		if p.fieldIdx < 0 {
			p.fieldIdx = 2
		}
		p.updateFocus()
		return nil, CreateWorkActionNone
	}

	// Handle input based on focused field
	var cmd tea.Cmd
	switch p.fieldIdx {
	case 0: // Mode toggle
		switch msg.String() {
		case "enter", " ", "left", "right", "h", "l":
			p.useExistingBranch = !p.useExistingBranch
			// Reset filter when switching modes
			p.branchFilter = ""
			p.applyBranchFilter()
		}
	case 1: // Branch input or selector
		if p.useExistingBranch {
			p.updateBranchSelector(msg)
		} else {
			p.branchInput, cmd = p.branchInput.Update(msg)
		}
	case 2: // Buttons
		switch msg.String() {
		case "k", "up":
			p.buttonIdx--
			if p.buttonIdx < 0 {
				p.buttonIdx = 2
			}
		case "j", "down":
			p.buttonIdx = (p.buttonIdx + 1) % 3
		case "enter":
			branchName := p.getSelectedBranchName()
			if branchName == "" {
				// Empty branch name - don't submit
				return nil, CreateWorkActionNone
			}
			switch p.buttonIdx {
			case 0: // Execute
				return nil, CreateWorkActionExecute
			case 1: // Auto
				return nil, CreateWorkActionAuto
			case 2: // Cancel
				p.branchInput.Blur()
				return nil, CreateWorkActionCancel
			}
		}
	}
	return cmd, CreateWorkActionNone
}

// updateFocus updates focus state based on current field index
func (p *CreateWorkPanel) updateFocus() {
	if p.fieldIdx == 1 && !p.useExistingBranch {
		p.branchInput.Focus()
	} else {
		p.branchInput.Blur()
	}
}

// updateBranchSelector handles key events for the branch selector
func (p *CreateWorkPanel) updateBranchSelector(msg tea.KeyMsg) {
	switch msg.String() {
	case "k", "up":
		if p.selectedBranchIdx > 0 {
			p.selectedBranchIdx--
			// Scroll up if needed
			if p.selectedBranchIdx < p.branchScrollOffset {
				p.branchScrollOffset = p.selectedBranchIdx
			}
		}
	case "j", "down":
		if p.selectedBranchIdx < len(p.filteredBranches)-1 {
			p.selectedBranchIdx++
			// Scroll down if needed
			if p.selectedBranchIdx >= p.branchScrollOffset+p.maxVisibleBranches {
				p.branchScrollOffset = p.selectedBranchIdx - p.maxVisibleBranches + 1
			}
		}
	case "backspace":
		// Remove last character from filter
		if len(p.branchFilter) > 0 {
			p.branchFilter = p.branchFilter[:len(p.branchFilter)-1]
			p.applyBranchFilter()
		}
	default:
		// Add typed characters to filter
		if len(msg.String()) == 1 && msg.String() >= " " && msg.String() <= "~" {
			p.branchFilter += msg.String()
			p.applyBranchFilter()
		}
	}
}

// getSelectedBranchName returns the currently selected branch name
func (p *CreateWorkPanel) getSelectedBranchName() string {
	if p.useExistingBranch {
		if p.selectedBranchIdx < len(p.filteredBranches) {
			return p.filteredBranches[p.selectedBranchIdx]
		}
		return ""
	}
	return strings.TrimSpace(p.branchInput.Value())
}

// GetResult returns the current form values
func (p *CreateWorkPanel) GetResult() CreateWorkResult {
	return CreateWorkResult{
		BranchName:        p.getSelectedBranchName(),
		BeadID:            p.beadID,
		UseExistingBranch: p.useExistingBranch,
	}
}

// GetBeadID returns the bead ID for this work
func (p *CreateWorkPanel) GetBeadID() string {
	return p.beadID
}

// Blur removes focus from the input
func (p *CreateWorkPanel) Blur() {
	p.branchInput.Blur()
}

// SetSize updates the panel dimensions
func (p *CreateWorkPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

// SetFocus updates the focus state
func (p *CreateWorkPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *CreateWorkPanel) IsFocused() bool {
	return p.focused
}

// SetFormState updates the form state (deprecated - panel owns its state now)
func (p *CreateWorkPanel) SetFormState(
	beadID string,
	branchInput *textinput.Model,
	fieldIdx int,
	buttonIdx int,
) {
	// No-op: panel owns its own state now
	// This method is kept for backwards compatibility during migration
}

// SetHoveredButton updates which button is hovered
func (p *CreateWorkPanel) SetHoveredButton(button string) {
	p.hoveredButton = button
}

// Render returns the work creation form content
func (p *CreateWorkPanel) Render() string {
	var content strings.Builder

	// Panel header
	content.WriteString(tuiSuccessStyle().Render("Create Work"))
	content.WriteString("\n\n")

	// Show bead info
	beadInfo := fmt.Sprintf("Creating work from issue: %s", issueIDStyle().Render(p.beadID))
	content.WriteString(beadInfo)
	content.WriteString("\n\n")

	// Mode toggle
	var modeLabel string
	if p.fieldIdx == 0 {
		modeLabel = tuiSuccessStyle().Render("Branch mode:") + " " + tuiDimStyle().Render("(press Enter/Space to toggle)")
	} else {
		modeLabel = tuiLabelStyle().Render("Branch mode:")
	}
	content.WriteString(modeLabel)
	content.WriteString("\n")

	// Mode options
	newBranchStyle := tuiDimStyle()
	existingBranchStyle := tuiDimStyle()
	if !p.useExistingBranch {
		newBranchStyle = tuiSelectedStyle()
	} else {
		existingBranchStyle = tuiSelectedStyle()
	}
	content.WriteString("  " + newBranchStyle.Render("[New branch]") + "  " + existingBranchStyle.Render("[Existing branch]"))
	content.WriteString("\n\n")

	// Branch input or selector based on mode
	if p.useExistingBranch {
		// Existing branch selector
		var branchLabel string
		if p.fieldIdx == 1 {
			branchLabel = tuiSuccessStyle().Render("Select branch:") + " " + tuiDimStyle().Render("(type to filter, j/k to navigate)")
		} else {
			branchLabel = tuiLabelStyle().Render("Select branch:")
		}
		content.WriteString(branchLabel)
		content.WriteString("\n")

		// Show filter if active
		if p.branchFilter != "" {
			content.WriteString(tuiDimStyle().Render("Filter: ") + p.branchFilter + tuiDimStyle().Render("_"))
			content.WriteString("\n")
		}

		// Show branches
		if len(p.filteredBranches) == 0 {
			if len(p.branches) == 0 {
				content.WriteString(tuiDimStyle().Render("  (loading branches...)"))
			} else {
				content.WriteString(tuiDimStyle().Render("  (no matching branches)"))
			}
			content.WriteString("\n")
		} else {
			// Determine visible range
			endIdx := p.branchScrollOffset + p.maxVisibleBranches
			if endIdx > len(p.filteredBranches) {
				endIdx = len(p.filteredBranches)
			}

			// Show scroll indicator if needed
			if p.branchScrollOffset > 0 {
				content.WriteString(tuiDimStyle().Render("  ↑ (more above)"))
				content.WriteString("\n")
			}

			for i := p.branchScrollOffset; i < endIdx; i++ {
				branch := p.filteredBranches[i]
				prefix := "  "
				style := tuiDimStyle()
				if i == p.selectedBranchIdx {
					prefix = "> "
					if p.fieldIdx == 1 {
						style = tuiSelectedStyle()
					} else {
						style = tuiLabelStyle()
					}
				}
				// Truncate long branch names
				displayBranch := branch
				if len(displayBranch) > 50 {
					displayBranch = displayBranch[:47] + "..."
				}
				content.WriteString(prefix + style.Render(displayBranch))
				content.WriteString("\n")
			}

			// Show scroll indicator if needed
			if endIdx < len(p.filteredBranches) {
				content.WriteString(tuiDimStyle().Render("  ↓ (more below)"))
				content.WriteString("\n")
			}
		}
		content.WriteString("\n")
	} else {
		// New branch name input
		var branchLabel string
		if p.fieldIdx == 1 {
			branchLabel = tuiSuccessStyle().Render("Branch name:") + " " + tuiDimStyle().Render("(editing)")
		} else {
			branchLabel = tuiLabelStyle().Render("Branch name:")
		}
		content.WriteString(branchLabel)
		content.WriteString("\n")
		content.WriteString(p.branchInput.View())
		content.WriteString("\n\n")
	}

	// Action buttons
	content.WriteString("Actions:\n")

	// Execute button
	executeStyle := tuiDimStyle()
	executePrefix := "  "
	if p.fieldIdx == 2 && p.buttonIdx == 0 {
		executeStyle = tuiSelectedStyle()
		executePrefix = "> "
	} else if p.hoveredButton == "execute" {
		executeStyle = tuiSuccessStyle()
	}
	executeButtonText := executePrefix + "Execute"
	content.WriteString("  " + zone.Mark("dialog-execute", executeStyle.Render(executeButtonText)))
	content.WriteString(" - Create work and spawn orchestrator\n")

	// Auto button
	autoStyle := tuiDimStyle()
	autoPrefix := "  "
	if p.fieldIdx == 2 && p.buttonIdx == 1 {
		autoStyle = tuiSelectedStyle()
		autoPrefix = "> "
	} else if p.hoveredButton == "auto" {
		autoStyle = tuiSuccessStyle()
	}
	autoButtonText := autoPrefix + "Auto"
	content.WriteString("  " + zone.Mark("dialog-auto", autoStyle.Render(autoButtonText)))
	content.WriteString(" - Create work with automated workflow\n")

	// Cancel button
	cancelStyle := tuiDimStyle()
	cancelPrefix := "  "
	if p.fieldIdx == 2 && p.buttonIdx == 2 {
		cancelStyle = tuiSelectedStyle()
		cancelPrefix = "> "
	} else if p.hoveredButton == "cancel" {
		cancelStyle = tuiSuccessStyle()
	}
	cancelButtonText := cancelPrefix + "Cancel"
	content.WriteString("  " + zone.Mark("dialog-cancel", cancelStyle.Render(cancelButtonText)))
	content.WriteString(" - Cancel work creation\n")

	// Navigation help
	content.WriteString("\n")
	var helpText string
	if p.useExistingBranch && p.fieldIdx == 1 {
		helpText = "Navigation: [Tab/Shift+Tab] Switch field  [j/k] Navigate  [type] Filter  [Backspace] Clear filter  [Esc] Cancel"
	} else {
		helpText = "Navigation: [Tab/Shift+Tab] Switch field  [j/k] Select button  [Enter] Confirm  [Esc] Cancel"
	}
	content.WriteString(tuiDimStyle().Render(helpText))

	return content.String()
}

// RenderWithPanel returns the panel with border styling
func (p *CreateWorkPanel) RenderWithPanel(contentHeight int) string {
	panelContent := p.Render()

	panelStyle := tuiPanelStyle().Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(CurrentTheme().Accent)
	}

	result := panelStyle.Render(tuiTitleStyle().Render("Create Work") + "\n" + panelContent)

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
