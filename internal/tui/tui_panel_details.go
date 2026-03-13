package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
	"github.com/sargehq/sarge/internal/logging"
)

// Panel padding: tuiPanelStyle has Padding(0, 1) = 2 chars horizontal padding total

// IssueDetailsPanel renders issue details for the focused bean.
type IssueDetailsPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Viewport for scrolling
	viewport viewport.Model

	// Data (set by coordinator)
	focusedBean      *beanItem
	hasActiveSession bool
	childBeanMap     map[string]*beanItem // For looking up child status
}

// NewIssueDetailsPanel creates a new IssueDetailsPanel
func NewIssueDetailsPanel() *IssueDetailsPanel {
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(20)) // Initial size, will be updated
	// Mouse wheel events are handled at the top level (planModel.handleMouseWheel)
	// to ensure only the panel under the cursor scrolls
	vp.MouseWheelEnabled = false

	return &IssueDetailsPanel{
		width:        60,
		height:       20,
		viewport:     vp,
		childBeanMap: make(map[string]*beanItem),
	}
}

// SetSize updates the panel dimensions
func (p *IssueDetailsPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Update viewport dimensions
	// Calculate available lines for content:
	// - 2 for border (top + bottom)
	// - 1 for title line
	// - 1 for the status bar.
	visibleLines := max(height-4, 1)

	// Set viewport size accounting for padding (2 chars total)
	p.viewport.SetWidth(width - 2)
	p.viewport.SetHeight(visibleLines)
}

// SetFocus updates the focus state
func (p *IssueDetailsPanel) SetFocus(focused bool) {
	p.focused = focused
}

// IsFocused returns whether the panel is focused
func (p *IssueDetailsPanel) IsFocused() bool {
	return p.focused
}

// SetData updates the panel's data with the focused bean
func (p *IssueDetailsPanel) SetData(focusedBean *beanItem, hasActiveSession bool, childBeanMap map[string]*beanItem) {
	// Check if bean changed - reset scroll if so
	beanChanged := p.focusedBean == nil || focusedBean == nil ||
		(p.focusedBean != nil && focusedBean != nil && p.focusedBean.ID != focusedBean.ID)

	p.focusedBean = focusedBean
	p.hasActiveSession = hasActiveSession
	p.childBeanMap = childBeanMap

	// Reset scroll when switching beans
	if beanChanged {
		p.viewport.SetYOffset(0)
	}
}

// ScrollUp scrolls the content up (shows earlier content)
func (p *IssueDetailsPanel) ScrollUp() {
	p.viewport.ScrollUp(1)
}

// ScrollDown scrolls the content down (shows later content)
func (p *IssueDetailsPanel) ScrollDown() {
	p.viewport.ScrollDown(1)
}

// ScrollToTop scrolls to the beginning of the content
func (p *IssueDetailsPanel) ScrollToTop() {
	p.viewport.GotoTop()
}

// ScrollToBottom scrolls to the end of the content
func (p *IssueDetailsPanel) ScrollToBottom() {
	p.viewport.GotoBottom()
}

// GetViewport returns the viewport for external updates
func (p *IssueDetailsPanel) GetViewport() *viewport.Model {
	return &p.viewport
}

// Render returns the details panel content (without border/panel styling)
func (p *IssueDetailsPanel) Render() string {
	// Update viewport content
	fullContent := p.renderFullIssueContent()
	p.viewport.SetContent(fullContent)

	// Return viewport's rendered view
	return p.viewport.View()
}

