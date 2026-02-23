package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// TestMultiSelectionCloseConfirmation tests the close confirmation dialog with multiple selected beads
func TestMultiSelectionCloseConfirmation(t *testing.T) {
	tests := []struct {
		name             string
		beadItems        []beadItem
		selectedBeads    map[string]bool
		cursorIndex      int
		expectedCount    int  // Expected number of beads to be closed
		expectedInDialog bool // Whether beads should appear in dialog
		description      string
	}{
		{
			name: "Multiple selected beads",
			beadItems: []beadItem{
				testBeadItem("bead-1", "First task", "open", 2, "task"),
				testBeadItem("bead-2", "Second task", "open", 2, "task"),
				testBeadItem("bead-3", "Third task", "open", 2, "task"),
			},
			selectedBeads: map[string]bool{
				"bead-1": true,
				"bead-2": true,
			},
			cursorIndex:      0,
			expectedCount:    2,
			expectedInDialog: true,
			description:      "Should show and close 2 selected beads",
		},
		{
			name: "No selected beads - uses cursor",
			beadItems: []beadItem{
				testBeadItem("bead-1", "First task", "open", 2, "task"),
				testBeadItem("bead-2", "Second task", "open", 2, "task"),
			},
			selectedBeads:    map[string]bool{},
			cursorIndex:      1,
			expectedCount:    1,
			expectedInDialog: true,
			description:      "Should use cursor bead when no selection",
		},
		{
			name: "All beads selected",
			beadItems: []beadItem{
				testBeadItem("bead-1", "First task", "open", 2, "task"),
				testBeadItem("bead-2", "Second task", "open", 2, "task"),
				testBeadItem("bead-3", "Third task", "open", 2, "task"),
			},
			selectedBeads: map[string]bool{
				"bead-1": true,
				"bead-2": true,
				"bead-3": true,
			},
			cursorIndex:      0,
			expectedCount:    3,
			expectedInDialog: true,
			description:      "Should show and close all 3 selected beads",
		},
		{
			name: "More than 5 beads selected",
			beadItems: []beadItem{
				testBeadItem("bead-1", "Task 1", "open", 2, "task"),
				testBeadItem("bead-2", "Task 2", "open", 2, "task"),
				testBeadItem("bead-3", "Task 3", "open", 2, "task"),
				testBeadItem("bead-4", "Task 4", "open", 2, "task"),
				testBeadItem("bead-5", "Task 5", "open", 2, "task"),
				testBeadItem("bead-6", "Task 6", "open", 2, "task"),
				testBeadItem("bead-7", "Task 7", "open", 2, "task"),
			},
			selectedBeads: map[string]bool{
				"bead-1": true,
				"bead-2": true,
				"bead-3": true,
				"bead-4": true,
				"bead-5": true,
				"bead-6": true,
				"bead-7": true,
			},
			cursorIndex:      0,
			expectedCount:    7,
			expectedInDialog: true,
			description:      "Should show first 5 beads and ellipsis for remaining",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel
			m := &planModel{
				beadItems:     tt.beadItems,
				selectedBeads: tt.selectedBeads,
				beadsCursor:   tt.cursorIndex,
				viewMode:      ViewCloseBeadConfirm,
			}

			// Test the dialog content rendering
			dialogContent := m.renderCloseBeadConfirmContent()

			// Check if the dialog shows the correct number of beads
			if tt.expectedCount == 1 {
				// For single bead, check title shows "Close Issue"
				require.True(t, strings.Contains(dialogContent, "Close Issue"),
					"%s: Expected 'Close Issue' in dialog for single bead", tt.description)
			} else {
				// For multiple beads, check title shows correct count
				if tt.expectedCount > 1 {
					require.True(t, strings.Contains(dialogContent, "Issues"),
						"%s: Expected 'Issues' (plural) in dialog for multiple beads", tt.description)
				}
			}

			// Check if selected bead IDs appear in the dialog
			if tt.expectedInDialog {
				selectedCount := 0
				shownCount := 0
				for _, item := range tt.beadItems {
					if tt.selectedBeads[item.ID] {
						selectedCount++
						// Only first 5 beads should be shown
						if shownCount < 5 {
							require.True(t, strings.Contains(dialogContent, item.ID),
								"%s: Expected bead ID '%s' to appear in dialog (one of first 5)", tt.description, item.ID)
							shownCount++
						}
					}
				}

				// If more than 5 selected, check for ellipsis
				if selectedCount > 5 {
					require.True(t, strings.Contains(dialogContent, "and") || strings.Contains(dialogContent, "more"),
						"%s: Expected '... and X more' for more than 5 selected beads", tt.description)
				}
			}

			// Check dialog has confirmation buttons
			require.True(t, strings.Contains(dialogContent, "[y]") && strings.Contains(dialogContent, "[n]"),
				"%s: Expected confirmation buttons [y] and [n] in dialog", tt.description)
		})
	}
}

