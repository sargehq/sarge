package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sargehq/sarge/internal/bridge"
)

// SessionPanelAction represents actions from the session panel.
type SessionPanelAction int

const (
	SessionPanelActionNone    SessionPanelAction = iota
	SessionPanelActionAbort                      // User pressed abort keybinding
	SessionPanelActionSteer                      // User pressed steer keybinding
	SessionPanelActionPrompt                     // User submitted a prompt
)

// sessionLine represents a rendered line in the session output.
type sessionLine struct {
	text    string
	isError bool
}

// SessionPanel renders streaming output from a pi bridge session and accepts user input.
type SessionPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Viewport for scrollable output
	viewport viewport.Model

	// Text input for interactive prompts
	input     textinput.Model
	inputMode bool // true when user is typing a prompt

	// Session content
	lines      []sessionLine // Accumulated output lines
	currentLine strings.Builder // Current line being built (text deltas)
	autoScroll bool          // Auto-scroll to bottom when new content arrives

	// Session metadata
	sessionID   string
	sessionType string // "orch", "agent", "plan"
	streaming   bool   // Whether session is currently streaming

	// Pending extension UI request
	pendingUIRequest *extensionUIRequest

	// Styles
	toolNameStyle   lipgloss.Style
	toolArgsStyle   lipgloss.Style
	toolResultStyle lipgloss.Style
	thinkingStyle   lipgloss.Style
	errorStyle      lipgloss.Style
	statusStyle     lipgloss.Style
	promptStyle     lipgloss.Style
}

// extensionUIRequest holds a pending UI request from an extension.
type extensionUIRequest struct {
	RequestType string   `json:"requestType"` // "select", "confirm", "input"
	Title       string   `json:"title"`
	Message     string   `json:"message"`
	Options     []string `json:"options,omitempty"`
	Default     string   `json:"default,omitempty"`
}

// NewSessionPanel creates a new SessionPanel.
func NewSessionPanel() *SessionPanel {
	vp := viewport.New(80, 20)
	vp.MouseWheelEnabled = false

	ti := textinput.New()
	ti.Placeholder = "Type a message..."
	ti.CharLimit = 500
	ti.Width = 70

	return &SessionPanel{
		width:      80,
		height:     20,
		viewport:   vp,
		input:      ti,
		autoScroll: true,
		toolNameStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("75")).
			Bold(true),
		toolArgsStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")),
		toolResultStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("249")),
		thinkingStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Italic(true),
		errorStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")),
		statusStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true),
		promptStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")),
	}
}

// SetSize updates the panel dimensions.
func (p *SessionPanel) SetSize(width, height int) {
	p.width = width
	p.height = height

	// Reserve space: 2 for border, 1 for title, 1 for input (if interactive)
	inputLines := 0
	if p.isInteractive() {
		inputLines = 2 // input line + separator
	}
	visibleLines := max(height-4-inputLines, 1)
	p.viewport.Width = width - 2
	p.viewport.Height = visibleLines

	if width > 10 {
		p.input.Width = width - 10
	}
}

// SetFocus updates the focus state.
func (p *SessionPanel) SetFocus(focused bool) {
	p.focused = focused
}

// SetSession configures the session this panel is displaying.
func (p *SessionPanel) SetSession(sessionID, sessionType string) {
	if p.sessionID != sessionID {
		// New session - clear content
		p.lines = nil
		p.currentLine.Reset()
		p.viewport.SetYOffset(0)
		p.autoScroll = true
	}
	p.sessionID = sessionID
	p.sessionType = sessionType
}

// SetStreaming updates whether the session is actively streaming.
func (p *SessionPanel) SetStreaming(streaming bool) {
	p.streaming = streaming
}

// isInteractive returns whether this session type supports user input.
func (p *SessionPanel) isInteractive() bool {
	return p.sessionType == "agent" || p.sessionType == "plan"
}

