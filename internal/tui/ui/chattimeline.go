package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ChatTimeline is a scrollable, selectable chat message list.
// It renders messages as bubbles and handles line-level scrolling
// to keep the selected message visible within the viewport.
type ChatTimeline struct {
	messages []ChatMessage
	colors   ChatBubbleColors
	width    int
	height   int // viewport height in lines

	selectedIdx      int // -1 = none selected
	suspendedIdx     int // remembered position when tabbing away (-1 = none)
	scrollOffset     int // first visible line

	// Cached render state (invalidated on changes)
	renderedLines []string   // all rendered lines
	msgLineStart  []int      // starting line index for each message
	msgLineEnd    []int      // ending line index (exclusive) for each message
	dirty         bool       // true when cache needs rebuild
}

// NewChatTimeline creates a new scrollable chat timeline.
func NewChatTimeline(colors ChatBubbleColors) *ChatTimeline {
	return &ChatTimeline{
		colors:       colors,
		selectedIdx:  -1,
		suspendedIdx: -1,
		dirty:        true,
	}
}

// SetSize updates the viewport dimensions.
func (t *ChatTimeline) SetSize(width, height int) {
	if t.width != width {
		t.dirty = true // width change affects bubble wrapping
	}
	t.width = width
	t.height = height
	t.clampScroll()
}

// SetColors updates the color scheme.
func (t *ChatTimeline) SetColors(colors ChatBubbleColors) {
	t.colors = colors
	t.dirty = true
}

// SetMessages replaces the message list.
func (t *ChatTimeline) SetMessages(msgs []ChatMessage) {
	t.messages = msgs
	t.dirty = true
	// Clamp selection if messages were removed
	if t.selectedIdx >= len(msgs) {
		t.selectedIdx = len(msgs) - 1
	}
	// Auto-scroll to bottom when new messages arrive and nothing is selected
	if t.selectedIdx < 0 {
		t.scrollToBottom()
	}
}

// AppendMessage adds a message and auto-scrolls to bottom if nothing is selected.
func (t *ChatTimeline) AppendMessage(msg ChatMessage) {
	t.messages = append(t.messages, msg)
	t.dirty = true
	if t.selectedIdx < 0 {
		t.scrollToBottom()
	}
}

// SelectUp moves selection to the previous message, scrolling if needed.
// If nothing is selected, selects the last message.
func (t *ChatTimeline) SelectUp() {
	if len(t.messages) == 0 {
		return
	}
	if t.selectedIdx < 0 {
		t.selectedIdx = len(t.messages) - 1
	} else if t.selectedIdx > 0 {
		t.selectedIdx--
	}
	t.dirty = true
	t.ensureSelectedVisible()
}

// SelectDown moves selection to the next message, scrolling if needed.
// Returns true if selection moved past the last message (caller can use
// this to move focus to the input).
func (t *ChatTimeline) SelectDown() bool {
	if len(t.messages) == 0 {
		return true
	}
	if t.selectedIdx < 0 {
		return true
	}
	if t.selectedIdx < len(t.messages)-1 {
		t.selectedIdx++
		t.dirty = true
		t.ensureSelectedVisible()
		return false
	}
	// Already at last message — signal to move to input
	return true
}

// ClearSelection deselects, clears suspended position, and scrolls to bottom.
func (t *ChatTimeline) ClearSelection() {
	t.selectedIdx = -1
	t.suspendedIdx = -1
	t.dirty = true
	t.scrollToBottom()
}

// SuspendSelection deselects but remembers the current position.
// Use ResumeSelection to restore it. Scroll position is preserved.
func (t *ChatTimeline) SuspendSelection() {
	if t.selectedIdx >= 0 {
		t.suspendedIdx = t.selectedIdx
		t.selectedIdx = -1
		t.dirty = true
		// Don't scroll — keep viewport where it is
	}
}

// ResumeSelection restores the suspended selection and scrolls to it.
// Returns true if a position was restored, false if there was nothing to resume.
func (t *ChatTimeline) ResumeSelection() bool {
	if t.suspendedIdx >= 0 && t.suspendedIdx < len(t.messages) {
		t.selectedIdx = t.suspendedIdx
		t.suspendedIdx = -1
		t.dirty = true
		t.ensureSelectedVisible()
		return true
	}
	return false
}

// ResetSuspended clears the remembered position (e.g., after submitting a prompt).
func (t *ChatTimeline) ResetSuspended() {
	t.suspendedIdx = -1
}