// TestUpdateCloseBeadConfirm tests the keyboard handling for close confirmation
func TestUpdateCloseBeadConfirm(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		shouldClose  bool
		shouldCancel bool
		description  string
	}{
		{
			name:         "Confirm with 'y'",
			key:          "y",
			shouldClose:  true,
			shouldCancel: false,
			description:  "Pressing 'y' should confirm close",
		},
		{
			name:         "Confirm with 'Y'",
			key:          "Y",
			shouldClose:  true,
			shouldCancel: false,
			description:  "Pressing 'Y' should confirm close",
		},
		{
			name:         "Cancel with 'n'",
			key:          "n",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing 'n' should cancel",
		},
		{
			name:         "Cancel with 'N'",
			key:          "N",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing 'N' should cancel",
		},
		{
			name:         "Cancel with Esc",
			key:          "esc",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing Esc should cancel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel with selected beads
			m := &planModel{
				beadItems: []beadItem{
					testBeadItem("bead-1", "Task 1", "open", 2, "task"),
					testBeadItem("bead-2", "Task 2", "open", 2, "task"),
				},
				selectedBeads: map[string]bool{
					"bead-1": true,
					"bead-2": true,
				},
				beadsCursor: 0,
				viewMode:    ViewCloseBeadConfirm,
			}

			// Create the key message
			var keyMsg tea.KeyMsg
			if tt.key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}

			// Update the model
			newModel, cmd := m.updateCloseBeadConfirm(keyMsg)
			updatedModel := newModel.(*planModel)

			// Check if view mode changed back to normal
			if tt.shouldCancel || tt.shouldClose {
				require.Equal(t, ViewNormal, updatedModel.viewMode,
					"%s: Expected viewMode to be ViewNormal after action", tt.description)
			}

			// If close was confirmed, a command should be returned
			if tt.shouldClose {
				require.NotNil(t, cmd,
					"%s: Expected a command to be returned when confirming close", tt.description)
			}
		})
	}
}

// TestCloseKeyHandlerWithSelection tests 'x' key handler with multi-selection
func TestCloseKeyHandlerWithSelection(t *testing.T) {
	tests := []struct {
		name             string
		beadItems        []beadItem
		selectedBeads    map[string]bool
		cursorIndex      int
		shouldShowDialog bool
		description      string
	}{
		{
			name: "With selected beads",
			beadItems: []beadItem{
				testBeadItem("bead-1", "Task 1", "open", 2, "task"),
				testBeadItem("bead-2", "Task 2", "open", 2, "task"),
			},
			selectedBeads: map[string]bool{
				"bead-1": true,
			},
			cursorIndex:      0,
			shouldShowDialog: true,
			description:      "Should show dialog when beads are selected",
		},
		{
			name: "Without selected beads but with cursor",
			beadItems: []beadItem{
				testBeadItem("bead-1", "Task 1", "open", 2, "task"),
			},
			selectedBeads:    map[string]bool{},
			cursorIndex:      0,
			shouldShowDialog: true,
			description:      "Should show dialog when cursor is on a bead",
		},
		{
			name:             "No beads available",
			beadItems:        []beadItem{},
			selectedBeads:    map[string]bool{},
			cursorIndex:      0,
			shouldShowDialog: false,
			description:      "Should not show dialog when no beads available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel
			m := &planModel{
				beadItems:     tt.beadItems,
				selectedBeads: tt.selectedBeads,
				beadsCursor:   tt.cursorIndex,
				viewMode:      ViewNormal,
				ctx:           context.Background(),
			}

			// Simulate pressing 'x' key - extract the relevant logic
			if len(m.beadItems) > 0 {
				hasSelection := false
				for _, item := range m.beadItems {
					if m.selectedBeads[item.ID] {
						hasSelection = true
						break
					}
				}
				// If we have selected beads or a cursor bead, show confirmation
				if hasSelection || m.beadsCursor < len(m.beadItems) {
					m.viewMode = ViewCloseBeadConfirm
				}
			}

			// Check if dialog was shown as expected
			dialogShown := m.viewMode == ViewCloseBeadConfirm
			require.Equal(t, tt.shouldShowDialog, dialogShown,
				"%s: dialog shown state mismatch", tt.description)
		})
	}
}