// HandleEvent processes a bridge event and updates the panel content.
func (p *SessionPanel) HandleEvent(evt bridge.Event) {
	switch evt.Type {
	case bridge.EventAgentStart:
		p.appendStatus("Agent started")
		p.streaming = true

	case bridge.EventAgentEnd:
		p.streaming = false
		p.flushCurrentLine()
		p.appendStatus("Agent finished")

	case bridge.EventTurnStart:
		// Separator between turns
		p.flushCurrentLine()

	case bridge.EventMessageUpdate:
		parsed, err := bridge.ParseMessageUpdateEvent(evt.Raw)
		if err != nil {
			return
		}
		if parsed.AssistantMessageEvent != nil {
			delta := parsed.AssistantMessageEvent
			switch delta.Type {
			case "text_delta":
				p.appendTextDelta(delta.Delta)
			case "thinking_delta":
				p.appendThinking(delta.Delta)
			}
		}

	case bridge.EventToolExecutionStart:
		p.flushCurrentLine()
		var toolEvt bridge.ToolExecutionStartEvent
		if err := json.Unmarshal(evt.Raw, &toolEvt); err == nil {
			p.appendToolStart(toolEvt.ToolName, toolEvt.Args)
		}

	case bridge.EventToolExecutionEnd:
		var toolEvt bridge.ToolExecutionEndEvent
		if err := json.Unmarshal(evt.Raw, &toolEvt); err == nil {
			p.appendToolEnd(toolEvt.ToolName, toolEvt.IsError, toolEvt.Result)
		}

	case bridge.EventAutoCompactionStart:
		p.appendStatus("Compacting context...")

	case bridge.EventAutoCompactionEnd:
		p.appendStatus("Compaction complete")

	case bridge.EventAutoRetryStart:
		p.appendStatus("Retrying...")

	case bridge.EventExtensionUIRequest:
		var req extensionUIRequest
		if err := json.Unmarshal(evt.Raw, &req); err == nil {
			p.pendingUIRequest = &req
		}
	}

	// Auto-scroll to bottom
	if p.autoScroll {
		p.refreshViewport()
		p.viewport.GotoBottom()
	}
}

// appendTextDelta appends streaming text to the current line.
func (p *SessionPanel) appendTextDelta(text string) {
	for _, ch := range text {
		if ch == '\n' {
			p.lines = append(p.lines, sessionLine{text: p.currentLine.String()})
			p.currentLine.Reset()
		} else {
			p.currentLine.WriteRune(ch)
		}
	}
}

// appendThinking appends thinking output (dimmed/italic).
func (p *SessionPanel) appendThinking(text string) {
	p.flushCurrentLine()
	// Split thinking text into lines
	for _, line := range strings.Split(text, "\n") {
		if line != "" {
			p.lines = append(p.lines, sessionLine{text: p.thinkingStyle.Render(line)})
		}
	}
}

// appendToolStart renders a tool execution header.
func (p *SessionPanel) appendToolStart(toolName string, args json.RawMessage) {
	header := p.toolNameStyle.Render("⚙ " + toolName)

	// Parse and display key args
	if len(args) > 0 {
		var argsMap map[string]interface{}
		if err := json.Unmarshal(args, &argsMap); err == nil {
			// Show the most relevant arg based on tool type
			switch toolName {
			case "Bash":
				if cmd, ok := argsMap["command"].(string); ok {
					// Truncate long commands
					innerWidth := p.width - 8
					if len(cmd) > innerWidth {
						cmd = cmd[:innerWidth-3] + "..."
					}
					header += "\n" + p.toolArgsStyle.Render("  $ "+cmd)
				}
			case "Read":
				if path, ok := argsMap["path"].(string); ok {
					header += " " + p.toolArgsStyle.Render(path)
				}
			case "Edit":
				if path, ok := argsMap["path"].(string); ok {
					header += " " + p.toolArgsStyle.Render(path)
				}
			case "Write":
				if path, ok := argsMap["path"].(string); ok {
					header += " " + p.toolArgsStyle.Render(path)
				}
			}
		}
	}

	for _, line := range strings.Split(header, "\n") {
		p.lines = append(p.lines, sessionLine{text: line})
	}
}

// appendToolEnd renders a tool execution result.
func (p *SessionPanel) appendToolEnd(toolName string, isError bool, result json.RawMessage) {
	if isError {
		p.lines = append(p.lines, sessionLine{
			text:    p.errorStyle.Render("  ✗ Error"),
			isError: true,
		})
	} else {
		// For Bash, show truncated output
		if toolName == "Bash" && len(result) > 0 {
			var output string
			if err := json.Unmarshal(result, &output); err == nil {
				lines := strings.Split(output, "\n")
				maxLines := 5
				innerWidth := p.width - 6
				for i, line := range lines {
					if i >= maxLines {
						p.lines = append(p.lines, sessionLine{
							text: p.toolResultStyle.Render(fmt.Sprintf("  ... (%d more lines)", len(lines)-maxLines)),
						})
						break
					}
					if len(line) > innerWidth {
						line = line[:innerWidth-3] + "..."
					}
					p.lines = append(p.lines, sessionLine{
						text: p.toolResultStyle.Render("  " + line),
					})
				}
			}
		}
		p.lines = append(p.lines, sessionLine{text: p.toolNameStyle.Render("  ✓")})
	}
}