// RenderWithPanel returns the details panel with border styling
func (p *IssueDetailsPanel) RenderWithPanel(contentHeight int) string {
	detailsContent := p.Render()

	// Truncate content to fit exactly within the panel.
	// The viewport constrains line count, but lipgloss may wrap long lines
	// inside the bordered panel, pushing the rendered height beyond the target.
	// Inner space: contentHeight - 2 (border) - 1 (title) = contentHeight - 3
	targetContentLines := contentHeight - 3
	linesBeforeTrunc := strings.Count(detailsContent, "\n") + 1
	detailsContent = padOrTruncateLines(detailsContent, targetContentLines)
	linesAfterTrunc := strings.Count(detailsContent, "\n") + 1

	panelStyle := tuiPanelStyle.Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(lipgloss.Color("214"))
	}

	innerContent := tuiTitleStyle.Render("Details") + "\n" + detailsContent
	innerLines := strings.Count(innerContent, "\n") + 1

	result := panelStyle.Render(innerContent)
	resultHeight := lipgloss.Height(result)
	resultLineCount := strings.Count(result, "\n") + 1

	// Log every line width in the result to find wrapping
	if resultHeight != contentHeight {
		resultLines := strings.Split(result, "\n")
		for i, line := range resultLines {
			w := lipgloss.Width(line)
			if w > p.width+2 { // +2 for border chars
				logging.Debug("DetailsPanel WIDE LINE",
					"lineIndex", i,
					"lineWidth", w,
					"panelWidth", p.width,
					"line", line,
				)
			}
		}
	}

	logging.Debug("DetailsPanel.RenderWithPanel",
		"contentHeight", contentHeight,
		"targetContentLines", targetContentLines,
		"viewportHeight", p.viewport.Height(),
		"viewportWidth", p.viewport.Width(),
		"linesBeforeTrunc", linesBeforeTrunc,
		"linesAfterTrunc", linesAfterTrunc,
		"innerLines", innerLines,
		"panelStyleHeight", contentHeight-2,
		"panelWidth", p.width,
		"resultHeight(lipgloss)", resultHeight,
		"resultLineCount", resultLineCount,
		"overflow", resultHeight > contentHeight,
	)

	return result
}

// padOrTruncateLines ensures content has exactly targetLines lines.
func padOrTruncateLines(content string, targetLines int) string {
	if targetLines < 1 {
		targetLines = 1
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > targetLines {
		lines = lines[:targetLines]
	} else {
		for len(lines) < targetLines {
			lines = append(lines, "")
		}
	}
	return strings.Join(lines, "\n")
}

// renderFullIssueContent renders all content without line limits
func (p *IssueDetailsPanel) renderFullIssueContent() string {
	if p.focusedBean == nil {
		return tuiDimStyle.Render("No issue selected")
	}

	var content strings.Builder
	bean := p.focusedBean

	// Calculate inner width (panel has Padding(0, 1) = 2 chars total horizontal padding)
	innerWidth := p.width - 2

	// Build header line - may need truncation to fit
	var header strings.Builder
	header.WriteString(tuiLabelStyle.Render("ID: "))
	header.WriteString(tuiValueStyle.Render(bean.ID))
	header.WriteString("  ")
	header.WriteString(tuiLabelStyle.Render("Type: "))
	header.WriteString(tuiValueStyle.Render(bean.Type))
	header.WriteString("  ")
	header.WriteString(tuiLabelStyle.Render("P"))
	header.WriteString(tuiValueStyle.Render(bean.Priority))
	header.WriteString("  ")
	header.WriteString(tuiLabelStyle.Render("Status: "))
	header.WriteString(tuiValueStyle.Render(bean.Status))
	if p.hasActiveSession {
		header.WriteString("  ")
		header.WriteString(tuiSuccessStyle.Render("[Session Active]"))
	}
	if bean.assignedWorkID != "" {
		header.WriteString("  ")
		header.WriteString(tuiDimStyle.Render("Work: " + bean.assignedWorkID))
	}

	// Truncate header to fit inner width
	headerStr := header.String()
	if lipgloss.Width(headerStr) > innerWidth {
		headerStr = ansi.Truncate(headerStr, innerWidth, "...")
	}
	content.WriteString(headerStr)
	content.WriteString("\n")

	// Truncate title to fit on one line
	titleStr := bean.Title
	if lipgloss.Width(titleStr) > innerWidth {
		titleStr = ansi.Truncate(titleStr, innerWidth, "...")
	}
	content.WriteString(tuiValueStyle.Render(titleStr))

	// Show full description
	if bean.Body != "" {
		content.WriteString("\n\n")
		// Word wrap description to fit within inner width
		wrapped := wordwrap.String(bean.Body, innerWidth)
		content.WriteString(tuiDimStyle.Render(wrapped))
	}

	// Show all children (issues blocked by this one)
	if len(bean.children) > 0 {
		content.WriteString("\n\n")
		content.WriteString(tuiLabelStyle.Render("Blocks:"))

		// Show all children with status
		for _, childID := range bean.children {
			var childLine string
			if child, ok := p.childBeanMap[childID]; ok {
				childLine = fmt.Sprintf("\n  %s %s %s",
					statusIcon(child.Status),
					issueIDStyle.Render(child.ID),
					child.Title)
			} else {
				childLine = fmt.Sprintf("\n  ? %s", issueIDStyle.Render(childID))
			}
			// Truncate to fit inner width
			if lipgloss.Width(childLine)-1 > innerWidth {
				childLine = ansi.Truncate(childLine, innerWidth+1, "...")
			}
			content.WriteString(childLine)
		}
	}

	return content.String()
}