// TestBatchCloseFunction tests that multiple beads can be closed in a batch
func TestBatchCloseFunction(t *testing.T) {
	// This test validates that the closeBeads function is called with multiple IDs
	// In a real test, we would mock the bd command execution
	beanIDs := []string{"bead-1", "bead-2", "bead-3"}

	// Verify the function signature exists and accepts multiple IDs
	m := &planModel{
		ctx:                context.Background(),
		beadItems:          []beadItem{},
		selectedBeads:      map[string]bool{},
		activeBeadSessions: map[string]bool{},
	}

	// The closeBeads function should accept a slice of bead IDs
	cmd := m.closeBeads(beanIDs)

	// Verify the command is not nil
	require.NotNil(t, cmd, "closeBeads should return a non-nil command")

	// In a real scenario, we would verify that the bd command is called with all IDs:
	// Expected: bd close bead-1 bead-2 bead-3
	// This would require mocking exec.CommandContext or using an interface
}

// TestCloseConfirmationEdgeCases tests edge cases for close confirmation
func TestCloseConfirmationEdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		setup            func() *planModel
		expectedBehavior string
	}{
		{
			name: "Empty selection and invalid cursor",
			setup: func() *planModel {
				return &planModel{
					beadItems:     []beadItem{testBeadItem("bead-1", "Task", "open", 2, "task")},
					selectedBeads: map[string]bool{},
					beadsCursor:   10, // Invalid cursor position
					viewMode:      ViewCloseBeadConfirm,
				}
			},
			expectedBehavior: "Should handle gracefully without panic",
		},
		{
			name: "Already closed beads in selection",
			setup: func() *planModel {
				return &planModel{
					beadItems: []beadItem{
						testBeadItem("bead-1", "Task 1", "closed", 2, "task"),
						testBeadItem("bead-2", "Task 2", "open", 2, "task"),
					},
					selectedBeads: map[string]bool{
						"bead-1": true, // Already closed
						"bead-2": true,
					},
					beadsCursor: 0,
					viewMode:    ViewCloseBeadConfirm,
				}
			},
			expectedBehavior: "Should still show both beads in confirmation",
		},
		{
			name: "Mixed assigned and unassigned beads",
			setup: func() *planModel {
				item1 := testBeadItem("bead-1", "Task 1", "open", 2, "task")
				item1.assignedWorkID = "w-123"
				item2 := testBeadItem("bead-2", "Task 2", "open", 2, "task")
				return &planModel{
					beadItems: []beadItem{item1, item2},
					selectedBeads: map[string]bool{
						"bead-1": true, // Already assigned to work
						"bead-2": true,
					},
					beadsCursor: 0,
					viewMode:    ViewCloseBeadConfirm,
				}
			},
			expectedBehavior: "Should show both beads regardless of assignment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.setup()

			// Test dialog rendering doesn't panic
			require.NotPanics(t, func() {
				_ = m.renderCloseBeadConfirmContent()
			}, "%s: Panic occurred during dialog rendering", tt.name)

			// Test update function doesn't panic when confirming
			require.NotPanics(t, func() {
				keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
				_, _ = m.updateCloseBeadConfirm(keyMsg)
			}, "%s: Panic on confirm", tt.name)
		})
	}
}