// appendStatus appends a status message line.
func (p *SessionPanel) appendStatus(msg string) {
	p.flushCurrentLine()
	p.lines = append(p.lines, sessionLine{text: p.statusStyle.Render("── " + msg + " ──")})
}

// flushCurrentLine moves the current partial line into the lines array.
func (p *SessionPanel) flushCurrentLine() {
	if p.currentLine.Len() > 0 {
		p.lines = append(p.lines, sessionLine{text: p.currentLine.String()})
		p.currentLine.Reset()
	}
}

// refreshViewport updates the viewport content from the accumulated lines.
func (p *SessionPanel) refreshViewport() {
	var b strings.Builder
	for i, line := range p.lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line.text)
	}
	// Include current partial line
	if p.currentLine.Len() > 0 {
		if len(p.lines) > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p.currentLine.String())
	}
	p.viewport.SetContent(b.String())
}

// GetPendingPrompt returns the submitted prompt text and clears the input.
// Returns empty string if no prompt was submitted.
func (p *SessionPanel) GetPendingPrompt() string {
	val := p.input.Value()
	if val != "" {
		// Record the prompt in the output
		p.flushCurrentLine()
		p.lines = append(p.lines, sessionLine{
			text: p.promptStyle.Render("▶ " + val),
		})
		p.input.Reset()
		p.autoScroll = true
		p.refreshViewport()
		p.viewport.GotoBottom()
	}
	return val
}

// GetPendingUIRequest returns and clears any pending extension UI request.
func (p *SessionPanel) GetPendingUIRequest() *extensionUIRequest {
	req := p.pendingUIRequest
	p.pendingUIRequest = nil
	return req
}

// Clear removes all content from the panel.
func (p *SessionPanel) Clear() {
	p.lines = nil
	p.currentLine.Reset()
	p.viewport.SetContent("")
	p.viewport.SetYOffset(0)
	p.autoScroll = true
	p.pendingUIRequest = nil
	p.sessionID = ""
	p.sessionType = ""
	p.streaming = false
}

// ScrollUp scrolls the viewport up.
func (p *SessionPanel) ScrollUp() {
	p.autoScroll = false
	p.viewport.ScrollUp(3)
}

// ScrollDown scrolls the viewport down.
func (p *SessionPanel) ScrollDown() {
	p.viewport.ScrollDown(3)
	// Re-enable auto-scroll if at bottom
	if p.viewport.AtBottom() {
		p.autoScroll = true
	}
}

// Update handles key events for the session panel.
func (p *SessionPanel) Update(msg tea.KeyMsg) (tea.Cmd, SessionPanelAction) {
	// Handle extension UI request dialog if pending
	if p.pendingUIRequest != nil {
		return p.handleUIRequestInput(msg)
	}

	switch msg.String() {
	case "esc":
		if p.inputMode {
			p.inputMode = false
			p.input.Blur()
			return nil, SessionPanelActionNone
		}
		return nil, SessionPanelActionNone

	case "ctrl+c":
		return nil, SessionPanelActionAbort

	case "ctrl+s":
		// Switch to steer mode - prompt input for steering message
		if p.streaming {
			return nil, SessionPanelActionSteer
		}
		return nil, SessionPanelActionNone

	case "enter":
		if p.inputMode {
			val := p.input.Value()
			if val != "" {
				return nil, SessionPanelActionPrompt
			}
			return nil, SessionPanelActionNone
		}
		// Enter input mode
		if p.isInteractive() && !p.streaming {
			p.inputMode = true
			p.input.Focus()
			return textinput.Blink, SessionPanelActionNone
		}
		return nil, SessionPanelActionNone

	case "i":
		// Enter input mode (vim-like)
		if !p.inputMode && p.isInteractive() && !p.streaming {
			p.inputMode = true
			p.input.Focus()
			return textinput.Blink, SessionPanelActionNone
		}
	}

	// If in input mode, pass all keys to the text input
	if p.inputMode {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd, SessionPanelActionNone
	}

	// Viewport scrolling when not in input mode
	switch msg.String() {
	case "j", "down":
		p.ScrollDown()
	case "k", "up":
		p.ScrollUp()
	case "g":
		p.viewport.GotoTop()
		p.autoScroll = false
	case "G":
		p.viewport.GotoBottom()
		p.autoScroll = true
	case "pgdown", "ctrl+d":
		p.viewport.HalfPageDown()
	case "pgup", "ctrl+u":
		p.viewport.HalfPageUp()
		p.autoScroll = false
	}

	return nil, SessionPanelActionNone
}

