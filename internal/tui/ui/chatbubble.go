package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// MessageSource indicates who sent a chat message.
type MessageSource int

const (
	// MessageFromUser is a message sent by the user.
	MessageFromUser MessageSource = iota
	// MessageFromSystem is a message sent by sarge/system.
	MessageFromSystem
)

// ChatMessage represents a single message in the chat timeline.
type ChatMessage struct {
	ID        int64         // DB row ID (0 for unsaved messages)
	Source    MessageSource
	Text      string
	Time      string // e.g. "2m ago", "just now"
	WorkID    string // optional, for (o) navigation
	TaskID    string // optional, for (o) navigation
	BeadID    string // optional, for (o) navigation
	EventType string // for icons later
}

// ChatBubbleColors holds the color scheme for chat bubbles.
type ChatBubbleColors struct {
	UserBg       lipgloss.Color // Background fill for user messages
	UserFg       lipgloss.Color // Text color for user messages
	SystemBorder lipgloss.Color // Border color for system messages
	SystemFg     lipgloss.Color // Text color for system messages
	TimeDim      lipgloss.Color // Dim color for timestamps
	SelectedBg   lipgloss.Color // Background when selected
}

// RenderChatBubble renders a single chat message as an iMessage-style bubble.
// User messages: solid fill, aligned right.
// System messages: border only, aligned left.
func RenderChatBubble(msg ChatMessage, maxWidth int, selected bool, colors ChatBubbleColors) string {
	bubbleWidth := maxWidth * 3 / 4
	if bubbleWidth < 30 {
		bubbleWidth = 30
	}
	if bubbleWidth > maxWidth-4 {
		bubbleWidth = maxWidth - 4
	}

	contentWidth := bubbleWidth - 4 // account for padding/border

	// Wrap text to fit
	wrapped := wordWrap(msg.Text, contentWidth)

	// Timestamp line
	timeStyle := lipgloss.NewStyle().Foreground(colors.TimeDim)
	timeLine := timeStyle.Render(msg.Time)

	if msg.Source == MessageFromUser {
		return renderUserBubble(wrapped, timeLine, bubbleWidth, maxWidth, selected, colors)
	}
	return renderSystemBubble(wrapped, timeLine, bubbleWidth, maxWidth, selected, colors)
}

// renderUserBubble renders a solid-fill bubble aligned to the right.
func renderUserBubble(text, timeLine string, bubbleWidth, maxWidth int, selected bool, colors ChatBubbleColors) string {
	bg := colors.UserBg
	if selected {
		bg = colors.SelectedBg
	}

	style := lipgloss.NewStyle().
		Background(bg).
		Foreground(colors.UserFg).
		Padding(0, 2).
		Width(bubbleWidth).
		Border(lipgloss.RoundedBorder()).
		BorderBackground(bg).
		BorderForeground(bg)

	bubble := style.Render(text)

	// Add timestamp below, right-aligned within the bubble
	timeRight := lipgloss.NewStyle().Width(bubbleWidth).Align(lipgloss.Right)
	timeRendered := timeRight.Render(timeLine)

	block := lipgloss.JoinVertical(lipgloss.Right, bubble, timeRendered)

	// Align the whole block to the right
	return lipgloss.NewStyle().Width(maxWidth).Align(lipgloss.Right).Render(block)
}

// renderSystemBubble renders a border-only bubble aligned to the left.
func renderSystemBubble(text, timeLine string, bubbleWidth, maxWidth int, selected bool, colors ChatBubbleColors) string {
	borderColor := colors.SystemBorder
	if selected {
		borderColor = colors.SelectedBg
	}

	style := lipgloss.NewStyle().
		Foreground(colors.SystemFg).
		Padding(0, 2).
		Width(bubbleWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	bubble := style.Render(text)

	// Add timestamp below, left-aligned
	timeLeft := lipgloss.NewStyle().Width(bubbleWidth).Align(lipgloss.Left).PaddingLeft(1)
	timeRendered := timeLeft.Render(timeLine)

	block := lipgloss.JoinVertical(lipgloss.Left, bubble, timeRendered)

	// Align the whole block to the left
	return lipgloss.NewStyle().Width(maxWidth).Align(lipgloss.Left).Render(block)
}

// RenderChatTimeline renders a list of chat messages as a vertical timeline.
// selectedIdx is the index of the currently selected message (-1 for none).
func RenderChatTimeline(messages []ChatMessage, maxWidth, maxHeight, selectedIdx int, colors ChatBubbleColors) string {
	if len(messages) == 0 {
		return ""
	}

	var lines []string
	for i, msg := range messages {
		bubble := RenderChatBubble(msg, maxWidth, i == selectedIdx, colors)
		lines = append(lines, bubble)
	}

	return strings.Join(lines, "\n")
}

// wordWrap wraps text at word boundaries to fit within maxWidth.
func wordWrap(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	currentLine := words[0]

	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)

	return strings.Join(lines, "\n")
}