// SelectedIdx returns the currently selected message index (-1 if none).
func (t *ChatTimeline) SelectedIdx() int {
	return t.selectedIdx
}

// SelectedMessage returns the selected message, or nil if none.
func (t *ChatTimeline) SelectedMessage() *ChatMessage {
	if t.selectedIdx >= 0 && t.selectedIdx < len(t.messages) {
		return &t.messages[t.selectedIdx]
	}
	return nil
}

// MessageCount returns the number of messages.
func (t *ChatTimeline) MessageCount() int {
	return len(t.messages)
}

// HasHiddenAbove returns true when there are rendered lines scrolled above the viewport.
func (t *ChatTimeline) HasHiddenAbove() bool {
	t.rebuildIfDirty()
	return t.scrollOffset > 0
}

// Render returns the visible portion of the timeline.
func (t *ChatTimeline) Render() string {
	if len(t.messages) == 0 || t.height <= 0 || t.width <= 0 {
		return ""
	}

	t.rebuildIfDirty()

	totalLines := len(t.renderedLines)
	if totalLines == 0 {
		return ""
	}

	// Clamp scroll offset
	t.clampScroll()

	// Extract visible lines
	end := t.scrollOffset + t.height
	if end > totalLines {
		end = totalLines
	}
	start := t.scrollOffset
	if start > totalLines {
		start = totalLines
	}

	visible := make([]string, len(t.renderedLines[start:end]))
	copy(visible, t.renderedLines[start:end])

	// Pad to fill viewport height (push content to bottom when short)
	padCount := 0
	if len(visible) < t.height {
		padCount = t.height - len(visible)
		padding := make([]string, padCount)
		for i := range padding {
			padding[i] = ""
		}
		visible = append(padding, visible...)
	}

	// Show nav hint above topmost message when scrolled up with a message selected
	if t.selectedIdx >= 0 && t.scrollOffset > 0 && padCount > 0 {
		hintStyle := lipgloss.NewStyle().Foreground(t.colors.TimeDim)
		hint := hintStyle.Render("Shift+Tab to switch to Nav")
		// Place on the last padding line (right above first content), indented 2 cells
		hintIdx := padCount - 1
		visible[hintIdx] = "  " + hint
	}

	return strings.Join(visible, "\n")
}

// rebuildIfDirty re-renders all bubbles and rebuilds the line index.
func (t *ChatTimeline) rebuildIfDirty() {
	if !t.dirty {
		return
	}
	t.dirty = false

	t.renderedLines = nil
	t.msgLineStart = make([]int, len(t.messages))
	t.msgLineEnd = make([]int, len(t.messages))

	for i, msg := range t.messages {
		bubble := RenderChatBubble(msg, t.width, i == t.selectedIdx, t.colors)
		bubbleLines := strings.Split(bubble, "\n")

		t.msgLineStart[i] = len(t.renderedLines)
		t.renderedLines = append(t.renderedLines, bubbleLines...)
		t.msgLineEnd[i] = len(t.renderedLines)
	}
}

// ensureSelectedVisible adjusts scroll so the selected message is fully visible.
func (t *ChatTimeline) ensureSelectedVisible() {
	if t.selectedIdx < 0 || t.selectedIdx >= len(t.messages) {
		return
	}

	t.rebuildIfDirty()

	if len(t.msgLineStart) == 0 {
		return
	}

	msgStart := t.msgLineStart[t.selectedIdx]
	msgEnd := t.msgLineEnd[t.selectedIdx]

	// If message starts above viewport, scroll up to show it
	if msgStart < t.scrollOffset {
		t.scrollOffset = msgStart
	}

	// If message ends below viewport, scroll down to show it
	if msgEnd > t.scrollOffset+t.height {
		t.scrollOffset = msgEnd - t.height
	}
}

// scrollToBottom sets scroll so the last line is visible.
func (t *ChatTimeline) scrollToBottom() {
	t.rebuildIfDirty()
	totalLines := len(t.renderedLines)
	if totalLines > t.height {
		t.scrollOffset = totalLines - t.height
	} else {
		t.scrollOffset = 0
	}
}

// clampScroll ensures scrollOffset is within valid bounds.
func (t *ChatTimeline) clampScroll() {
	t.rebuildIfDirty()
	totalLines := len(t.renderedLines)
	maxOffset := totalLines - t.height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if t.scrollOffset > maxOffset {
		t.scrollOffset = maxOffset
	}
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
}