// handleUIRequestInput handles input for extension UI request dialogs.
func (p *SessionPanel) handleUIRequestInput(msg tea.KeyMsg) (tea.Cmd, SessionPanelAction) {
	req := p.pendingUIRequest

	switch req.RequestType {
	case "confirm":
		switch msg.String() {
		case "y", "Y":
			p.pendingUIRequest = nil
			p.lines = append(p.lines, sessionLine{
				text: p.promptStyle.Render("▶ Yes"),
			})
			// The response handling is delegated to the parent
			return nil, SessionPanelActionPrompt
		case "n", "N", "esc":
			p.pendingUIRequest = nil
			p.lines = append(p.lines, sessionLine{
				text: p.promptStyle.Render("▶ No"),
			})
			return nil, SessionPanelActionPrompt
		}

	case "select":
		// For select, use j/k to navigate and enter to select
		// This is simplified - a full implementation would track cursor position
		switch msg.String() {
		case "esc":
			p.pendingUIRequest = nil
			return nil, SessionPanelActionNone
		}

	case "input":
		// Route to text input
		switch msg.String() {
		case "esc":
			p.pendingUIRequest = nil
			return nil, SessionPanelActionNone
		case "enter":
			p.pendingUIRequest = nil
			return nil, SessionPanelActionPrompt
		default:
			var cmd tea.Cmd
			p.input, cmd = p.input.Update(msg)
			return cmd, SessionPanelActionNone
		}
	}

	return nil, SessionPanelActionNone
}

// Render returns the session panel content.
func (p *SessionPanel) Render() string {
	p.refreshViewport()
	return p.viewport.View()
}

// RenderWithPanel returns the session panel with border styling.
func (p *SessionPanel) RenderWithPanel(contentHeight int) string {
	var content strings.Builder

	// Title with session info
	title := "Session"
	if p.sessionID != "" {
		title = fmt.Sprintf("Session [%s]", p.sessionType)
		if p.streaming {
			title += " ●"
		}
	}
	content.WriteString(tuiTitleStyle.Render(title))
	content.WriteString("\n")

	// Viewport content
	content.WriteString(p.Render())

	// Extension UI request overlay
	if p.pendingUIRequest != nil {
		content.WriteString("\n")
		content.WriteString(p.renderUIRequest())
	}

	// Input area for interactive sessions
	if p.isInteractive() {
		content.WriteString("\n")
		if p.inputMode {
			content.WriteString(p.input.View())
		} else if p.streaming {
			content.WriteString(tuiDimStyle.Render("streaming... ctrl+c abort, ctrl+s steer"))
		} else {
			content.WriteString(tuiDimStyle.Render("press enter or i to type"))
		}
	}

	panelStyle := tuiPanelStyle.Width(p.width).Height(contentHeight - 2)
	if p.focused {
		panelStyle = panelStyle.BorderForeground(lipgloss.Color("214"))
	}

	return panelStyle.Render(content.String())
}

// renderUIRequest renders a pending extension UI request as a dialog overlay.
func (p *SessionPanel) renderUIRequest() string {
	if p.pendingUIRequest == nil {
		return ""
	}
	req := p.pendingUIRequest
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("99"))
	b.WriteString(titleStyle.Render(req.Title))
	b.WriteString("\n")

	if req.Message != "" {
		innerWidth := p.width - 6
		msg := req.Message
		if len(msg) > innerWidth {
			msg = ansi.Truncate(msg, innerWidth, "...")
		}
		b.WriteString(msg)
		b.WriteString("\n")
	}

	switch req.RequestType {
	case "confirm":
		b.WriteString(tuiDimStyle.Render("[y]es / [n]o"))
	case "select":
		for i, opt := range req.Options {
			if i > 8 {
				b.WriteString(fmt.Sprintf("\n  ... (%d more)", len(req.Options)-i))
				break
			}
			b.WriteString(fmt.Sprintf("\n  %d. %s", i+1, opt))
		}
	case "input":
		b.WriteString(p.input.View())
	}

	return b.String()
}
